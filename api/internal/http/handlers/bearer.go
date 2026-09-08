package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
)

var (
	// ErrMissingBearer covers both a wholly absent Authorization header
	// and one that does not use the Bearer scheme.
	ErrMissingBearer = errors.New("handlers: missing or malformed Authorization: Bearer header")
	// ErrSessionRevoked is returned when the presented access token's
	// session family has already been revoked (auth-session-tokens:
	// Immediate Session Revocation Effect).
	ErrSessionRevoked = errors.New("handlers: session revoked")
)

const bearerPrefix = "Bearer "

// authenticate implements D-H step 9's Bearer-auth leg: parse the
// Authorization header, verify the JWT, then consult the revocation
// cache keyed by the token's sid (= family_id) so an already-revoked
// family's still-cryptographically-valid access token is rejected
// immediately, not merely at its next refresh.
func (d *Deps) authenticate(ctx context.Context, authorizationHeader string) (token.Claims, error) {
	if !strings.HasPrefix(authorizationHeader, bearerPrefix) {
		return token.Claims{}, ErrMissingBearer
	}
	raw := strings.TrimPrefix(authorizationHeader, bearerPrefix)
	if raw == "" {
		return token.Claims{}, ErrMissingBearer
	}

	claims, err := d.AccessIssuer.VerifyAccess(raw)
	if err != nil {
		return token.Claims{}, err
	}

	if d.RevocationCache != nil {
		if familyID, perr := uuid.Parse(claims.SID); perr == nil {
			revoked, rerr := d.RevocationCache.IsRevoked(ctx, familyID)
			if rerr != nil {
				return token.Claims{}, rerr
			}
			if revoked {
				return token.Claims{}, ErrSessionRevoked
			}
		}
	}

	return claims, nil
}

// Authenticate is the exported passthrough to authenticate, used by
// internal/http/api's per-route Bearer-auth-and-rate-limit middleware
// (D-H step 9). Every authenticated handler in this package ALSO calls
// its own unexported authenticate internally, so a caller reaching a
// handler by any other path (e.g. a direct unit test) still gets the
// exact same check.
func (d *Deps) Authenticate(ctx context.Context, authorizationHeader string) (token.Claims, error) {
	return d.authenticate(ctx, authorizationHeader)
}
