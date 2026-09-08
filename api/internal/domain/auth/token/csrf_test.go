package token_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
)

func mustDeriveKey(t *testing.T, secret string) []byte {
	t.Helper()
	key, err := token.DeriveCSRFKey([]byte(secret))
	if err != nil {
		t.Fatalf("DeriveCSRFKey: unexpected error: %v", err)
	}
	return key
}

// auth-csrf-origin: CSRF Token Delivered In-Body -- "Valid cookie and
// valid CSRF header" (domain leg: mint then verify round-trips).
func TestMintAndVerifyCSRF_RoundTrips(t *testing.T) {
	key := mustDeriveKey(t, "JWT_REFRESH_SECRET-at-least-32-bytes-long")
	familyID := uuid.New()
	sessionID := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	exp := now.Add(30 * 24 * time.Hour)

	tok, err := token.MintCSRF(key, familyID, sessionID, exp)
	if err != nil {
		t.Fatalf("MintCSRF: unexpected error: %v", err)
	}
	if err := token.VerifyCSRF(key, tok, familyID, sessionID, now); err != nil {
		t.Fatalf("VerifyCSRF: unexpected error for a freshly minted token: %v", err)
	}
}

func TestVerifyCSRF_RejectsForgedToken(t *testing.T) {
	key := mustDeriveKey(t, "JWT_REFRESH_SECRET-at-least-32-bytes-long")
	familyID, sessionID := uuid.New(), uuid.New()
	now := time.Now()

	otherKey := mustDeriveKey(t, "a-totally-different-refresh-secret-value")
	forged, err := token.MintCSRF(otherKey, familyID, sessionID, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("MintCSRF: unexpected error: %v", err)
	}

	if err := token.VerifyCSRF(key, forged, familyID, sessionID, now); err == nil {
		t.Fatalf("VerifyCSRF: expected an error for a forged token, got nil")
	}
}

func TestVerifyCSRF_RejectsExpiredToken(t *testing.T) {
	key := mustDeriveKey(t, "JWT_REFRESH_SECRET-at-least-32-bytes-long")
	familyID, sessionID := uuid.New(), uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tok, err := token.MintCSRF(key, familyID, sessionID, now.Add(-time.Second))
	if err != nil {
		t.Fatalf("MintCSRF: unexpected error: %v", err)
	}
	if err := token.VerifyCSRF(key, tok, familyID, sessionID, now); err == nil {
		t.Fatalf("VerifyCSRF: expected an error for an expired token, got nil")
	}
}

// D-E: the token is bound to family_id AND session_id, so rotation
// (which changes session_id within the same family) invalidates the
// previous token by construction.
func TestVerifyCSRF_RejectsFamilyMismatch(t *testing.T) {
	key := mustDeriveKey(t, "JWT_REFRESH_SECRET-at-least-32-bytes-long")
	familyID, sessionID := uuid.New(), uuid.New()
	now := time.Now()
	tok, err := token.MintCSRF(key, familyID, sessionID, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("MintCSRF: unexpected error: %v", err)
	}

	otherFamily := uuid.New()
	if err := token.VerifyCSRF(key, tok, otherFamily, sessionID, now); err == nil {
		t.Fatalf("VerifyCSRF: expected an error for a family_id mismatch, got nil")
	}
}

func TestVerifyCSRF_RejectsSessionMismatchAfterRotation(t *testing.T) {
	key := mustDeriveKey(t, "JWT_REFRESH_SECRET-at-least-32-bytes-long")
	familyID := uuid.New()
	oldSessionID := uuid.New()
	newSessionID := uuid.New()
	now := time.Now()

	tok, err := token.MintCSRF(key, familyID, oldSessionID, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("MintCSRF: unexpected error: %v", err)
	}

	// After rotation the session_id the cookie hashes to has changed;
	// the old CSRF token must no longer verify against it.
	if err := token.VerifyCSRF(key, tok, familyID, newSessionID, now); err == nil {
		t.Fatalf("VerifyCSRF: expected an error after rotation changed session_id, got nil")
	}
}

func TestVerifyCSRF_RejectsMalformedToken(t *testing.T) {
	key := mustDeriveKey(t, "JWT_REFRESH_SECRET-at-least-32-bytes-long")
	if err := token.VerifyCSRF(key, "not-a-csrf-token", uuid.New(), uuid.New(), time.Now()); err == nil {
		t.Fatalf("VerifyCSRF: expected an error for a malformed token, got nil")
	}
}

// D-D: the CSRF key is derived from JWT_REFRESH_SECRET via
// HKDF-SHA256, so the same secret always derives the same key and two
// different secrets never collide.
func TestDeriveCSRFKey_IsDeterministicAndSecretSpecific(t *testing.T) {
	key1, err := token.DeriveCSRFKey([]byte("secret-a-at-least-32-bytes-long"))
	if err != nil {
		t.Fatalf("DeriveCSRFKey: unexpected error: %v", err)
	}
	key1Again, err := token.DeriveCSRFKey([]byte("secret-a-at-least-32-bytes-long"))
	if err != nil {
		t.Fatalf("DeriveCSRFKey: unexpected error: %v", err)
	}
	if string(key1) != string(key1Again) {
		t.Fatalf("DeriveCSRFKey is not deterministic for the same secret")
	}

	key2, err := token.DeriveCSRFKey([]byte("secret-b-at-least-32-bytes-long"))
	if err != nil {
		t.Fatalf("DeriveCSRFKey: unexpected error: %v", err)
	}
	if string(key1) == string(key2) {
		t.Fatalf("DeriveCSRFKey produced identical keys for different secrets")
	}
}
