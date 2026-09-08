package token

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/hkdf"
)

// csrfInfo is the HKDF context string binding the derived key to this
// exact use (D-D): k = HKDF-SHA256(JWT_REFRESH_SECRET, info="vecingest/csrf/v1").
const csrfInfo = "vecingest/csrf/v1"

// csrfNonceLen is the random-nonce length inside the token payload.
const csrfNonceLen = 16

var (
	// ErrCSRFMalformed is returned for a token that cannot even be
	// decoded into a payload/signature pair.
	ErrCSRFMalformed = errors.New("csrf: malformed token")
	// ErrCSRFInvalid is returned when the HMAC does not verify -- a
	// forged token, or one whose family_id/session_id binding does not
	// match what the caller expects (D-E).
	ErrCSRFInvalid = errors.New("csrf: invalid signature")
	// ErrCSRFExpired is returned when the token's embedded expiry has
	// passed.
	ErrCSRFExpired = errors.New("csrf: expired")
)

// DeriveCSRFKey derives the CSRF signing key from JWT_REFRESH_SECRET via
// HKDF-SHA256 (D-D). M0 refresh tokens are opaque random values, so this
// keeps JWT_REFRESH_SECRET's PRD §8.2 variable list unchanged rather
// than adding a dedicated CSRF secret.
func DeriveCSRFKey(refreshSecret []byte) ([]byte, error) {
	kdf := hkdf.New(sha256.New, refreshSecret, nil, []byte(csrfInfo))
	key := make([]byte, sha256.Size)
	if _, err := io.ReadFull(kdf, key); err != nil {
		return nil, fmt.Errorf("token: derive csrf key: %w", err)
	}
	return key, nil
}

// MintCSRF issues a signed double-submit CSRF token bound to familyID,
// sessionID and exp (D-D/D-E). The token is
// base64url(nonce‖exp) + "." + base64url(HMAC-SHA256(key, info‖nonce‖family_id‖session_id‖exp)).
// family_id and session_id are folded into the signature only, never
// into the payload: verification requires the caller to already know
// which session it expects (the row the refresh cookie's hash resolved
// to), which is what makes the session_id binding invalidate a stale
// token the instant rotation produces a new session_id.
func MintCSRF(key []byte, familyID, sessionID uuid.UUID, exp time.Time) (string, error) {
	nonce := make([]byte, csrfNonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("token: mint csrf: %w", err)
	}
	payload := encodeCSRFPayload(nonce, exp)
	sig := signCSRF(key, nonce, familyID, sessionID, exp)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// VerifyCSRF validates a token minted by MintCSRF against the exact
// familyID and sessionID the caller expects, and rejects it once exp has
// passed. Any failure is reported as one of the sentinel errors above;
// the HTTP layer (Phase 5) maps all of them to 403 AUTH_CSRF_INVALID
// except ErrCSRFExpired, which is also a 403 per D-E -- the distinction
// exists for logging, not for a different status code.
func VerifyCSRF(key []byte, raw string, familyID, sessionID uuid.UUID, now time.Time) error {
	payloadB64, sigB64, ok := strings.Cut(raw, ".")
	if !ok {
		return ErrCSRFMalformed
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil || len(payload) != csrfNonceLen+8 {
		return ErrCSRFMalformed
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return ErrCSRFMalformed
	}

	nonce := payload[:csrfNonceLen]
	exp := decodeCSRFExpiry(payload[csrfNonceLen:])

	expected := signCSRF(key, nonce, familyID, sessionID, exp)
	if !hmac.Equal(sig, expected) {
		return ErrCSRFInvalid
	}
	if !now.Before(exp) {
		return ErrCSRFExpired
	}
	return nil
}

func encodeCSRFPayload(nonce []byte, exp time.Time) []byte {
	buf := make([]byte, 0, csrfNonceLen+8)
	buf = append(buf, nonce...)
	return binary.BigEndian.AppendUint64(buf, uint64(exp.Unix())) //nolint:gosec // exp is always a near-future timestamp, never negative or beyond int64 range
}

func decodeCSRFExpiry(b []byte) time.Time {
	return time.Unix(int64(binary.BigEndian.Uint64(b)), 0).UTC() //nolint:gosec // round-trips a value this package itself encoded via encodeCSRFPayload
}

func signCSRF(key, nonce []byte, familyID, sessionID uuid.UUID, exp time.Time) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(csrfInfo))
	mac.Write(nonce)
	mac.Write(familyID[:])
	mac.Write(sessionID[:])
	expBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(expBuf, uint64(exp.Unix())) //nolint:gosec // exp is always a near-future timestamp, never negative or beyond int64 range
	mac.Write(expBuf)
	return mac.Sum(nil)
}
