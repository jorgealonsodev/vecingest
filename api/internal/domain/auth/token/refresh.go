package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"
)

const (
	// RefreshLifetimeDefault is the 30-day refresh-token lifetime for
	// every non-superadmin account.
	RefreshLifetimeDefault = 30 * 24 * time.Hour
	// RefreshLifetimeSuperadmin is the shortened 8-hour refresh-token
	// lifetime for `superadmin` accounts (PRD §6.1).
	RefreshLifetimeSuperadmin = 8 * time.Hour

	// refreshRawByteLen is the number of raw random bytes in a refresh
	// token (D-D: "32 opaque bytes from crypto/rand").
	refreshRawByteLen = 32
)

// RefreshLifetime returns the applicable refresh-token lifetime for the
// given account (auth-session-tokens: JWT Access and Refresh Issuance).
func RefreshLifetime(isSuperadmin bool) time.Duration {
	if isSuperadmin {
		return RefreshLifetimeSuperadmin
	}
	return RefreshLifetimeDefault
}

// GenerateRefresh returns a new opaque refresh token: raw is the
// base64url-encoded value handed to the client, and hash is its
// SHA-256 digest -- the only form that may ever reach storage or a log
// (auth-session-tokens: Hashed Storage of Sensitive Tokens).
func GenerateRefresh() (raw string, hash []byte, err error) {
	buf := make([]byte, refreshRawByteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("token: generate refresh: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashToken(raw), nil
}

// GenerateOpaqueToken is GenerateRefresh under a name that does not
// imply "refresh token": password-reset and invitation tokens use the
// identical 32-byte-random/SHA-256-hash construction (auth-session-tokens:
// Hashed Storage of Sensitive Tokens), so this is the shared entry point
// production wiring uses to satisfy password.TokenIssuer.
func GenerateOpaqueToken() (raw string, hash []byte, err error) {
	return GenerateRefresh()
}

// HashToken returns the SHA-256 digest of raw. It is the single hashing
// primitive for refresh, password-reset and OTP-challenge tokens, all
// of which MUST be stored hashed and never in cleartext
// (auth-session-tokens: Hashed Storage of Sensitive Tokens).
func HashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}
