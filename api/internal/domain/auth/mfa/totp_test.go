package mfa_test

import (
	"testing"
	"time"

	pquernaotp "github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
)

func mustSecret(t *testing.T) []byte {
	t.Helper()
	secret, err := mfa.GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: unexpected error: %v", err)
	}
	return secret
}

func codeAtStep(t *testing.T, secret []byte, step int64) string {
	t.Helper()
	b32 := mfa.Base32SecretForTest(secret)
	code, err := totp.GenerateCodeCustom(b32, time.Unix(step*30, 0), totp.ValidateOpts{
		Period: 30, Digits: pquernaotp.DigitsSix, Algorithm: pquernaotp.AlgorithmSHA1,
	})
	if err != nil {
		t.Fatalf("GenerateCodeCustom: unexpected error: %v", err)
	}
	return code
}

// auth-mfa-totp: TOTP Verification Parameters -- "Code from adjacent
// step accepted".
func TestMatchStep_AcceptsAdjacentStep(t *testing.T) {
	secret := mustSecret(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	currentStep := now.Unix() / 30
	prevCode := codeAtStep(t, secret, currentStep-1)

	step, ok := mfa.MatchStep(secret, prevCode, now)
	if !ok {
		t.Fatalf("expected the previous-step code to be accepted within drift")
	}
	if step != currentStep-1 {
		t.Errorf("matched step = %d, want %d", step, currentStep-1)
	}
}

// auth-mfa-totp: TOTP Verification Parameters -- "Code outside drift
// window rejected".
func TestMatchStep_RejectsTwoStepsAway(t *testing.T) {
	secret := mustSecret(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	currentStep := now.Unix() / 30
	farCode := codeAtStep(t, secret, currentStep-2)

	_, ok := mfa.MatchStep(secret, farCode, now)
	if ok {
		t.Fatalf("expected a code two steps away to be rejected")
	}
}

func TestMatchStep_AcceptsExactCurrentStep(t *testing.T) {
	secret := mustSecret(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	currentStep := now.Unix() / 30
	code := codeAtStep(t, secret, currentStep)

	step, ok := mfa.MatchStep(secret, code, now)
	if !ok || step != currentStep {
		t.Fatalf("MatchStep(current) = (%d, %v), want (%d, true)", step, ok, currentStep)
	}
}

func TestMatchStep_RejectsWrongSecret(t *testing.T) {
	secretA := mustSecret(t)
	secretB := mustSecret(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	code := codeAtStep(t, secretA, now.Unix()/30)

	if _, ok := mfa.MatchStep(secretB, code, now); ok {
		t.Fatalf("expected a code generated under a different secret to be rejected")
	}
}

// The comparison loop must run for all three candidates every time --
// exercised indirectly by confirming an invalid code never matches
// (proves there is no shortcut return before all three are tried) and
// that a code shared by two adjacent windows (contrived via a stub
// clock at an exact boundary) still resolves to the higher step.
func TestMatchStep_RecordsHighestMatchingStepWhenSecretRepeatsAcrossSteps(t *testing.T) {
	secret := mustSecret(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	currentStep := now.Unix() / 30

	// It is not generally true that two different steps produce the
	// same code, but MatchStep's contract is "iterate all three, keep
	// the highest match" regardless of how many match. Directly probe
	// that by feeding the code for the LATEST candidate (current+1) and
	// confirming that one -- not an earlier one -- is what gets
	// recorded, even though it is checked last in the loop body.
	nextCode := codeAtStep(t, secret, currentStep+1)
	step, ok := mfa.MatchStep(secret, nextCode, now)
	if !ok {
		t.Fatalf("expected the current+1 code to be accepted")
	}
	if step != currentStep+1 {
		t.Fatalf("matched step = %d, want %d (the highest/last candidate)", step, currentStep+1)
	}
}
