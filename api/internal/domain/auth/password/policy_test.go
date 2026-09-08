package password_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/password"
)

// auth-credentials: Conditional Password Length Floor -- "Password too
// short without 2FA".
func TestPasswordPolicy_CheckLength_RejectsFourteenCharsWithoutTOTP(t *testing.T) {
	policy := password.PasswordPolicy{}

	err := policy.CheckLength("12345678901234", false) // 14 chars
	if err == nil {
		t.Fatalf("expected a 14-character password without TOTP to be rejected")
	}

	var perr *password.PolicyError
	if !errors.As(err, &perr) {
		t.Fatalf("expected a *password.PolicyError, got %T", err)
	}
	if perr.Code != password.ErrCodeTooShortNoMFA {
		t.Fatalf("expected code %s, got %s", password.ErrCodeTooShortNoMFA, perr.Code)
	}
	if !strings.Contains(perr.Error(), "15") {
		t.Fatalf("expected the error to state the 15-character rule, got: %v", perr)
	}
	if perr.Details["min_length"] != 15 {
		t.Fatalf("expected details.min_length=15, got %v", perr.Details["min_length"])
	}
}

// auth-credentials: Conditional Password Length Floor -- "Password
// accepted with TOTP active".
func TestPasswordPolicy_CheckLength_AcceptsTwelveCharsWithTOTP(t *testing.T) {
	policy := password.PasswordPolicy{}

	err := policy.CheckLength("123456789012", true) // 12 chars
	if err != nil {
		t.Fatalf("expected a 12-character password with TOTP active to be accepted, got: %v", err)
	}
}

// Triangulation: a 12-character password WITHOUT TOTP must still be
// rejected, and the error must name the WITH_MFA code when TOTP is
// active but the password is still too short for that floor.
func TestPasswordPolicy_CheckLength_TwelveCharsWithoutTOTPRejected(t *testing.T) {
	policy := password.PasswordPolicy{}
	err := policy.CheckLength("123456789012", false)
	if err == nil {
		t.Fatalf("expected a 12-character password without TOTP to be rejected")
	}
}

func TestPasswordPolicy_CheckLength_ElevenCharsWithTOTPRejectedNamingMFARule(t *testing.T) {
	policy := password.PasswordPolicy{}
	err := policy.CheckLength("12345678901", true) // 11 chars

	var perr *password.PolicyError
	if !errors.As(err, &perr) {
		t.Fatalf("expected a *password.PolicyError, got %T (%v)", err, err)
	}
	if perr.Code != password.ErrCodeTooShortWithMFA {
		t.Fatalf("expected code %s, got %s", password.ErrCodeTooShortWithMFA, perr.Code)
	}
	if !strings.Contains(perr.Error(), "12") {
		t.Fatalf("expected the error to state the 12-character rule, got: %v", perr)
	}
}

// --- HIBP integration into PasswordPolicy.Validate ---

type stubHIBPChecker struct {
	breached bool
	err      error
}

func (s stubHIBPChecker) IsBreached(_ context.Context, _ string) (bool, error) {
	return s.breached, s.err
}

type recordingHook struct {
	called bool
}

func (h *recordingHook) record(_ context.Context) { h.called = true }

func TestPasswordPolicy_Validate_RejectsBreachedPassword(t *testing.T) {
	policy := password.PasswordPolicy{HIBP: stubHIBPChecker{breached: true}}

	err := policy.Validate(context.Background(), "a-long-enough-password-value", false)
	if err == nil {
		t.Fatalf("expected a breached password to be rejected")
	}
}

func TestPasswordPolicy_Validate_AcceptsNonBreachedPassword(t *testing.T) {
	policy := password.PasswordPolicy{HIBP: stubHIBPChecker{breached: false}}

	err := policy.Validate(context.Background(), "a-long-enough-password-value", false)
	if err != nil {
		t.Fatalf("expected a non-breached, sufficiently long password to be accepted, got: %v", err)
	}
}

// auth-credentials: HIBP k-Anonymity Breach Check -- "HIBP transport
// error fails open": the password is accepted on that basis alone, a
// WARN is logged, and the audit hook fires.
func TestPasswordPolicy_Validate_FailsOpenOnHIBPTransportError(t *testing.T) {
	hook := &recordingHook{}
	var warnLogged bool
	policy := password.PasswordPolicy{
		HIBP:              stubHIBPChecker{err: errors.New("connection refused")},
		WarnLog:           func(_ context.Context, _ string, _ ...any) { warnLogged = true },
		OnHIBPUnavailable: hook.record,
	}

	err := policy.Validate(context.Background(), "a-long-enough-password-value", false)
	if err != nil {
		t.Fatalf("expected fail-open to accept the password despite the HIBP transport error, got: %v", err)
	}
	if !warnLogged {
		t.Fatalf("expected a WARN to be logged on HIBP transport error")
	}
	if !hook.called {
		t.Fatalf("expected the audit hook to fire on HIBP transport error")
	}
}

// Triangulation: length is still enforced even on the fail-open path --
// HIBP failing open must never bypass the length floor.
func TestPasswordPolicy_Validate_LengthStillEnforcedWhenHIBPFailsOpen(t *testing.T) {
	policy := password.PasswordPolicy{
		HIBP: stubHIBPChecker{err: errors.New("connection refused")},
	}

	err := policy.Validate(context.Background(), "short", false)
	if err == nil {
		t.Fatalf("expected the length floor to still reject a short password even when HIBP fails open")
	}
}
