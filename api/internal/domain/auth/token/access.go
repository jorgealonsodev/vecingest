// Package token implements the M0 access, refresh and CSRF token model
// (design D-D, D-E). It has no database dependency: issuance and
// verification are pure functions over injected secrets and an injected
// Clock, so every scenario here is unit-testable without a container.
package token

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AccessTTL is the fixed 15-minute access-token lifetime (PRD §5.1;
// auth-session-tokens: JWT Access and Refresh Issuance).
const AccessTTL = 15 * time.Minute

// Clock abstracts time.Now() so tests never depend on the wall clock.
type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// Claims are the JWT access-token claims (D-D): sub, sid (= family_id),
// sa (is_superadmin), iat, exp and jti, carried by jwt.RegisteredClaims
// plus the two vecingest-specific fields.
type Claims struct {
	jwt.RegisteredClaims
	SID string `json:"sid"`
	SA  bool   `json:"sa"`
}

var (
	// ErrUnknownKID is returned when a token's kid header matches
	// neither JWT_SECRET nor JWT_SECRET_PREVIOUS.
	ErrUnknownKID = errors.New("token: unrecognized kid")
	// ErrInvalidToken is returned for a syntactically valid JWT whose
	// signature or claims do not validate.
	ErrInvalidToken = errors.New("token: invalid access token")
)

// Issuer issues and verifies access tokens. Secret is always required;
// PreviousSecret is optional and, when set, is accepted for
// verification only -- never for issuance -- so a secret rotation can
// still validate tokens minted under the old secret until they expire
// (D-D, §7.7). Clock defaults to the system clock when nil.
type Issuer struct {
	Secret         []byte
	PreviousSecret []byte
	Clock          Clock
}

func (iss Issuer) clock() Clock {
	if iss.Clock != nil {
		return iss.Clock
	}
	return systemClock{}
}

// kidFor derives a short, stable, non-reversible fingerprint of secret
// to use as the JWT "kid" header, so verification can select the right
// secret without either secret being transmitted or guessable from the
// kid alone.
func kidFor(secret []byte) string {
	sum := sha256.Sum256(secret)
	return hex.EncodeToString(sum[:8])
}

// IssueAccess mints a 15-minute HS256 access token for userID under
// familyID (the session's family id, D-D's sid claim).
func (iss Issuer) IssueAccess(userID, familyID uuid.UUID, isSuperadmin bool) (string, error) {
	now := iss.clock().Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTTL)),
			ID:        uuid.NewString(),
		},
		SID: familyID.String(),
		SA:  isSuperadmin,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tok.Header["kid"] = kidFor(iss.Secret)
	return tok.SignedString(iss.Secret)
}

// VerifyAccess parses and validates an access token, selecting
// JWT_SECRET or JWT_SECRET_PREVIOUS by the token's kid header (D-D,
// §7.7).
func (iss Issuer) VerifyAccess(raw string) (Claims, error) {
	var claims Claims
	keyFunc := func(t *jwt.Token) (interface{}, error) {
		kid, _ := t.Header["kid"].(string)
		switch {
		case kid == kidFor(iss.Secret):
			return iss.Secret, nil
		case len(iss.PreviousSecret) > 0 && kid == kidFor(iss.PreviousSecret):
			return iss.PreviousSecret, nil
		default:
			return nil, ErrUnknownKID
		}
	}

	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithTimeFunc(iss.clock().Now),
	)
	tok, err := parser.ParseWithClaims(raw, &claims, keyFunc)
	if err != nil {
		return Claims{}, err
	}
	if !tok.Valid {
		return Claims{}, ErrInvalidToken
	}
	return claims, nil
}
