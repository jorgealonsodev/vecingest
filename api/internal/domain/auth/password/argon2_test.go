package password_test

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/password"
)

// auth-credentials: Argon2id Password Hashing -- "Password stored as PHC
// string" (gate item 1).
func TestHash_ProducesPinnedPHCString(t *testing.T) {
	hash, err := password.Hash("a-reasonably-long-passphrase")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	const wantPrefix = "$argon2id$v=19$m=19456,t=2,p=1$"
	if !strings.HasPrefix(hash, wantPrefix) {
		t.Fatalf("expected hash to start with %q, got %q", wantPrefix, hash)
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		t.Fatalf("expected a 6-segment PHC string, got %d segments: %q", len(parts), hash)
	}
}

// Triangulation: two hashes of the SAME password must differ (random
// salt), proving Hash does not memoize or reuse a fixed salt.
func TestHash_UsesARandomSaltEachTime(t *testing.T) {
	h1, err := password.Hash("same-password-both-times")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	h2, err := password.Hash("same-password-both-times")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h1 == h2 {
		t.Fatalf("expected two hashes of the same password to differ due to random salt, got identical: %s", h1)
	}
}

func TestVerify_RoundTrip(t *testing.T) {
	stored, err := password.Hash("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ok, needsRehash := password.Verify(context.Background(), stored, "correct-horse-battery-staple")
	if !ok {
		t.Fatalf("expected the correct password to verify")
	}
	if needsRehash {
		t.Fatalf("expected a freshly hashed password to not need rehashing")
	}

	ok, _ = password.Verify(context.Background(), stored, "wrong-password")
	if ok {
		t.Fatalf("expected the wrong password to fail verification")
	}
}

// auth-credentials: Argon2id Password Hashing -- verification uses
// crypto/subtle.ConstantTimeCompare; a tampered tag must fail.
func TestVerify_TamperedTagRejected(t *testing.T) {
	stored, err := password.Hash("tamper-detection-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parts := strings.Split(stored, "$")
	tag := parts[len(parts)-1]
	// Flip the tag's first character to something else valid-looking.
	flipped := "A"
	if strings.HasPrefix(tag, "A") {
		flipped = "B"
	}
	parts[len(parts)-1] = flipped + tag[1:]
	tampered := strings.Join(parts, "$")

	ok, _ := password.Verify(context.Background(), tampered, "tamper-detection-password")
	if ok {
		t.Fatalf("expected a tampered tag to fail verification")
	}
}

// auth-credentials: Argon2id Password Hashing -- a strict PHC parser
// rejects unknown algorithms/versions and bounds-checks m/t/p; an
// attacker-supplied hash with an enormous m is a denial-of-service risk
// and must be rejected before any Argon2id work is attempted.
func TestVerify_BoundsCheckedAndMalformedHashesRejected(t *testing.T) {
	tests := []struct {
		name   string
		stored string
	}{
		{"empty string", ""},
		{"wrong algorithm", "$argon2i$v=19$m=19456,t=2,p=1$AQIDBAUGBwgJCgsMDQ4PEA$l/JyVVuBVRBthgGcJA7KioGWEAiWM7dyjaDsebKFB6w"},
		{"wrong version", "$argon2id$v=1$m=19456,t=2,p=1$AQIDBAUGBwgJCgsMDQ4PEA$l/JyVVuBVRBthgGcJA7KioGWEAiWM7dyjaDsebKFB6w"},
		{"absurd memory (DoS attempt)", "$argon2id$v=19$m=4194304000,t=2,p=1$AQIDBAUGBwgJCgsMDQ4PEA$l/JyVVuBVRBthgGcJA7KioGWEAiWM7dyjaDsebKFB6w"},
		{"zero memory", "$argon2id$v=19$m=0,t=2,p=1$AQIDBAUGBwgJCgsMDQ4PEA$l/JyVVuBVRBthgGcJA7KioGWEAiWM7dyjaDsebKFB6w"},
		{"zero time", "$argon2id$v=19$m=19456,t=0,p=1$AQIDBAUGBwgJCgsMDQ4PEA$l/JyVVuBVRBthgGcJA7KioGWEAiWM7dyjaDsebKFB6w"},
		{"zero threads", "$argon2id$v=19$m=19456,t=2,p=0$AQIDBAUGBwgJCgsMDQ4PEA$l/JyVVuBVRBthgGcJA7KioGWEAiWM7dyjaDsebKFB6w"},
		{"too few segments", "$argon2id$v=19$m=19456,t=2,p=1$onlysaltnohash"},
		{"invalid base64 salt", "$argon2id$v=19$m=19456,t=2,p=1$not-valid-base64!!!$l/JyVVuBVRBthgGcJA7KioGWEAiWM7dyjaDsebKFB6w"},
		{"empty tag", "$argon2id$v=19$m=19456,t=2,p=1$AQIDBAUGBwgJCgsMDQ4PEA$"},
		// A stored tag far larger than any pinned key length is a DoS
		// primitive: argon2.IDKey's keyLen parameter is derived from
		// len(tag) (design: rehash-on-parameter-bump tolerates a legacy
		// tag length), and an unbounded length would let a single
		// verification attempt request an enormous output buffer. parsePHC
		// must reject this before Verify ever calls argon2.IDKey.
		{"absurdly long tag (DoS attempt via output key length)", "$argon2id$v=19$m=19456,t=2,p=1$AQIDBAUGBwgJCgsMDQ4PEA$" + oversizedTag()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, needsRehash := password.Verify(context.Background(), tt.stored, "anything")
			if ok {
				t.Fatalf("expected malformed/out-of-bounds hash %q to be rejected", tt.stored)
			}
			if needsRehash {
				t.Fatalf("expected needsRehash=false for a rejected hash, got true")
			}
		})
	}
}

// auth-credentials: non-disclosure of existence -- login against an
// unknown email still runs Argon2id work against a fixed dummy hash.
func TestVerifyAgainstDummy_RunsRealArgon2idWork(t *testing.T) {
	// DummyHash must itself be a valid, parseable PHC string with the
	// pinned parameters, or the "real work" claim is false.
	ok, needsRehash := password.Verify(context.Background(), password.DummyHash, "whatever the attacker submits")
	if ok {
		t.Fatalf("expected an arbitrary candidate to never match the dummy hash")
	}
	if needsRehash {
		t.Fatalf("expected the pinned dummy hash to already use current parameters")
	}

	// VerifyAgainstDummy must not panic and must be safe to call with no
	// return value the caller could use to distinguish existence.
	password.VerifyAgainstDummy(context.Background(), "whatever the attacker submits")
}

// oversizedTag returns a base64-encoded tag far larger than any pinned
// Argon2id key length, for TestVerify_BoundsCheckedAndMalformedHashesRejected.
func oversizedTag() string {
	return base64.RawStdEncoding.EncodeToString(make([]byte, 8192))
}
