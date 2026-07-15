package events

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	watermillsqs "github.com/ThreeDotsLabs/watermill-aws/sqs"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	amazonsqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	transport "github.com/aws/smithy-go/endpoints" // path says endpoints, package is transport
)

// Bounds on a single publish.
//
// Publishing happens inside the request that caused the event, so every one of
// these is time a person spends waiting to log in.
const (
	// publishTimeout bounds one SQS call. It is enforced on the HTTP client
	// rather than through a context, because watermill's SQS publisher calls
	// the AWS SDK with its own context.Background() and so never sees a
	// deadline we pass it. Without this a wedged queue would hang sign-ins.
	publishTimeout = 2 * time.Second
	// publishAttempts is deliberately low. The SDK defaults to three attempts
	// with exponential backoff, which measured at 3.3 seconds added to a
	// registration while the queue was down — the request survives, as it
	// must, but nobody wants to wait that long for it. One retry covers a
	// blip; anything more is making a user wait for an email they will never
	// notice is late.
	publishAttempts = 2
	// publishBackoff caps the wait between those attempts.
	publishBackoff = 200 * time.Millisecond
)

// SQS publishes events to an SQS queue.
type SQS struct {
	publisher message.Publisher
	queue     string
}

// NewSQS builds an SQS publisher.
//
// It returns an error rather than panicking as the reference does: a panic in a
// constructor replaces the one line naming the broken setting with a stack
// trace, and cannot be tested.
func NewSQS(ctx context.Context, cfg *Config) (*SQS, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithHTTPClient(&http.Client{Timeout: publishTimeout}),
		awsconfig.WithRetryer(func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) {
				o.MaxAttempts = publishAttempts
				o.MaxBackoff = publishBackoff
			})
		}),
	}

	// Static credentials only when they are configured, so the default chain —
	// which is what finds an instance role in production — still runs
	// otherwise. The reference hardcodes "test"/"test" whenever an endpoint is
	// set, so pointing it at any non-AWS endpoint that is not LocalStack sends
	// credentials that cannot work.
	if cfg.AccessKeyID != "" && cfg.SecretKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	var optFns []func(*amazonsqs.Options)

	if cfg.Endpoint != "" {
		endpoint, parseErr := url.Parse(cfg.Endpoint)
		if parseErr != nil {
			return nil, fmt.Errorf("parse sqs endpoint %q: %w", cfg.Endpoint, parseErr)
		}

		// The resolver the library documents for emulators, rather than the
		// reference's config.WithBaseEndpoint — which it feeds the *S3*
		// endpoint, and which only works because LocalStack happens to serve
		// every service on one port.
		optFns = append(optFns, amazonsqs.WithEndpointResolverV2(
			watermillsqs.OverrideEndpointResolver{
				Endpoint: transport.Endpoint{URI: *endpoint},
			},
		))
	}

	publisher, err := watermillsqs.NewPublisher(watermillsqs.PublisherConfig{
		AWSConfig: awsCfg,
		OptFns:    optFns,
		// The queue is provisioned by the init script in development, and by
		// whatever provisions infrastructure in production. Left to create it,
		// a typo in the queue name would silently make a second queue that
		// nothing reads, and the events would look published.
		DoNotCreateQueueIfNotExists: true,
	}, watermillLogger())
	if err != nil {
		return nil, fmt.Errorf("create sqs publisher: %w", err)
	}

	return &SQS{publisher: publisher, queue: cfg.QueueName}, nil
}

// Publish sends one event to the queue.
//
// ctx is accepted for the interface's sake and is not honoured: watermill's SQS
// publisher builds its own context.Background() internally. publishTimeout is
// what actually bounds the call.
func (p *SQS) Publish(_ context.Context, eventType string, payload any, metadata map[string]string) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal %s payload: %w", eventType, err)
	}

	msg := message.NewMessage(watermill.NewUUID(), data)

	msg.Metadata.Set(MetadataEventType, eventType)

	for k, v := range metadata {
		msg.Metadata.Set(k, v)
	}

	if err := p.publisher.Publish(p.queue, msg); err != nil {
		return fmt.Errorf("publish %s to %s: %w", eventType, p.queue, err)
	}

	return nil
}

// Close releases the publisher.
func (p *SQS) Close() error {
	if err := p.publisher.Close(); err != nil {
		return fmt.Errorf("close sqs publisher: %w", err)
	}

	return nil
}

// watermillLogger discards watermill's own logging.
//
// Its output is neither this application's format nor its levels. The thing
// worth knowing — that a publish failed — is returned from Publish and logged
// by the caller, with the user and event that make it useful.
func watermillLogger() watermill.LoggerAdapter {
	return watermill.NopLogger{}
}
