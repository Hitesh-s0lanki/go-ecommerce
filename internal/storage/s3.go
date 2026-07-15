package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3 stores uploads in an S3 bucket, or anything that speaks its API —
// LocalStack in development, MinIO, R2.
type S3 struct {
	client   *s3.Client
	bucket   string
	baseURL  string
	endpoint string
}

// NewS3 builds an S3 provider.
//
// It returns an error rather than panicking as the reference does: a panic in a
// constructor takes the process down with a stack trace instead of the one line
// saying which setting is wrong, and it cannot be tested.
func NewS3(ctx context.Context, cfg *Config) (*S3, error) {
	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(cfg.Region)}

	// Static credentials only when they are configured. Otherwise the default
	// chain runs, which is what picks up an instance role in production —
	// hard-coding empty strings would break that.
	if cfg.AccessKeyID != "" && cfg.SecretKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			// LocalStack and MinIO serve buckets as a path, not as a subdomain
			// of the endpoint, which is what the SDK would otherwise assume.
			o.UsePathStyle = true
		}
	})

	return &S3{
		client:   client,
		bucket:   cfg.Bucket,
		baseURL:  strings.TrimSuffix(cfg.PublicBaseURL, "/"),
		endpoint: strings.TrimSuffix(cfg.Endpoint, "/"),
	}, nil
}

// Upload stores the bytes under key and returns the URL to fetch them from.
//
// PutObject rather than the transfer manager's multipart upload: uploads here
// are single images capped at MAX_UPLOAD_SIZE, well under the 5 GiB limit of a
// single PUT, so splitting them across parts buys nothing.
func (s *S3) Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) (string, error) {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   r,
		// Given explicitly so the SDK streams the body instead of reading it
		// all into memory to discover its length.
		ContentLength: aws.Int64(size),
		// Recorded on the object, so whatever serves it says "image" rather
		// than leaving the browser to guess. The reference omits this and S3
		// then defaults to application/octet-stream, which downloads the file
		// instead of displaying it.
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("upload to s3: %w", err)
	}

	return s.publicURL(key), nil
}

// Delete removes the object at key. S3 already treats deleting an absent key as
// a success, which is exactly the behaviour Provider asks for.
func (s *S3) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete from s3: %w", err)
	}

	return nil
}

// publicURL builds the URL a client fetches the key from.
//
// The reference returns the bare key here, so what lands in the database is not
// a URL at all and nothing can render it without knowing the bucket layout.
func (s *S3) publicURL(key string) string {
	// A CDN or bucket domain in front of the files.
	if s.baseURL != "" {
		return s.baseURL + "/" + key
	}

	// Development against LocalStack: path-style, addressing the endpoint.
	if s.endpoint != "" {
		return fmt.Sprintf("%s/%s/%s", s.endpoint, s.bucket, key)
	}

	// Real S3, no CDN configured.
	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", s.bucket, key)
}
