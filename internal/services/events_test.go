package services_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/events"
)

// publishedEvent is one announcement the fake recorded.
type publishedEvent struct {
	Type     string
	Payload  any
	Metadata map[string]string
}

// fakePublisher records what it was asked to publish.
//
// Mutex-guarded: publishing happens on whatever goroutine the caller is on, and
// the tests run with -race.
type fakePublisher struct {
	mu        sync.Mutex
	published []publishedEvent
	err       error
	closed    bool
}

func (f *fakePublisher) Publish(_ context.Context, eventType string, payload any, metadata map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return f.err
	}

	f.published = append(f.published, publishedEvent{Type: eventType, Payload: payload, Metadata: metadata})

	return nil
}

func (f *fakePublisher) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.closed = true

	return nil
}

func (f *fakePublisher) events() []publishedEvent {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]publishedEvent(nil), f.published...)
}

func (f *fakePublisher) failWith(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.err = err
}

func TestRegisterPublishesUserRegistered(t *testing.T) {
	svc, _, publisher := newAuthServiceWithEvents(t)

	reg, err := svc.Register(context.Background(), registerReq("newuser@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	published := publisher.events()
	if len(published) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(published), published)
	}

	if published[0].Type != events.TypeUserRegistered {
		t.Errorf("Type = %q, want %q", published[0].Type, events.TypeUserRegistered)
	}

	payload, ok := published[0].Payload.(events.UserEvent)
	if !ok {
		t.Fatalf("payload is %T, want events.UserEvent", published[0].Payload)
	}

	if payload.UserID != reg.User.ID {
		t.Errorf("UserID = %d, want %d", payload.UserID, reg.User.ID)
	}
	if payload.Email != "newuser@example.com" {
		t.Errorf("Email = %q, want newuser@example.com", payload.Email)
	}
	// A welcome email needs a name to greet.
	if payload.FirstName == "" {
		t.Error("FirstName is empty")
	}
}

func TestLoginPublishesUserLoggedIn(t *testing.T) {
	svc, _, publisher := newAuthServiceWithEvents(t)
	ctx := context.Background()

	if _, err := svc.Register(ctx, registerReq("loginevent@example.com")); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if _, err := svc.Login(ctx, &dto.LoginRequest{
		Email: "loginevent@example.com", Password: "correct-horse-battery",
	}); err != nil {
		t.Fatalf("Login: %v", err)
	}

	published := publisher.events()
	if len(published) != 2 {
		t.Fatalf("got %d events, want registered then logged_in: %+v", len(published), published)
	}

	if published[1].Type != events.TypeUserLoggedIn {
		t.Errorf("Type = %q, want %q", published[1].Type, events.TypeUserLoggedIn)
	}
}

// The reference publishes its login event from the helper that Register, Login
// and Refresh all share. So registering announces a login, and so does every
// token rotation — a client refreshing on a timer would emit one per interval,
// and anything sending a welcome email would send one per interval.
func TestRefreshPublishesNothing(t *testing.T) {
	svc, _, publisher := newAuthServiceWithEvents(t)
	ctx := context.Background()

	reg, err := svc.Register(ctx, registerReq("refreshevent@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	before := len(publisher.events())

	if _, err := svc.Refresh(ctx, &dto.RefreshTokenRequest{RefreshToken: reg.RefreshToken}); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	published := publisher.events()
	if len(published) != before {
		t.Fatalf("refresh published %+v; rotating a token is not a login",
			published[before:])
	}
}

// Registering announces exactly one thing. The reference's Register fires
// USER_LOGGED_IN, because it shares a helper with Login.
func TestRegisterDoesNotPublishLoggedIn(t *testing.T) {
	svc, _, publisher := newAuthServiceWithEvents(t)

	if _, err := svc.Register(context.Background(), registerReq("once@example.com")); err != nil {
		t.Fatalf("Register: %v", err)
	}

	for _, e := range publisher.events() {
		if e.Type == events.TypeUserLoggedIn {
			t.Error("registering announced a login as well")
		}
	}
}

// The publisher is down. People must still be able to get in.
//
// The reference returns the publish error from the call that issues tokens, so
// an outage of the thing that sends the welcome email takes down registration
// and login with it.
func TestPublishFailureDoesNotBreakAuth(t *testing.T) {
	svc, _, publisher := newAuthServiceWithEvents(t)
	ctx := context.Background()

	publisher.failWith(errors.New("sqs is unreachable"))

	reg, err := svc.Register(ctx, registerReq("queuedown@example.com"))
	if err != nil {
		t.Fatalf("Register: %v — a queue outage must not stop registration", err)
	}
	if reg.AccessToken == "" {
		t.Error("Register returned no token")
	}

	login, err := svc.Login(ctx, &dto.LoginRequest{
		Email: "queuedown@example.com", Password: "correct-horse-battery",
	})
	if err != nil {
		t.Fatalf("Login: %v — a queue outage must not stop login", err)
	}
	if login.AccessToken == "" {
		t.Error("Login returned no token")
	}
}

// The event carries what a consumer needs and nothing else. The reference
// publishes the user model itself, so the queue's schema is the table's.
func TestUserEventCarriesNoSecrets(t *testing.T) {
	svc, _, publisher := newAuthServiceWithEvents(t)

	if _, err := svc.Register(context.Background(), registerReq("secrets@example.com")); err != nil {
		t.Fatalf("Register: %v", err)
	}

	published := publisher.events()
	if len(published) != 1 {
		t.Fatalf("got %d events, want 1", len(published))
	}

	// Serialised, because that is what goes on the wire.
	data, err := json.Marshal(published[0].Payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	body := strings.ToLower(string(data))
	for _, forbidden := range []string{"password", "hash", "$2a$", "token"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("event body contains %q: %s", forbidden, data)
		}
	}
}
