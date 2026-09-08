package mfa

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jorgealonsodev/vecingest/internal/db"
)

// VerifyOutcome distinguishes WHY a TOTP verification did not succeed,
// so a caller can render the correct error code without ever risking a
// 500 on a legitimately-rejected concurrent loser.
type VerifyOutcome int

const (
	// OutcomeAccepted: the code matched an un-consumed step and the
	// monotonic counter advanced.
	OutcomeAccepted VerifyOutcome = iota
	// OutcomeInvalidCode: no candidate step (T-1, T, T+1) matched.
	OutcomeInvalidCode
	// OutcomeReplayed: the code matched a step, but that step was
	// already consumed -- either observed directly (zero rows affected,
	// the READ COMMITTED-only property design D-P relies on) or via a
	// concurrent-update serialization failure (pgerrcode 40001) under
	// any higher isolation level. Both classify identically: reject,
	// never a 500.
	OutcomeReplayed
)

// VerifyTOTP implements D-P's full verification: MatchStep, then a
// conditional UPDATE that only advances user_mfa.last_totp_step
// forward, never backward. internal/db's transaction on wdb MUST NOT
// raise the isolation level above READ COMMITTED for this call: "zero
// rows means a concurrent request won" is a READ COMMITTED-only
// property (design D-P). If it ever is raised, the pgerrcode.SerializationFailure
// branch below is what keeps this a clean rejection instead of an
// unhandled 500.
func VerifyTOTP(ctx context.Context, wdb db.WriteDB, clock Clock, userID uuid.UUID, secret []byte, code string) (VerifyOutcome, error) {
	step, ok := MatchStep(secret, code, clock.Now())
	if !ok {
		return OutcomeInvalidCode, nil
	}

	q := db.New(wdb)
	n, err := q.UpdateUserMFALastTOTPStep(ctx, db.UpdateUserMFALastTOTPStepParams{
		UserID:       userID,
		LastTotpStep: pgtype.Int8{Int64: step, Valid: true},
	})
	if err != nil {
		if isSerializationFailure(err) {
			return OutcomeReplayed, nil
		}
		return OutcomeInvalidCode, fmt.Errorf("mfa: update last_totp_step: %w", err)
	}
	if n == 0 {
		return OutcomeReplayed, nil
	}
	return OutcomeAccepted, nil
}

func isSerializationFailure(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == pgerrcode.SerializationFailure
	}
	return false
}
