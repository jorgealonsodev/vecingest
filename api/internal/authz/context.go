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
