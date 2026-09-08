package handlers

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/session"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
	"github.com/jorgealonsodev/vecingest/internal/http/dto"
)

// Refresh implements POST /v1/auth/refresh (auth-session-tokens: Refresh
// Rotation with Family Invalidation, Refresh Transport Mutual
// Exclusion; auth-csrf-origin: CSRF Token Delivered In-Body, Origin
// Validation on Refresh). It re-resolves the presented token's session
// row BEFORE rotating so the CSRF header can be verified against the
// EXACT family_id/session_id the cookie still names -- the same pair
// Rotator.Rotate itself re-derives internally from the same hash a
// moment later. The tiny double lookup keeps Rotator's already-tested
// Phase 4 API untouched rather than reshaping it to return pre-rotation
// state.
func (d *Deps) Refresh(ctx context.Context, in *dto.RefreshInput) (*dto.RefreshOutput, error) {
	cookieToken := ""
	if in.RefreshCookie.Name != "" {
		cookieToken = in.RefreshCookie.Value
	}

	transport, err := session.ResolveTransport(cookieToken, in.Body.RefreshToken)
	if err != nil {
		switch {
		case errors.Is(err, session.ErrAmbiguousTokenTransport):
			return nil, apperr.New(400, apperr.CodeAmbiguousTokenTransport, "refresh token present in both cookie and body", nil)
		default:
			return nil, apperr.New(400, apperr.CodeMissingTokenTransport, "no refresh token presented", nil)
		}
	}

	q := db.New(d.DB.Write)
	rawToken := transport.Token
	hash := token.HashToken(rawToken)
	row, err := q.GetSessionByRefreshTokenHash(ctx, hash)
	if err != nil {
		if isNoRows(err) {
			return nil, apperr.New(401, apperr.CodeRefreshInvalid, "refresh token invalid or expired", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	if transport.Kind == session.TransportCookie {
		if in.Origin != "" && !d.allowedOrigin(in.Origin) {
			return nil, apperr.New(403, apperr.CodeOriginInvalid, "origin not allowed", nil)
		}
		if in.XCSRFToken == "" {
			return nil, apperr.New(403, apperr.CodeCSRFInvalid, "missing CSRF token", nil)
		}
		if verr := token.VerifyCSRF(d.CSRFKey, in.XCSRFToken, row.FamilyID, row.ID, d.clock().Now()); verr != nil {
			return nil, apperr.New(403, apperr.CodeCSRFInvalid, "invalid CSRF token", nil)
		}
	}

	user, err := q.GetUserByID(ctx, row.UserID)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	ip := clientIP(ctx)
	platform := row.Platform
	deviceName := ""
	if row.DeviceName.Valid {
		deviceName = row.DeviceName.String
	}

	result, err := d.Rotator.Rotate(ctx, d.DB.Write, session.RotateInput{
		RawRefreshToken: rawToken,
		IsSuperadmin:    user.IsSuperadmin,
		DeviceName:      deviceName,
		Platform:        platform,
		IP:              nil,
	})
	if err != nil {
		switch {
		case errors.Is(err, session.ErrRefreshReused):
			if d.RevocationCache != nil {
				d.RevocationCache.MarkRevoked(row.FamilyID)
			}
			return nil, apperr.New(401, apperr.CodeRefreshReused, "refresh token reuse detected, session revoked", nil)
		case errors.Is(err, session.ErrRefreshInvalid):
			return nil, apperr.New(401, apperr.CodeRefreshInvalid, "refresh token invalid or expired", nil)
		default:
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
	}
	_ = ip

	access, err := d.AccessIssuer.IssueAccess(result.UserID, result.FamilyID, user.IsSuperadmin)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	body := dto.LoginResponse{
		AccessToken: access,
		ExpiresIn:   int64(token.AccessTTL.Seconds()),
	}
	out := &dto.RefreshOutput{Body: body}

	if transport.Kind == session.TransportCookie {
		out.SetCookie = RefreshCookie(d.CookieDomain, result.RawRefreshToken, result.ExpiresAt)
		csrf, cerr := token.MintCSRF(d.CSRFKey, result.FamilyID, result.SessionID, result.ExpiresAt)
		if cerr != nil {
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		out.Body.CSRFToken = csrf
	} else {
		out.Body.RefreshToken = result.RawRefreshToken
	}

	return out, nil
}

// RegisterRefresh wires POST /v1/auth/refresh into api.
func RegisterRefresh(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "refresh",
		Method:      "POST",
		Path:        "/v1/auth/refresh",
		Summary:     "Rotate the refresh token",
		Tags:        []string{"auth"},
	}, d.Refresh)
}
