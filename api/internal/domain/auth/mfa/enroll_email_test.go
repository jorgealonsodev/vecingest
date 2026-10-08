package mfa_test

import (
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
)

// auth-mfa-totp: Email-Confirmed Enrollment -- the code is six decimal
// digits, leading zeros kept.
func TestGenerateEnrollEmailCode_IsSixDigits(t *testing.T) {
	for range 200 {
		code, err := mfa.GenerateEnrollEmailCode()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(code) != 6 {
			t.Fatalf("expected 6 characters, got %q", code)
		}
		for _, r := range code {
			if r < '0' || r > '9' {
				t.Fatalf("expected only decimal digits, got %q", code)
			}
		}
	}
}

func TestEnrollEmailCodeMatches_IsKeyedAndExact(t *testing.T) {
	key := [32]byte{1}
	hash := mfa.HashEnrollEmailCode(key, "123456")
	if !mfa.EnrollEmailCodeMatches(key, "123456", hash) {
		t.Fatalf("expected the issued code to match its own digest")
	}
	if mfa.EnrollEmailCodeMatches(key, "123457", hash) {
		t.Fatalf("expected a different code not to match")
	}
	if mfa.EnrollEmailCodeMatches([32]byte{2}, "123456", hash) {
		t.Fatalf("expected the digest to be bound to the key")
	}
}
