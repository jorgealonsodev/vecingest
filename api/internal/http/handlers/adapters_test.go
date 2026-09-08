package handlers_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/lockout"
	"github.com/jorgealonsodev/vecingest/internal/http/handlers"
	"github.com/jorgealonsodev/vecingest/internal/mail"
)

type capturingSender struct {
	to, subject, body string
}

func (s *capturingSender) SendRaw(_ context.Context, to, subject, body string) error {
	s.to, s.subject, s.body = to, subject, body
	return nil
}

// LockoutMailer.Send must wrap lockout.Message's domain-authored body
// in the login_lockout.html template (design.md:473, D-N "the alert"),
// while leaving the subject the domain layer already decided on.
func TestLockoutMailer_Send_RendersLoginLockoutTemplate(t *testing.T) {
	sender := &capturingSender{}
	adapter := handlers.LockoutMailer{Sender: sender}

	msg := lockout.Message{
		To:      "victim@example.com",
		Subject: "Unusual sign-in activity on your account",
		Body:    "We blocked several failed sign-in attempts on your account.",
	}

	if err := adapter.Send(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sender.to != "victim@example.com" {
		t.Fatalf("expected recipient %q, got %q", "victim@example.com", sender.to)
	}
	if sender.subject != msg.Subject {
		t.Fatalf("expected subject %q, got %q", msg.Subject, sender.subject)
	}
	if !strings.Contains(sender.body, msg.Body) {
		t.Fatalf("expected rendered body to contain the domain-authored message, got: %s", sender.body)
	}
	if !strings.Contains(sender.body, "<html") {
		t.Fatalf("expected rendered body to be HTML, got: %s", sender.body)
	}
}

// DBResetRequester.RequestReset's DB write and full send path (real
// recipient email, real password_reset.html render) is exercised by
// the Testcontainers integration suite (api_integration_test.go),
// since it needs a real WriteDB -- see the two tests below for the
// pure rendering/recipient-wiring pieces this test can isolate without
// a database.

// mail.RenderPasswordReset is exercised directly to prove the
// template/subject constant DBResetRequester relies on stays in sync.
func TestRenderPasswordReset_MatchesDBResetRequesterExpectations(t *testing.T) {
	subject, body, err := mail.RenderPasswordReset(mail.PasswordResetData{RawToken: "reset-tok-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if subject != mail.PasswordResetSubject {
		t.Fatalf("expected subject %q, got %q", mail.PasswordResetSubject, subject)
	}
	if !strings.Contains(body, "reset-tok-123") {
		t.Fatalf("expected rendered body to contain the token, got: %s", body)
	}
}
