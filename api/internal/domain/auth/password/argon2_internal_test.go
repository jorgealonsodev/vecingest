package password

import (
	"context"
	"testing"
)

// auth-credentials: Argon2id Password Hashing -- rehash on parameter
// bump: a stored hash with outdated (but otherwise valid) parameters
// must verify successfully AND report needsRehash=true. This is a
// white-box test (package password, not password_test) because it needs
// encodePHC directly to construct a hash with non-pinned parameters --
// something the public API deliberately never lets a caller do.
func TestVerify_NeedsRehashWhenStoredParamsDiffer(t *testing.T) {
	outdated, err := hashWithParams("outdated-params-password", 1, 8192, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ok, needsRehash := Verify(context.Background(), outdated, "outdated-params-password")
	if !ok {
		t.Fatalf("expected the correct password against outdated params to still verify")
	}
	if !needsRehash {
		t.Fatalf("expected needsRehash=true when stored params differ from the pinned ones")
	}
}

// Triangulation: matching params must NOT need a rehash.
func TestVerify_NoRehashWhenStoredParamsMatchPinned(t *testing.T) {
	current, err := hashWithParams("current-params-password", argon2Time, argon2Memory, argon2Threads)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ok, needsRehash := Verify(context.Background(), current, "current-params-password")
	if !ok {
		t.Fatalf("expected the correct password to verify")
	}
	if needsRehash {
		t.Fatalf("expected needsRehash=false when stored params match the pinned ones")
	}
}
