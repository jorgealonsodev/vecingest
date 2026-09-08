package handlers

import (
	"context"
	"net/netip"

	"github.com/danielgtaylor/huma/v2"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/password"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
	"github.com/jorgealonsodev/vecingest/internal/http/dto"
)

// Login implements POST /v1/auth/login (auth-credentials;
// auth-session-tokens: JWT Access and Refresh Issuance). It refuses a
// superadmin account outright: that account has no path through this
// endpoint to satisfy auth-mfa-totp's Mandatory TOTP requirement, so
// allowing it to fully authenticate here would silently bypass the
// separate, TOTP-gated /v1/auth/superadmin/login route.
func (d *Deps) Login(ctx context.Context, in *dto.LoginInput) (*dto.LoginOutput, error) {
	ip := clientIP(ctx)

	locked, err := d.Lockout.IsLocked(ctx, in.Body.Email, ip)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if locked {
		return nil, apperr.New(429, apperr.CodeTooManyAttempts, "too many attempts, try again later", nil)
	}

	q := db.New(d.DB.Write)
	user, err := q.GetUserByEmail(ctx, in.Body.Email)
	if err != nil {
		if !isNoRows(err) {
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		password.VerifyAgainstDummy(ctx, in.Body.Password)
		_, _ = d.Lockout.RecordFailure(ctx, in.Body.Email, ip, false)
		return nil, invalidCredentials()
	}

	ok, needsRehash := password.Verify(ctx, user.PasswordHash, in.Body.Password)
	if !ok {
		_, _ = d.Lockout.RecordFailure(ctx, in.Body.Email, ip, true)
		return nil, invalidCredentials()
	}
	if user.IsSuperadmin {
		// Enumeration-safe: identical error to a wrong password, never
		// revealing that the account exists and is a superadmin.
		_, _ = d.Lockout.RecordFailure(ctx, in.Body.Email, ip, true)
		return nil, invalidCredentials()
	}

	if err := d.Lockout.RecordSuccess(ctx, in.Body.Email); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if needsRehash {
		if newHash, herr := password.Hash(in.Body.Password); herr == nil {
			_ = q.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{ID: user.ID, PasswordHash: newHash})
		}
	}

	return d.issueSession(ctx, q, user.ID, false, in.Body.DeviceName, in.Body.Platform, ip)
}

// issueSession creates a brand-new session (a fresh family_id -- every
// login starts a new family; only rotation shares one) and renders the
// LoginResponse/LoginOutput shape shared by login, superadmin login and
// refresh.
func (d *Deps) issueSession(ctx context.Context, q *db.Queries, userID uuid.UUID, isSuperadmin bool, deviceName, platform, ip string) (*dto.LoginOutput, error) {
	familyID := uuid.New()
	sessionID := uuid.New()
	rawRefresh, refreshHash, err := token.GenerateRefresh()
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	now := d.clock().Now()
	expiresAt := now.Add(token.RefreshLifetime(isSuperadmin))

	var deviceNameCol pgtype.Text
	if deviceName != "" {
		deviceNameCol = pgtype.Text{String: deviceName, Valid: true}
	}
	var ipAddr *netip.Addr
	if parsed, perr := netip.ParseAddr(ip); perr == nil {
		ipAddr = &parsed
	}

	if _, err := q.InsertSession(ctx, db.InsertSessionParams{
		ID:               sessionID,
		UserID:           userID,
		RefreshTokenHash: refreshHash,
		FamilyID:         familyID,
		DeviceName:       deviceNameCol,
		Platform:         platform,
		Ip:               ipAddr,
		ExpiresAt:        expiresAt,
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	access, err := d.AccessIssuer.IssueAccess(userID, familyID, isSuperadmin)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	body := dto.LoginResponse{
		AccessToken: access,
		ExpiresIn:   int64(token.AccessTTL.Seconds()),
	}
	out := &dto.LoginOutput{Body: body}

	if platform == "web" {
		out.SetCookie = RefreshCookie(d.CookieDomain, rawRefresh, expiresAt)
		csrf, cerr := token.MintCSRF(d.CSRFKey, familyID, sessionID, expiresAt)
		if cerr != nil {
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		out.Body.CSRFToken = csrf
	} else {
		out.Body.RefreshToken = rawRefresh
	}

	return out, nil
}

func invalidCredentials() error {
	return apperr.New(401, apperr.CodeInvalidCredentials, "invalid email or password", nil)
}

// clientIP reads the RESOLVED client IP set by router step 2
// (middleware.ClientIPFromXFF), never a raw header (D-H).
func clientIP(ctx context.Context) string {
	return chimiddleware.GetClientIP(ctx)
}

// RegisterLogin wires POST /v1/auth/login into api.
func RegisterLogin(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "login",
		Method:      "POST",
		Path:        "/v1/auth/login",
		Summary:     "Password login",
		Tags:        []string{"auth"},
	}, d.Login)
}
