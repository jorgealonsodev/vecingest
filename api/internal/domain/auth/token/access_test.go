package token_test

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

// auth-session-tokens: JWT Access and Refresh Issuance -- "Successful
// login issues both tokens" leg (the access-token half; the refresh half
// is exercised in refresh_test.go).
func TestIssueAccess_ExpiresIn15Minutes(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	iss := token.Issuer{Secret: []byte("current-secret-32-bytes-minimum!"), Clock: fixedClock{now}}
	userID := uuid.New()
	familyID := uuid.New()

	raw, err := iss.IssueAccess(userID, familyID, false, false)
	if err != nil {
		t.Fatalf("IssueAccess: unexpected error: %v", err)
	}

	claims, err := iss.VerifyAccess(raw)
	if err != nil {
		t.Fatalf("VerifyAccess: unexpected error: %v", err)
	}
	if claims.Subject != userID.String() {
		t.Errorf("sub = %q, want %q", claims.Subject, userID.String())
	}
	if claims.SID != familyID.String() {
		t.Errorf("sid = %q, want %q", claims.SID, familyID.String())
	}
	if claims.SA {
		t.Errorf("sa = true, want false")
	}
	if claims.ID == "" {
		t.Errorf("jti is empty, want a generated value")
	}
	gotExp := claims.ExpiresAt.Time
	wantExp := now.Add(15 * time.Minute)
	if !gotExp.Equal(wantExp) {
		t.Errorf("exp = %v, want %v", gotExp, wantExp)
	}
	gotIat := claims.IssuedAt.Time
	if !gotIat.Equal(now) {
		t.Errorf("iat = %v, want %v", gotIat, now)
	}
}

// auth-mfa-totp (review lineage review-0e1833930adf141a): the mfa claim
// reports how the SESSION authenticated. Both readings matter, so both
// are asserted: an issuer told the session passed a TOTP challenge must
// say so, and one told it did not must not.
func TestIssueAccess_CarriesMFAClaimBothWays(t *testing.T) {
	iss := token.Issuer{Secret: []byte("current-secret-32-bytes-minimum!")}

	raw, err := iss.IssueAccess(uuid.New(), uuid.New(), false, true)
	if err != nil {
		t.Fatalf("IssueAccess: unexpected error: %v", err)
	}
	claims, err := iss.VerifyAccess(raw)
	if err != nil {
		t.Fatalf("VerifyAccess: unexpected error: %v", err)
	}
	if !claims.MFA {
		t.Errorf("mfa = false for a second-factor-authenticated session, want true")
	}

	raw, err = iss.IssueAccess(uuid.New(), uuid.New(), false, false)
	if err != nil {
		t.Fatalf("IssueAccess: unexpected error: %v", err)
	}
	claims, err = iss.VerifyAccess(raw)
	if err != nil {
		t.Fatalf("VerifyAccess: unexpected error: %v", err)
	}
	if claims.MFA {
		t.Errorf("mfa = true for a password-only session, want false")
	}
}

// A token minted BEFORE the mfa claim existed carries no such property.
// It must decode as NOT second-factor authenticated -- the fail-closed
// reading -- because none of those tokens ever passed a challenge. This
// is what makes the fix safe on the already-deployed stack rather than
// only on a fresh one.
func TestVerifyAccess_TokenWithoutMFAClaimDecodesAsNotAuthenticated(t *testing.T) {
	secret := []byte("current-secret-32-bytes-minimum!")
	iss := token.Issuer{Secret: secret}

	// Minted by hand in the pre-mfa shape: sub/sid/sa and nothing else.
	legacy := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": uuid.NewString(),
		"sid": uuid.NewString(),
		"sa":  false,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(10 * time.Minute).Unix(),
		"jti": uuid.NewString(),
	})
	legacy.Header["kid"] = token.KIDForTest(secret)
	raw, err := legacy.SignedString(secret)
	if err != nil {
		t.Fatalf("sign legacy token: %v", err)
	}

	claims, err := iss.VerifyAccess(raw)
	if err != nil {
		t.Fatalf("VerifyAccess: a token minted before the mfa claim existed must still verify: %v", err)
	}
	if claims.MFA {
		t.Errorf("mfa = true for a token that carries no mfa claim, want false -- an already-issued token must never read as elevated")
	}
}

func TestIssueAccess_CarriesSuperadminClaim(t *testing.T) {
	iss := token.Issuer{Secret: []byte("current-secret-32-bytes-minimum!")}
	raw, err := iss.IssueAccess(uuid.New(), uuid.New(), true, false)
	if err != nil {
		t.Fatalf("IssueAccess: unexpected error: %v", err)
	}
	claims, err := iss.VerifyAccess(raw)
	if err != nil {
		t.Fatalf("VerifyAccess: unexpected error: %v", err)
	}
	if !claims.SA {
		t.Errorf("sa = false, want true")
	}
}

// D-D: verification selects JWT_SECRET_PREVIOUS by kid during rotation.
func TestVerifyAccess_AcceptsPreviousSecretByKID(t *testing.T) {
	previous := []byte("previous-secret-32-bytes-minimum")
	current := []byte("current--secret-32-bytes-minimum")

	oldIssuer := token.Issuer{Secret: previous}
	raw, err := oldIssuer.IssueAccess(uuid.New(), uuid.New(), false, false)
	if err != nil {
		t.Fatalf("IssueAccess: unexpected error: %v", err)
	}

	rotated := token.Issuer{Secret: current, PreviousSecret: previous}
	if _, err := rotated.VerifyAccess(raw); err != nil {
		t.Fatalf("VerifyAccess with rotated secret: unexpected error: %v", err)
	}
}

func TestVerifyAccess_RejectsUnknownKID(t *testing.T) {
	issuer := token.Issuer{Secret: []byte("issuer-secret-32-bytes-minimum!!")}
	raw, err := issuer.IssueAccess(uuid.New(), uuid.New(), false, false)
	if err != nil {
		t.Fatalf("IssueAccess: unexpected error: %v", err)
	}

	other := token.Issuer{Secret: []byte("a-totally-different-secret-value")}
	if _, err := other.VerifyAccess(raw); err == nil {
		t.Fatalf("VerifyAccess: expected an error for an unrecognized kid, got nil")
	}
}

func TestVerifyAccess_RejectsExpiredToken(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	iss := token.Issuer{Secret: []byte("current-secret-32-bytes-minimum!"), Clock: fixedClock{now}}
	raw, err := iss.IssueAccess(uuid.New(), uuid.New(), false, false)
	if err != nil {
		t.Fatalf("IssueAccess: unexpected error: %v", err)
	}

	future := fixedClock{now.Add(16 * time.Minute)}
	lateIssuer := token.Issuer{Secret: iss.Secret, Clock: future}
	if _, err := lateIssuer.VerifyAccess(raw); err == nil {
		t.Fatalf("VerifyAccess: expected an error for an expired token, got nil")
	}
}

func TestVerifyAccess_RejectsTamperedSignature(t *testing.T) {
	iss := token.Issuer{Secret: []byte("current-secret-32-bytes-minimum!")}
	raw, err := iss.IssueAccess(uuid.New(), uuid.New(), false, false)
	if err != nil {
		t.Fatalf("IssueAccess: unexpected error: %v", err)
	}
	// Tamper the FIRST character of the signature segment, not the last.
	// An HMAC-SHA256 signature is 32 bytes, which base64url-encodes to 43
	// characters whose final character carries only 4 significant bits: four
	// different trailing characters decode to the same 32 bytes, so flipping
	// the last one leaves the signature unchanged roughly once in sixteen.
	// Every bit of the first character is significant.
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		t.Fatalf("IssueAccess: expected a three-segment JWT, got %d segments", len(parts))
	}
	sig := parts[2]
	replacement := byte('A')
	if sig[0] == 'A' {
		replacement = 'B'
	}
	parts[2] = string(replacement) + sig[1:]
	tampered := strings.Join(parts, ".")
	if _, err := iss.VerifyAccess(tampered); err == nil {
		t.Fatalf("VerifyAccess: expected an error for a tampered token, got nil")
	}
}
