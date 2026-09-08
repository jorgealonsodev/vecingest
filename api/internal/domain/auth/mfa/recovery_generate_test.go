package mfa_test

import (
	"encoding/hex"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
)

// auth-mfa-totp: One-Time Recovery Codes -- "10 recovery codes, 128-bit,
// SHA-256 hashed".
func TestGenerateRecoveryCodes_ProducesTenDistinct128BitCodes(t *testing.T) {
	raw, hashed, err := mfa.GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes: unexpected error: %v", err)
	}
	if len(raw) != 10 || len(hashed) != 10 {
		t.Fatalf("expected 10 raw and 10 hashed codes, got %d and %d", len(raw), len(hashed))
	}

	seen := map[string]bool{}
	for i, code := range raw {
		if seen[code] {
			t.Fatalf("duplicate raw recovery code at index %d: %s", i, code)
		}
		seen[code] = true

		decoded, err := hex.DecodeString(code)
		if err != nil {
			t.Fatalf("raw code %q is not hex: %v", code, err)
		}
		if len(decoded) != 16 {
			t.Fatalf("expected a 128-bit (16-byte) code, got %d bytes", len(decoded))
		}
	}

	seenHash := map[string]bool{}
	for i, h := range hashed {
		if seenHash[h] {
			t.Fatalf("duplicate hashed recovery code at index %d", i)
		}
		seenHash[h] = true
		if want := mfa.HashRecoveryCode(raw[i]); want != h {
			t.Fatalf("hashed[%d] = %q, want HashRecoveryCode(raw[%d]) = %q", i, h, i, want)
		}
	}
}
