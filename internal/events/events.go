// Package events publishes domain events, so that work which is not the
// caller's concern — a welcome email, an analytics record — happens somewhere
// other than the request that triggered it.
package events

import "context"

// Event types.
//
// Dotted and past tense: an event is a statement that something happened, and
// the name is a contract with every consumer, so it changes only when the
// meaning does.
const (
	// TypeUserRegistered fires once, when an account is created.
	TypeUserRegistered = "user.registered"
	// TypeUserLoggedIn fires when credentials are exchanged for tokens. It
	// does not fire on a token refresh: refreshing is the session continuing,
	// not a person logging in, and a consumer that emails on sign-in would
	// otherwise write every time a client rotated its token.
	TypeUserLoggedIn = "user.logged_in"
)

// Metadata keys set on every message.
const (
	// MetadataEventType lets a consumer route a message without unmarshalling
	// its body.
	MetadataEventType = "event_type"
)

// Publisher publishes domain events.
//
// Publish takes a context for the contract's sake, but see SQS.Publish: the
// underlying client does not honour it.
type Publisher interface {
	Publish(ctx context.Context, eventType string, payload any, metadata map[string]string) error
	Close() error
}

// UserEvent is the payload of the user.* events.
//
// A purpose-built struct, not models.User as the reference publishes. A model
// is the shape of a database table: publishing it makes every consumer depend
// on our column names, turns a migration into a breaking change for them, and
// means anything added to the table later starts flowing onto a queue without
// anyone deciding it should.
type UserEvent struct {
	UserID    uint   `json:"user_id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Role      string `json:"role"`
}

// Config is what a publisher needs, so this package does not depend on the
// whole application configuration.
type Config struct {
	// Enabled false gives a publisher that discards everything.
	Enabled     bool
	QueueName   string
	Region      string
	AccessKeyID string
	SecretKey   string
	// Endpoint points the SQS client somewhere other than AWS, e.g. LocalStack.
	Endpoint string
}

// New builds the publisher described by cfg, or one that discards events when
// they are switched off.
func New(ctx context.Context, cfg *Config) (Publisher, error) {
	if !cfg.Enabled {
		return Noop{}, nil
	}

	return NewSQS(ctx, cfg)
}

// Noop discards every event.
//
// It is what runs when events are switched off, so that nothing else has to
// know whether publishing is configured: the alternative is a nil check at
// every call site, and the one that gets forgotten panics.
type Noop struct{}

// Publish discards the event.
func (Noop) Publish(_ context.Context, _ string, _ any, _ map[string]string) error {
	return nil
}

// Close does nothing.
func (Noop) Close() error {
	return nil
}
