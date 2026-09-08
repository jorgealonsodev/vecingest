package handlers

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
	"github.com/jorgealonsodev/vecingest/internal/http/dto"
)

// refreshCSRFInput is GET /v1/auth/refresh/csrf's only input: the
// refresh cookie itself.
type refreshCSRFInput struct {
	RefreshCookie http.Cookie `cookie:"vecingest_refresh"`
}

// RegisterRefreshCSRF wires GET /v1/auth/refresh/csrf into api
// (auth-csrf-origin: CSRF Token Recovery Endpoint). It is deliberately
// placed under /v1/auth/refresh/csrf, a subpath of the refresh cookie's
// own Path=/v1/auth/refresh, so RFC 6265 §5.1.4 path-matching actually
// attaches the cookie -- /v1/auth/csrf would not receive it and would
// return 401 unconditionally (design D-E). It is a safe method: it
// mints a token bound to the exact live session row the cookie hashes
// to, and rotates nothing.
func RegisterRefreshCSRF(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "refreshCSRF",
		Method:      "GET",
		Path:        "/v1/auth/refresh/csrf",
		Summary:     "Restore the CSRF token after a page reload",
		Tags:        []string{"auth"},
	}, func(ctx context.Context, in *refreshCSRFInput) (*dto.CSRFTokenOutput, error) {
		out := &dto.CSRFTokenOutput{
			CacheControl: "no-store",
			Vary:         "Origin",
			CORP:         "same-origin",
		}

		if in.RefreshCookie.Name == "" {
			return nil, apperr.New(401, apperr.CodeRefreshInvalid, "missing refresh cookie", nil)
		}

		q := db.New(d.DB.Write)
		hash := token.HashToken(in.RefreshCookie.Value)
		row, err := q.GetSessionByRefreshTokenHash(ctx, hash)
		if err != nil {
			if isNoRows(err) {
				return nil, apperr.New(401, apperr.CodeRefreshInvalid, "refresh token invalid or expired", nil)
			}
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		if row.RevokedAt.Valid || !row.ExpiresAt.After(d.clock().Now()) {
			return nil, apperr.New(401, apperr.CodeRefreshInvalid, "refresh token invalid or expired", nil)
		}

		csrf, err := token.MintCSRF(d.CSRFKey, row.FamilyID, row.ID, row.ExpiresAt)
		if err != nil {
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		out.Body.CSRFToken = csrf
		return out, nil
	})
}
