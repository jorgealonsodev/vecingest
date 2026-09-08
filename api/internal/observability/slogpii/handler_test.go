package slogpii_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/observability/slogpii"
)

func newLogger(buf *bytes.Buffer) *slog.Logger {
	jsonHandler := slog.NewJSONHandler(buf, &slog.HandlerOptions{ReplaceAttr: slogpii.ReplaceAttr})
	return slog.New(slogpii.Wrap(jsonHandler))
}

// request-protection: slog PII Redaction — "Structured attribute
// redacted" scenario.
func TestHandler_RedactsStructuredEmailAttribute(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf)

	logger.Info("login attempt", slog.String("email", "user@example.com"))

	out := buf.String()
	if strings.Contains(out, "user@example.com") {
		t.Fatalf("expected the literal email to be redacted, got: %s", out)
	}
}

// request-protection: slog PII Redaction — "Interpolated message
// redacted" scenario. ReplaceAttr never sees the message string, which
// is exactly why the wrapping Handler leg is required (D-J).
func TestHandler_RedactsEmailInterpolatedIntoMessage(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf)

	logger.Info("user user@example.com logged in")

	out := buf.String()
	if strings.Contains(out, "user@example.com") {
		t.Fatalf("expected the literal email in the message to be redacted, got: %s", out)
	}
}

func TestHandler_RedactsPhoneAndIBANInMessage(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf)

	logger.Info("contact 612345678 iban ES9121000418450200051332")

	out := buf.String()
	if strings.Contains(out, "612345678") {
		t.Fatalf("expected phone to be redacted, got: %s", out)
	}
	if strings.Contains(out, "ES9121000418450200051332") {
		t.Fatalf("expected IBAN to be redacted, got: %s", out)
	}
}

func TestHandler_RedactsSensitiveKeyRegardlessOfCase(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf)

	logger.Info("token issued", slog.String("Authorization", "Bearer secret-token-value"))

	out := buf.String()
	if strings.Contains(out, "secret-token-value") {
		t.Fatalf("expected the Authorization value to be redacted, got: %s", out)
	}
}

func TestHandler_RedactsNestedGroupAttributes(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf)

	logger.Info("request", slog.Group("user", slog.String("email", "nested@example.com")))

	out := buf.String()
	if strings.Contains(out, "nested@example.com") {
		t.Fatalf("expected nested group email to be redacted, got: %s", out)
	}
}

// leg 3 (D-J): typed PII survives a forgetful call site that never named
// the attribute key "email"/"phone"/"iban".
func TestHandler_RedactsLogValuerEvenWithGenericKey(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf)

	logger.Info("profile updated", slog.Any("contact", piiEmail("typed@example.com")))

	out := buf.String()
	if strings.Contains(out, "typed@example.com") {
		t.Fatalf("expected a LogValuer-typed value to redact itself, got: %s", out)
	}
}

type piiEmail string

func (e piiEmail) LogValue() slog.Value { return slog.StringValue("[redacted:email]") }
