package mfa

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ThrottleThreshold and ThrottleWindow mirror D-N's login-lockout
// parameters exactly, per D-P: "same store, window and threshold" as
// auth:fail:email:*/auth:fail:ip:*.
const (
	ThrottleThreshold = 5
	ThrottleWindow    = 15 * time.Minute
)

// AttemptCounter is the same D-N seam lockout.AttemptCounter declares,
// re-declared here at this package's own point of use (Go's structural
// typing means internal/platform/attempts.Counter satisfies both with
// no adapter).
type AttemptCounter interface {
	Fail(ctx context.Context, key string, window time.Duration) (int, error)
	Count(ctx context.Context, key string, window time.Duration) (int, error)
	Reset(ctx context.Context, key string) error
}

func totpFailKey(userID uuid.UUID) string {
	return "auth:fail:totp:" + userID.String()
}

// ThrottledVerify wraps a TOTP-or-recovery-code verification closure
// with the auth:fail:totp:<user_id> budget (auth-mfa-totp: TOTP Attempt
// Throttling; D-N/D-P). Both VerifyTOTP and ConsumeRecoveryCode wire
// through this same function so recovery-code failures count toward the
// identical budget and are never a way around it. throttled=true means
// verify was never even called. A successful verify resets the per-user
// key; a failing one increments it. This function touches ONLY the
// per-user key -- it never resets or reads any IP-keyed counter, which
// is what keeps it independent of D-N's own login-lockout counters.
func ThrottledVerify(ctx context.Context, counter AttemptCounter, userID uuid.UUID, verify func(ctx context.Context) (bool, error)) (accepted, throttled bool, err error) {
	key := totpFailKey(userID)

	n, err := counter.Count(ctx, key, ThrottleWindow)
	if err != nil {
		return false, false, err
	}
	if n >= ThrottleThreshold {
		return false, true, nil
	}

	ok, err := verify(ctx)
	if err != nil {
		return false, false, err
	}
	if ok {
		_ = counter.Reset(ctx, key)
		return true, false, nil
	}

	if _, err := counter.Fail(ctx, key, ThrottleWindow); err != nil {
		return false, false, err
	}
	return false, false, nil
}
