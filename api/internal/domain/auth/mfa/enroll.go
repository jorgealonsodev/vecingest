package mfa

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/db"
)

// EnrollResult is what Enroll returns: the raw secret (so the caller can
// render a QR/provisioning URI once, before it is ever persisted again)
// and the encrypted form already stored in user_mfa.
type EnrollResult struct {
	Secret          []byte
	EncryptedSecret []byte
}

// Enroll generates a fresh TOTP secret, encrypts it under key, and
// upserts it into user_mfa -- WITHOUT activating it. enabled_at stays
// NULL until ConfirmEnrollment verifies one valid code
// (auth-mfa-totp: TOTP Enrollment).
// It takes a db.DBTX so the caller can pass the transaction that also
// issues the enrollment email challenge: a pending secret must never
// exist without the code that confirms it, nor the reverse.
func Enroll(ctx context.Context, dbtx db.DBTX, key [32]byte, userID uuid.UUID) (EnrollResult, error) {
	secret, err := GenerateSecret()
	if err != nil {
		return EnrollResult{}, err
	}
	encrypted, err := EncryptSecret(key, secret)
	if err != nil {
		return EnrollResult{}, err
	}

	q := db.New(dbtx)
	if _, err := q.UpsertUserMFA(ctx, db.UpsertUserMFAParams{
		UserID:              userID,
		TotpSecretEncrypted: encrypted,
	}); err != nil {
		return EnrollResult{}, fmt.Errorf("mfa: upsert user_mfa: %w", err)
	}

	return EnrollResult{Secret: secret, EncryptedSecret: encrypted}, nil
}

// ConfirmEnrollment verifies one code against the encrypted secret
// already stored for userID and, only on success, marks enrollment
// active (enabled_at). It does not touch last_totp_step -- that would
// let the confirmation code itself be replayed as the first live TOTP
// code; the design's replay protection is scoped to VerifyTOTP,
// invoked separately once enrollment is active.
// It takes a db.DBTX rather than a db.WriteDB so the caller can pass an
// open transaction: activation and recovery-code issuance MUST commit
// or roll back together (review lineage review-0e1833930adf141a).
func ConfirmEnrollment(ctx context.Context, dbtx db.DBTX, clock Clock, key [32]byte, userID uuid.UUID, code string) (bool, error) {
	q := db.New(dbtx)
	row, err := q.GetUserMFA(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("mfa: get user_mfa: %w", err)
	}

	secret, err := DecryptSecret(key, row.TotpSecretEncrypted)
	if err != nil {
		return false, fmt.Errorf("mfa: decrypt secret: %w", err)
	}

	if _, ok := MatchStep(secret, code, clock.Now()); !ok {
		return false, nil
	}

	if err := q.ConfirmUserMFAEnrollment(ctx, userID); err != nil {
		return false, fmt.Errorf("mfa: confirm enrollment: %w", err)
	}
	return true, nil
}
