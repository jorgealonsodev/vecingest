package password_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/password"
)

type fakeUserLookup struct {
	userID uuid.UUID
	found  bool
}

func (f fakeUserLookup) LookupByEmail(_ context.Context, _ string) (uuid.UUID, bool, error) {
	return f.userID, f.found, nil
}

type fakeTokenIssuer struct{}

func (fakeTokenIssuer) GenerateOpaqueToken() (string, []byte, error) {
	return "raw-token", []byte("hashed-token"), nil
}

type fakeResetRequester struct {
	mu       sync.Mutex
	called   bool
	gotEmail string
}

func (f *fakeResetRequester) RequestReset(_ context.Context, _ uuid.UUID, email, _ string, _ []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called = true
	f.gotEmail = email
	return nil
}

func (f *fakeResetRequester) wasCalled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.called
}

func (f *fakeResetRequester) calledWithEmail() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gotEmail
}

func syncDispatch(fn func()) { fn() }

// auth-credentials: Enumeration-Safe Auth Responses -- "Unknown email on
// forgot-password".
func TestForgotPassword_IdenticalResponseForUnregisteredEmail(t *testing.T) {
	requester := &fakeResetRequester{}
	svc := password.ForgotPasswordService{
		Users:     fakeUserLookup{found: false},
		Tokens:    fakeTokenIssuer{},
		Requester: requester,
		Dispatch:  syncDispatch,
	}

	result, err := svc.ForgotPassword(context.Background(), "unregistered@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Accepted {
		t.Fatalf("expected Accepted=true for an unregistered email, got %+v", result)
	}
	if requester.wasCalled() {
		t.Fatalf("expected no reset to be requested for an unregistered email")
	}
}

func TestForgotPassword_IdenticalResponseForRegisteredEmail(t *testing.T) {
	requester := &fakeResetRequester{}
	svc := password.ForgotPasswordService{
		Users:     fakeUserLookup{userID: uuid.New(), found: true},
		Tokens:    fakeTokenIssuer{},
		Requester: requester,
		Dispatch:  syncDispatch,
	}

	result, err := svc.ForgotPassword(context.Background(), "registered@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Accepted {
		t.Fatalf("expected Accepted=true for a registered email, got %+v", result)
	}
	if !requester.wasCalled() {
		t.Fatalf("expected a reset to be requested for a registered email")
	}
	if requester.calledWithEmail() != "registered@example.com" {
		t.Fatalf("expected RequestReset to receive the looked-up email, got %q", requester.calledWithEmail())
	}
}

// A real SMTP sender needs the recipient address to actually deliver
// the reset email; ResetRequester.RequestReset previously had no way
// to learn it at all (see verify-report.md CRITICAL-2 remediation).
func TestForgotPassword_PassesEmailToResetRequester(t *testing.T) {
	requester := &fakeResetRequester{}
	svc := password.ForgotPasswordService{
		Users:     fakeUserLookup{userID: uuid.New(), found: true},
		Tokens:    fakeTokenIssuer{},
		Requester: requester,
		Dispatch:  syncDispatch,
	}

	if _, err := svc.ForgotPassword(context.Background(), "someone@example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := requester.calledWithEmail(); got != "someone@example.com" {
		t.Fatalf("expected RequestReset to receive %q, got %q", "someone@example.com", got)
	}
}

func TestForgotPassword_ResponseShapeIsIdenticalRegardlessOfExistence(t *testing.T) {
	unknown := password.ForgotPasswordService{
		Users: fakeUserLookup{found: false}, Tokens: fakeTokenIssuer{},
		Requester: &fakeResetRequester{}, Dispatch: syncDispatch,
	}
	known := password.ForgotPasswordService{
		Users: fakeUserLookup{userID: uuid.New(), found: true}, Tokens: fakeTokenIssuer{},
		Requester: &fakeResetRequester{}, Dispatch: syncDispatch,
	}

	r1, err1 := unknown.ForgotPassword(context.Background(), "a@example.com")
	r2, err2 := known.ForgotPassword(context.Background(), "b@example.com")
	if err1 != err2 {
		t.Fatalf("errors differ: %v vs %v", err1, err2)
	}
	if r1 != r2 {
		t.Fatalf("response shapes differ: %+v vs %+v", r1, r2)
	}
}
