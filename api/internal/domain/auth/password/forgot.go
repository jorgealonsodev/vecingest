package password

import (
	"context"

	"github.com/google/uuid"
)

// ForgotPasswordResult is what a forgot-password request returns,
// identically, whether or not the target email is registered
// (auth-credentials: Enumeration-Safe Auth Responses). Accepted is
// always true; the HTTP layer (Phase 5) renders it into the same
// 200-shaped response every time, so no branch anywhere on the response
// path may depend on whether the lookup found a user.
type ForgotPasswordResult struct {
	Accepted bool
}

// UserLookup resolves an email to a user id, or ok=false when no
// account exists. The concrete implementation lives in internal/db.
type UserLookup interface {
	LookupByEmail(ctx context.Context, email string) (userID uuid.UUID, ok bool, err error)
}

// TokenIssuer mints an opaque token/hash pair for a password-reset
// token row (auth-session-tokens: Hashed Storage of Sensitive Tokens).
type TokenIssuer interface {
	GenerateOpaqueToken() (raw string, hash []byte, err error)
}

// ResetRequester persists the reset token and sends the reset email. It
// is invoked ONLY for a real account, so its cost (a DB write and an
// SMTP send) can never be observed from the response -- which is why
// ForgotPassword also dispatches it off its own call path. email is
// passed through so the concrete implementation has a recipient to
// send the reset email to.
type ResetRequester interface {
	RequestReset(ctx context.Context, userID uuid.UUID, email, rawToken string, tokenHash []byte) error
}

// ForgotPasswordService implements the enumeration-safe forgot-password
// flow.
type ForgotPasswordService struct {
	Users     UserLookup
	Tokens    TokenIssuer
	Requester ResetRequester
	// Dispatch decouples ResetRequester's work from ForgotPassword's own
	// call stack, exactly as lockout.Dispatcher does, and for the same
	// reason: inline SMTP/DB latency on the success path only would be a
	// timing oracle for account existence. Defaults to a real goroutine
	// when nil.
	Dispatch func(func())
}

func (s ForgotPasswordService) dispatch() func(func()) {
	if s.Dispatch != nil {
		return s.Dispatch
	}
	return func(fn func()) { go fn() }
}

// ForgotPassword looks up email, and if a user exists, dispatches
// RequestReset asynchronously; the returned result is identical either
// way (auth-credentials: Enumeration-Safe Auth Responses).
func (s ForgotPasswordService) ForgotPassword(ctx context.Context, email string) (ForgotPasswordResult, error) {
	userID, ok, err := s.Users.LookupByEmail(ctx, email)
	if err != nil {
		return ForgotPasswordResult{}, err
	}
	if ok {
		raw, hash, err := s.Tokens.GenerateOpaqueToken()
		if err != nil {
			return ForgotPasswordResult{}, err
		}
		s.dispatch()(func() {
			_ = s.Requester.RequestReset(ctx, userID, email, raw, hash)
		})
	}
	return ForgotPasswordResult{Accepted: true}, nil
}
