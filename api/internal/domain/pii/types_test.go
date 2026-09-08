package pii_test

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/domain/pii"
)

// D-J leg 3: typed PII survives a forgetful call site -- these types
// implement slog.LogValuer so a bare slog.Any("x", value) call redacts
// even when the key itself is not one of slogpii's known sensitive keys.
func TestTypedPII_LogValueRedacts(t *testing.T) {
	tests := []struct {
		name string
		v    slog.LogValuer
	}{
		{"Email", pii.Email("user@example.com")},
		{"Phone", pii.Phone("612345678")},
		{"IBAN", pii.IBAN("ES9121000418450200051332")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.v.LogValue().String()
			if got == string(tt.v.LogValue().String()) && got != "[redacted]" {
				t.Fatalf("expected LogValue() to redact, got %q", got)
			}
		})
	}
}

func TestTypedPII_StringerRedacts(t *testing.T) {
	tests := []struct {
		name string
		v    fmt.Stringer
	}{
		{"Email", pii.Email("user@example.com")},
		{"Phone", pii.Phone("612345678")},
		{"IBAN", pii.IBAN("ES9121000418450200051332")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.String(); got != "[redacted]" {
				t.Fatalf("expected String() to redact, got %q", got)
			}
		})
	}
}
