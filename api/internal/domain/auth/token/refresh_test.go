package token_test

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
)

// auth-session-tokens: JWT Access and Refresh Issuance -- refresh
// lifetimes (30 days normally, 8 hours for superadmin, PRD §6.1).
func TestRefreshLifetime(t *testing.T) {
	tests := []struct {
		name         string
		isSuperadmin bool
		want         time.Duration
	}{
		{"non-superadmin gets 30 days", false, 30 * 24 * time.Hour},
		{"superadmin gets 8 hours", true, 8 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := token.RefreshLifetime(tt.isSuperadmin)
			if got != tt.want {
				t.Errorf("RefreshLifetime(%v) = %v, want %v", tt.isSuperadmin, got, tt.want)
			}
		})
	}
}

// GenerateRefresh produces 32 opaque random bytes, base64url-encoded --
// not a JWT (D-D).
func TestGenerateRefresh_ProducesDistinctOpaqueTokens(t *testing.T) {
	raw1, hash1, err := token.GenerateRefresh()
	if err != nil {
		t.Fatalf("GenerateRefresh: unexpected error: %v", err)
	}
	raw2, hash2, err := token.GenerateRefresh()
	if err != nil {
		t.Fatalf("GenerateRefresh: unexpected error: %v", err)
	}
	if raw1 == raw2 {
		t.Fatalf("expected two distinct raw tokens, got identical: %s", raw1)
	}
	if string(hash1) == string(hash2) {
		t.Fatalf("expected two distinct hashes, got identical")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw1)
	if err != nil {
		t.Fatalf("raw token is not valid base64url: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("expected 32 raw bytes, got %d", len(decoded))
	}
	if len(hash1) != 32 {
		t.Fatalf("expected a 32-byte SHA-256 hash, got %d bytes", len(hash1))
	}
}

// auth-session-tokens: Hashed Storage of Sensitive Tokens -- the raw
// value must never be recoverable from what gets persisted, and hashing
// is deterministic so a lookup by hash works.
func TestHashToken_IsDeterministicAndNeverEqualsRaw(t *testing.T) {
	raw, hash, err := token.GenerateRefresh()
	if err != nil {
		t.Fatalf("GenerateRefresh: unexpected error: %v", err)
	}
	rehash := token.HashToken(raw)
	if string(rehash) != string(hash) {
		t.Fatalf("HashToken(raw) is not deterministic: %x != %x", rehash, hash)
	}
	if string(rehash) == raw {
		t.Fatalf("hash must never equal the raw token")
	}
}
