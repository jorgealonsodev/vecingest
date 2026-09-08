package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/password"
	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
	"github.com/jorgealonsodev/vecingest/internal/http/dto"
)

// SuperadminLogin implements POST /v1/auth/superadmin/login
// (auth-mfa-totp: Mandatory TOTP for Superadmin on a Separate Route).
// It is a route distinct from /v1/auth/login (the requirement's second
// scenario is satisfied structurally, by registering this at its own
// path); a non-superadmin account is rejected with the same generic
// invalid-credentials response Login itself would give, so this
// endpoint never discloses which accounts exist or which are
// superadmins.
func (d *Deps) SuperadminLogin(ctx context.Context, in *dto.SuperadminLoginInput) (*dto.SuperadminLoginOutput, error) {
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
	if err != nil || !user.IsSuperadmin {
		password.VerifyAgainstDummy(ctx, in.Body.Password)
		_, _ = d.Lockout.RecordFailure(ctx, in.Body.Email, ip, err == nil)
		return nil, invalidCredentials()
	}

	ok, needsRehash := password.Verify(ctx, user.PasswordHash, in.Body.Password)
	if !ok {
		_, _ = d.Lockout.RecordFailure(ctx, in.Body.Email, ip, true)
		return nil, invalidCredentials()
	}
	if needsRehash {
		if newHash, herr := password.Hash(in.Body.Password); herr == nil {
			_ = q.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{ID: user.ID, PasswordHash: newHash})
		}
	}

	mfaRow, mfaErr := q.GetUserMFA(ctx, user.ID)
	if mfaErr != nil || !mfaRow.EnabledAt.Valid {
		return nil, apperr.New(403, apperr.CodeMFAEnrollmentRequired, "TOTP enrollment required for superadmin accounts", nil)
	}

	secret, derr := mfa.DecryptSecret(d.MFAKey, mfaRow.TotpSecretEncrypted)
	if derr != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	accepted, throttled, verr := mfa.ThrottledVerify(ctx, d.MFACounter, user.ID, func(ctx context.Context) (bool, error) {
		outcome, err := mfa.VerifyTOTP(ctx, d.DB.Write, d.clock(), user.ID, secret, in.Body.TOTPCode)
		if err != nil {
			return false, err
		}
		return outcome == mfa.OutcomeAccepted, nil
	})
	if verr != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if throttled {
		return nil, apperr.New(429, apperr.CodeTOTPThrottled, "too many TOTP attempts, try again later", nil)
	}
	if !accepted {
		return nil, apperr.New(401, apperr.CodeTOTPInvalid, "invalid TOTP code", nil)
	}

	if err := d.Lockout.RecordSuccess(ctx, in.Body.Email); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	loginOut, err := d.issueSession(ctx, q, user.ID, true, in.Body.DeviceName, in.Body.Platform, ip)
	if err != nil {
		return nil, err
	}
	return &dto.SuperadminLoginOutput{SetCookie: loginOut.SetCookie, Body: loginOut.Body}, nil
}

// RegisterSuperadminLogin wires POST /v1/auth/superadmin/login into api.
func RegisterSuperadminLogin(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "superadminLogin",
		Method:      "POST",
		Path:        "/v1/auth/superadmin/login",
		Summary:     "TOTP-gated superadmin login",
		Tags:        []string{"auth"},
	}, d.SuperadminLogin)
}
