package mfa_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
	"github.com/jorgealonsodev/vecingest/internal/testhelpers"
)

type fixedMFAClock struct{ now time.Time }

func (c fixedMFAClock) Now() time.Time { return c.now }

var testEncryptionKey = testKey()

func seedUser(t *testing.T, ctx context.Context, handles db.Handles) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	if _, err := handles.Write.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, 'x', 'Test User')`,
		userID, userID.String()+"@example.com"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return userID
}

// auth-mfa-totp: TOTP Enrollment -- "Enrollment requires verification".
func TestEnrollThenConfirm_StaysInactiveUntilValidCode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, _ := testhelpers.AppRWHandles(t)
	ctx := context.Background()
	userID := seedUser(t, ctx, handles)
	now := time.Now().UTC()
	clock := fixedMFAClock{now}

	result, err := mfa.Enroll(ctx, handles.Write, testEncryptionKey, userID)
	if err != nil {
		t.Fatalf("Enroll: unexpected error: %v", err)
	}

	q := db.New(handles.Read)
	row, err := q.GetUserMFA(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserMFA: %v", err)
	}
	if row.EnabledAt.Valid {
		t.Fatalf("expected enrollment to be inactive right after Enroll")
	}

	ok, err := mfa.ConfirmEnrollment(ctx, handles.Write, clock, testEncryptionKey, userID, "000000")
	if err != nil {
		t.Fatalf("ConfirmEnrollment(invalid): %v", err)
	}
	if ok {
		t.Fatalf("expected an invalid code to fail confirmation")
	}
	row, err = q.GetUserMFA(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserMFA: %v", err)
	}
	if row.EnabledAt.Valid {
		t.Fatalf("expected TOTP to remain inactive after an invalid confirmation code")
	}

	validCode := codeAtStep(t, result.Secret, now.Unix()/30)
	ok, err = mfa.ConfirmEnrollment(ctx, handles.Write, clock, testEncryptionKey, userID, validCode)
	if err != nil {
		t.Fatalf("ConfirmEnrollment(valid): %v", err)
	}
	if !ok {
		t.Fatalf("expected a valid code to confirm enrollment")
	}
	row, err = q.GetUserMFA(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserMFA: %v", err)
	}
	if !row.EnabledAt.Valid {
		t.Fatalf("expected TOTP to be active after a valid confirmation code")
	}
}

// auth-mfa-totp: TOTP Replay Protection -- "Replayed code rejected".
func TestVerifyTOTP_RejectsReplayedCode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, _ := testhelpers.AppRWHandles(t)
	ctx := context.Background()
	userID := seedUser(t, ctx, handles)
	now := time.Now().UTC()
	clock := fixedMFAClock{now}

	result, err := mfa.Enroll(ctx, handles.Write, testEncryptionKey, userID)
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	code := codeAtStep(t, result.Secret, now.Unix()/30)

	outcome, err := mfa.VerifyTOTP(ctx, handles.Write, clock, userID, result.Secret, code)
	if err != nil {
		t.Fatalf("first VerifyTOTP: %v", err)
	}
	if outcome != mfa.OutcomeAccepted {
		t.Fatalf("first VerifyTOTP outcome = %v, want Accepted", outcome)
	}

	outcome, err = mfa.VerifyTOTP(ctx, handles.Write, clock, userID, result.Secret, code)
	if err != nil {
		t.Fatalf("second VerifyTOTP: %v", err)
	}
	if outcome != mfa.OutcomeReplayed {
		t.Fatalf("second VerifyTOTP outcome = %v, want Replayed", outcome)
	}
}

// A T-1 code rejected after T was accepted (backward-drift replay).
func TestVerifyTOTP_RejectsEarlierStepAfterLaterAccepted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, _ := testhelpers.AppRWHandles(t)
	ctx := context.Background()
	userID := seedUser(t, ctx, handles)
	now := time.Now().UTC()
	clock := fixedMFAClock{now}

	result, err := mfa.Enroll(ctx, handles.Write, testEncryptionKey, userID)
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}

	currentCode := codeAtStep(t, result.Secret, now.Unix()/30)
	outcome, err := mfa.VerifyTOTP(ctx, handles.Write, clock, userID, result.Secret, currentCode)
	if err != nil || outcome != mfa.OutcomeAccepted {
		t.Fatalf("VerifyTOTP(current): outcome=%v err=%v, want Accepted", outcome, err)
	}

	earlierCode := codeAtStep(t, result.Secret, now.Unix()/30-1)
	outcome, err = mfa.VerifyTOTP(ctx, handles.Write, clock, userID, result.Secret, earlierCode)
	if err != nil {
		t.Fatalf("VerifyTOTP(T-1): %v", err)
	}
	if outcome != mfa.OutcomeReplayed {
		t.Fatalf("VerifyTOTP(T-1 after T) outcome = %v, want Replayed", outcome)
	}
}

// db-access-control-adjacent (D-P): two concurrent TOTP verifications at
// READ COMMITTED -- exactly one wins, the loser gets a clean rejection,
// never a 500.
func TestVerifyTOTP_ConcurrentVerificationsExactlyOneWins(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, _ := testhelpers.AppRWHandles(t)
	ctx := context.Background()
	userID := seedUser(t, ctx, handles)
	now := time.Now().UTC()
	clock := fixedMFAClock{now}

	result, err := mfa.Enroll(ctx, handles.Write, testEncryptionKey, userID)
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	code := codeAtStep(t, result.Secret, now.Unix()/30)

	const attempts = 10
	outcomes := make([]mfa.VerifyOutcome, attempts)
	errs := make([]error, attempts)

	g, gctx := errgroup.WithContext(ctx)
	for i := 0; i < attempts; i++ {
		i := i
		g.Go(func() error {
			outcomes[i], errs[i] = mfa.VerifyTOTP(gctx, handles.Write, clock, userID, result.Secret, code)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatalf("errgroup: %v", err)
	}

	accepted := 0
	for i := 0; i < attempts; i++ {
		if errs[i] != nil {
			t.Fatalf("attempt %d returned an error instead of a clean outcome: %v", i, errs[i])
		}
		if outcomes[i] == mfa.OutcomeAccepted {
			accepted++
		} else if outcomes[i] != mfa.OutcomeReplayed {
			t.Fatalf("attempt %d outcome = %v, want Accepted or Replayed", i, outcomes[i])
		}
	}
	if accepted != 1 {
		t.Fatalf("expected exactly 1 of %d concurrent verifications to win, got %d", attempts, accepted)
	}
}

// auth-mfa-totp: One-Time Recovery Codes -- "Recovery code reused".
func TestConsumeRecoveryCode_SingleUse(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, _ := testhelpers.AppRWHandles(t)
	ctx := context.Background()
	userID := seedUser(t, ctx, handles)

	raw, hashed, err := mfa.GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes: %v", err)
	}
	if _, err := mfa.Enroll(ctx, handles.Write, testEncryptionKey, userID); err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	q := db.New(handles.Write)
	if err := q.SetUserMFARecoveryCodes(ctx, db.SetUserMFARecoveryCodesParams{
		UserID: userID, RecoveryCodesHashed: hashed,
	}); err != nil {
		t.Fatalf("SetUserMFARecoveryCodes: %v", err)
	}

	ok, err := mfa.ConsumeRecoveryCode(ctx, handles.Write, userID, raw[0])
	if err != nil {
		t.Fatalf("first ConsumeRecoveryCode: %v", err)
	}
	if !ok {
		t.Fatalf("expected the first consumption to succeed")
	}

	ok, err = mfa.ConsumeRecoveryCode(ctx, handles.Write, userID, raw[0])
	if err != nil {
		t.Fatalf("second ConsumeRecoveryCode: %v", err)
	}
	if ok {
		t.Fatalf("expected a reused recovery code to be rejected")
	}

	// The other 9 codes remain usable.
	ok, err = mfa.ConsumeRecoveryCode(ctx, handles.Write, userID, raw[1])
	if err != nil {
		t.Fatalf("ConsumeRecoveryCode(raw[1]): %v", err)
	}
	if !ok {
		t.Fatalf("expected an untouched recovery code to still be consumable")
	}
}
