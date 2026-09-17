package authz

import (
	"context"

	"github.com/google/uuid"
)

// userIDContextKey is an unexported type so no other package can ever
// collide with or forge this context value.
type userIDContextKey struct{}

// ContextWithUserID attaches the already-authenticated caller's user id
// to ctx. The Bearer-auth middleware wrapping a scoped operation's group
// calls this once verification succeeds (mirroring the existing D-H
// bearerAuthAndRateLimit pattern); scoped.* reads it back via
// UserIDFromContext before resolving membership, so resolution never
// trusts a header or body for the caller's identity.
func ContextWithUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, userIDContextKey{}, userID)
}

// UserIDFromContext reads back the value ContextWithUserID stored. ok is
// false when no authenticated caller was ever attached.
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDContextKey{}).(uuid.UUID)
	return id, ok
}

// mfaContextKey is, like userIDContextKey, an unexported type: the
// second-factor fact is read back only through this package's own
// accessor, and no other package can forge it into a context.
type mfaContextKey struct{}

// ContextWithMFAAuthenticated attaches the SESSION's second-factor fact
// (the access token's mfa claim) to ctx. The same Bearer-auth middleware
// that publishes the caller's user id publishes this beside it, so the
// mandatory-TOTP gate reads how THIS session authenticated rather than
// what the account is capable of (review lineage
// review-0e1833930adf141a, R1-mandatory-totp-gate-is-only-an-enrollment-
// flag).
func ContextWithMFAAuthenticated(ctx context.Context, mfaAuthenticated bool) context.Context {
	return context.WithValue(ctx, mfaContextKey{}, mfaAuthenticated)
}

// MFAAuthenticatedFromContext reads the fact back. A context that was
// never given one reports false -- fail-closed: an unauthenticated,
// forged or simply unwired path is treated as NOT second-factor
// authenticated, never as elevated.
func MFAAuthenticatedFromContext(ctx context.Context) bool {
	mfaAuthenticated, _ := ctx.Value(mfaContextKey{}).(bool)
	return mfaAuthenticated
}
