package handlers

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
	"github.com/jorgealonsodev/vecingest/internal/http/dto"
)

// Logout implements POST /v1/auth/logout (auth-session-tokens:
// Bearer-Authenticated Logout). It is authenticated by the access
// token, never the refresh cookie -- the cookie's Path=/v1/auth/refresh
// means it is never sent here -- and requires no CSRF token, because
// the request carries no ambient authority a cross-site attacker could
// abuse. The refresh cookie is still cleared: setting a cookie is not
// path-matched the way sending one is.
func (d *Deps) Logout(ctx context.Context, in *dto.LogoutInput) (*dto.LogoutOutput, error) {
	claims, err := d.authenticate(ctx, in.Authorization)
	if err != nil {
		if errors.Is(err, ErrMissingBearer) {
			return nil, apperr.New(401, apperr.CodeUnauthorized, "missing or invalid Authorization header", nil)
		}
		return nil, apperr.New(401, apperr.CodeUnauthorized, "invalid or expired access token", nil)
	}

	familyID, err := uuid.Parse(claims.SID)
	if err != nil {
		return nil, apperr.New(401, apperr.CodeUnauthorized, "invalid session claim", nil)
	}

	q := db.New(d.DB.Write)
	if err := q.RevokeSessionFamily(ctx, familyID); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if d.RevocationCache != nil {
		d.RevocationCache.MarkRevoked(familyID)
	}

	return &dto.LogoutOutput{SetCookie: ClearedRefreshCookie(d.CookieDomain)}, nil
}

// RegisterLogout wires POST /v1/auth/logout into api.
func RegisterLogout(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "logout",
		Method:      "POST",
		Path:        "/v1/auth/logout",
		Summary:     "Revoke the current session family",
		Tags:        []string{"auth"},
	}, d.Logout)
}
