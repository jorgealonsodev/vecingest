// Package password implements Argon2id password hashing and the
// conditional length policy (design D-F, D-G; auth-credentials spec).
// golang.org/x/crypto/argon2 is a bare KDF: the PHC string encoding, the
// verification path, and crypto/subtle.ConstantTimeCompare are all
// implemented here.
package password

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Pinned Argon2id parameters (PRD §5.1; auth-credentials: Argon2id
// Password Hashing, gate item 1). These are the ONLY parameters Hash
// ever produces; Verify accepts any bounded parameters found in a stored
// hash so a parameter bump can be rolled out via rehash-on-login.
const (
	argon2Time    uint32 = 2
	argon2Memory  uint32 = 19456 // KiB
	argon2Threads uint8  = 1
	saltLen              = 16
	keyLen        uint32 = 32
)

// Bounds on parameters accepted from a STORED hash. A stored hash is, in
// principle, attacker-influenceable data (a compromised or malicious
// write path); an unbounded m/t/p would let a single verification
// attempt allocate gigabytes of memory or spin for an unbounded time --
// a denial-of-service primitive. These bounds are generous relative to
// the pinned parameters, but never unbounded.
const (
	maxMemoryKiB uint64 = 1 << 20 // 1 GiB
	maxTime      uint64 = 16
	maxThreads   uint64 = 16

	// maxTagLen bounds a stored hash's tag length. It is generous relative
	// to keyLen (32, the only length Hash ever produces) so a legacy
	// rehash-on-parameter-bump tag stays accepted, but it exists for two
	// concrete reasons, not gosec box-ticking: (1) a zero-length tag makes
	// argon2.IDKey's underlying blake2b hash dereference a nil buffer and
	// panic -- confirmed by TestVerify_BoundsCheckedAndMalformedHashesRejected,
	// which crashed the whole process before this bound existed; (2) an
	// unbounded tag length is passed directly as argon2.IDKey's keyLen
	// parameter (see Verify below), so a stored hash with an enormous tag
	// is a memory-exhaustion primitive exactly like the already-bounded
	// m/t/p parameters above.
	minTagLen        = 1
	maxTagLen uint64 = 1024
)

// DummyHash is a fixed, valid PHC string with the pinned parameters. It
// does not correspond to any real account. Its only purpose is to let
// VerifyAgainstDummy perform real Argon2id work when no stored hash
// exists, so an unknown-email login takes the same time as a real one
// and the response does not disclose whether the email is registered
// (auth-credentials: Enumeration-Safe Auth Responses; PRD §5.1).
const DummyHash = "$argon2id$v=19$m=19456,t=2,p=1$AQIDBAUGBwgJCgsMDQ4PEA$l/JyVVuBVRBthgGcJA7KioGWEAiWM7dyjaDsebKFB6w"

// Hash computes an Argon2id PHC string for password using the pinned
// parameters and a fresh 16-byte salt from crypto/rand.
func Hash(password string) (string, error) {
	return hashWithParams(password, argon2Time, argon2Memory, argon2Threads)
}

func hashWithParams(password string, time, memory uint32, threads uint8) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password: generate salt: %w", err)
	}
	tag := argon2.IDKey([]byte(password), salt, time, memory, threads, keyLen)
	return encodePHC(time, memory, threads, salt, tag), nil
}

func encodePHC(time, memory uint32, threads uint8, salt, tag []byte) string {
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory, time, threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(tag),
	)
}

type phcParams struct {
	time    uint32
	memory  uint32
	threads uint8
	salt    []byte
	tag     []byte
	// tagLen is uint32(len(tag)), computed and bounds-checked once here
	// (minTagLen..maxTagLen) rather than at every call site: it is the
	// exact value Verify passes to argon2.IDKey as keyLen, and it must
	// never reach that call unbounded (see maxTagLen's doc comment).
	tagLen uint32
}

// parsePHC strictly parses a PHC string: unknown algorithms/versions are
// rejected, and m/t/p are bounds-checked BEFORE any Argon2id work is
// attempted, precisely to keep a malformed or hostile stored hash from
// becoming a memory- or CPU-exhaustion vector.
func parsePHC(s string) (*phcParams, error) {
	parts := strings.Split(s, "$")
	// A well-formed PHC string splits into 6 parts on "$":
	// ["", "argon2id", "v=19", "m=...,t=...,p=...", salt, tag]
	if len(parts) != 6 {
		return nil, fmt.Errorf("password: malformed PHC string")
	}
	if parts[0] != "" {
		return nil, fmt.Errorf("password: malformed PHC string")
	}
	if parts[1] != "argon2id" {
		return nil, fmt.Errorf("password: unsupported algorithm %q", parts[1])
	}
	if parts[2] != "v=19" {
		return nil, fmt.Errorf("password: unsupported version %q", parts[2])
	}

	var m, t, p uint64
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return nil, fmt.Errorf("password: malformed parameter segment %q: %w", parts[3], err)
	}
	if m == 0 || m > maxMemoryKiB {
		return nil, fmt.Errorf("password: memory parameter out of bounds")
	}
	if t == 0 || t > maxTime {
		return nil, fmt.Errorf("password: time parameter out of bounds")
	}
	if p == 0 || p > maxThreads {
		return nil, fmt.Errorf("password: threads parameter out of bounds")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, fmt.Errorf("password: malformed salt: %w", err)
	}
	tag, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, fmt.Errorf("password: malformed tag: %w", err)
	}
	tagLen := uint64(len(tag))
	if tagLen < minTagLen || tagLen > maxTagLen {
		return nil, fmt.Errorf("password: tag length out of bounds")
	}

	return &phcParams{
		time:    uint32(t),
		memory:  uint32(m),
		threads: uint8(p),
		salt:    salt,
		tag:     tag,
		// Safe: tagLen was just bounds-checked above to
		// [minTagLen, maxTagLen], both small compile-time constants
		// nowhere near uint32's range.
		tagLen: uint32(tagLen), //nolint:gosec // G115: tagLen is bounds-checked immediately above (minTagLen..maxTagLen=1024), never a raw unchecked conversion
	}, nil
}

// Verify recomputes the Argon2id tag for candidate using the parameters
// and salt found in stored, and compares it against the stored tag with
// crypto/subtle.ConstantTimeCompare. needsRehash is true when stored
// used parameters other than the currently pinned ones -- the caller is
// expected to rehash and persist the new PHC string in the same
// transaction as a successful login (auth-credentials: scenario "Rehash
// on parameter bump"). A malformed or out-of-bounds stored hash always
// returns (false, false): it can never be a match, and it never needs
// "rehashing" because there is nothing valid to rehash from.
func Verify(_ context.Context, stored, candidate string) (ok, needsRehash bool) {
	params, err := parsePHC(stored)
	if err != nil {
		return false, false
	}

	computed := argon2.IDKey([]byte(candidate), params.salt, params.time, params.memory, params.threads, params.tagLen)
	if subtle.ConstantTimeCompare(computed, params.tag) != 1 {
		return false, false
	}

	needsRehash = params.time != argon2Time || params.memory != argon2Memory || params.threads != argon2Threads
	return true, needsRehash
}

// VerifyAgainstDummy runs a real Argon2id comparison against DummyHash
// and discards the result. Call this exactly where a real Verify call
// would otherwise happen for an email that does not exist, so the
// response timing cannot be used to enumerate registered accounts.
func VerifyAgainstDummy(ctx context.Context, candidate string) {
	_, _ = Verify(ctx, DummyHash, candidate)
}
