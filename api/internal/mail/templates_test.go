package mail_test

import (
	"strings"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/mail"
)

// design.md:473 mandates password_reset.html and login_lockout.html;
// these tests are the RED that existed before either template did.
func TestRenderPasswordReset_IncludesToken(t *testing.T) {
	subject, body, err := mail.RenderPasswordReset(mail.PasswordResetData{RawToken: "abc123token"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if subject != mail.PasswordResetSubject {
		t.Fatalf("expected subject %q, got %q", mail.PasswordResetSubject, subject)
	}
	if !strings.Contains(body, "abc123token") {
		t.Fatalf("expected rendered body to contain the raw token, got: %s", body)
	}
	if !strings.Contains(body, "<html") {
		t.Fatalf("expected an HTML body, got: %s", body)
	}
}

// html/template auto-escapes: a token containing HTML-significant
// characters must never inject markup into the rendered email.
func TestRenderPasswordReset_EscapesToken(t *testing.T) {
	_, body, err := mail.RenderPasswordReset(mail.PasswordResetData{RawToken: "<script>evil()</script>"}) //nolint:gosec // G101: a test fixture string, not a credential value
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(body, "<script>") {
		t.Fatalf("expected the token to be HTML-escaped, got: %s", body)
	}
}

func TestRenderLoginLockoutAlert_IncludesDomainAuthoredMessage(t *testing.T) {
	body, err := mail.RenderLoginLockoutAlert(mail.LoginLockoutData{
		Message: "We blocked several failed sign-in attempts on your account.",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(body, "We blocked several failed sign-in attempts on your account.") {
		t.Fatalf("expected rendered body to contain the domain-authored message, got: %s", body)
	}
}
