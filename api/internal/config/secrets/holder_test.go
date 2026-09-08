package secrets_test

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/config/secrets"
)

func TestHolder_RedactsOnEveryLeg(t *testing.T) {
	h := secrets.NewHolder(secrets.Values{
		JWTSecret:     "super-secret-value",
		EncryptionKey: []byte("0123456789012345678901234567890"),
	})

	if got := h.String(); strings.Contains(got, "super-secret-value") {
		t.Fatalf("String() leaked the secret: %s", got)
	}

	b, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}
	if strings.Contains(string(b), "super-secret-value") {
		t.Fatalf("MarshalJSON leaked the secret: %s", b)
	}

	val := h.LogValue()
	if val.Kind() != slog.KindString || strings.Contains(val.String(), "super-secret-value") {
		t.Fatalf("LogValue leaked the secret: %v", val)
	}

	// Triangulation: embedding the holder in a larger struct being
	// logged/marshaled must not leak the secret either.
	type wrapper struct {
		Secrets *secrets.Holder `json:"secrets"`
		Public  string          `json:"public"`
	}
	wb, err := json.Marshal(wrapper{Secrets: h, Public: "ok"})
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}
	if strings.Contains(string(wb), "super-secret-value") {
		t.Fatalf("embedded holder leaked the secret: %s", wb)
	}
}

func TestHolder_AccessorsReturnStoredValues(t *testing.T) {
	h := secrets.NewHolder(secrets.Values{
		JWTSecret:         "a",
		JWTSecretPrevious: "b",
		JWTRefreshSecret:  "c",
		DatabaseURL:       "postgres://x",
		AppDBPassword:     "pw",
	})

	if h.JWTSecret() != "a" || h.JWTSecretPrevious() != "b" || h.JWTRefreshSecret() != "c" {
		t.Fatalf("JWT accessors did not round-trip")
	}
	if h.DatabaseURL() != "postgres://x" || h.AppDBPassword() != "pw" {
		t.Fatalf("DB accessors did not round-trip")
	}
}
