package mfa

import (
	"context"
	"crypto/subtle"
	"fmt"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/db"
)

// ConsumeRecoveryCode looks up every stored hash for userID and
// compares raw's hash against ALL of them with a constant-time
// comparison (auth-mfa-totp: One-Time Recovery Codes) -- never an
// early-exit search -- before removing the matched entry. Single-use is
// enforced by removing the matched hash from the stored array in the
// same call: a reused code no longer has a matching stored hash on its
// second presentation.
func ConsumeRecoveryCode(ctx context.Context, wdb db.WriteDB, userID uuid.UUID, raw string) (bool, error) {
	q := db.New(wdb)
	row, err := q.GetUserMFA(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("mfa: get user_mfa: %w", err)
	}

	target := HashRecoveryCode(raw)
	matched := false
	for _, stored := range row.RecoveryCodesHashed {
		if subtle.ConstantTimeCompare([]byte(stored), []byte(target)) == 1 {
			matched = true
		}
	}
	if !matched {
		return false, nil
	}

	n, err := q.ConsumeUserMFARecoveryCode(ctx, db.ConsumeUserMFARecoveryCodeParams{
		UserID:      userID,
		ArrayRemove: target,
	})
	if err != nil {
		return false, fmt.Errorf("mfa: consume recovery code: %w", err)
	}
	return n > 0, nil
}
