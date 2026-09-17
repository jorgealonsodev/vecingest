package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/password"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
	"github.com/jorgealonsodev/vecingest/internal/http/dto"
)

// ForgotPassword implements POST /v1/auth/forgot-password
// (auth-credentials: Enumeration-Safe Auth Responses). The response is
// {"accepted": true} whether or not the email is registered; only a
// real account triggers the (async) reset-token dispatch.
func (d *Deps) ForgotPassword(ctx context.Context, in *dto.ForgotPasswordInput) (*dto.ForgotPasswordOutput, error) {
	// public-form-protection: Turnstile Always Required On Forgot-
	// Password -- unconditional, unlike login's failure-count branch,
	// and checked before any user lookup runs.
	//
	// DEGRADED PATH, deliberate (review lineage
	// review-e72754dc7521b57a): cerr means the VERIFIER is unreachable,
	// which is categorically different from it rejecting a token. This
	// endpoint fails OPEN on that, because the alternative -- what the
	// code did before -- is that a Cloudflare outage denies password
	// recovery to every user for its whole duration. What protects the
	// endpoint meanwhile is unchanged: limiter.LoginReset's per-IP
	// budget wraps the whole group BEFORE this handler runs and is
	// entirely independent of Turnstile, and the response is
	// enumeration-safe either way. Login makes the opposite call
	// (auth_login.go) and says why there.
	// One rule, and the threshold is what decides it: forgot-password
	// degrades open only once a SUSTAINED outage has been recorded
	// (captchaOutageThreshold failures inside captchaOutageWindow).
	//
	// A single transport error no longer opens the endpoint by itself.
	// It used to, which left the threshold constants deciding nothing
	// here; with verifyCaptcha no longer short-circuiting an empty token
	// (captcha.go), tokenless requests now produce that evidence
	// themselves, so the counter is both reachable and load-bearing
	// (R4-captcha-degradation-cannot-engage-without-a-token, review
	// lineage review-c4efc3f92d076299).
	//
	// The cost is explicit: during a genuine outage the first
	// captchaOutageThreshold recovery attempts in a window are still
	// refused, because that is how the outage becomes known. Login makes
	// the opposite call and stays fail-closed throughout
	// (auth_login.go), while still feeding this same counter.
	ok, cerr := d.verifyCaptcha(ctx, in.Body.TurnstileToken, clientIP(ctx))
	if cerr != nil {
		d.recordCaptchaOutage(ctx)
	}
	if !ok && !d.captchaUnavailable(ctx) {
		return nil, captchaRequired()
	}

	q := db.New(d.DB.Write)
	svc := password.ForgotPasswordService{
		Users:     userLookup{q: q},
		Tokens:    d.TokenIssuer,
		Requester: d.ResetRequester,
	}
	result, err := svc.ForgotPassword(ctx, in.Body.Email)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	return &dto.ForgotPasswordOutput{Body: dto.ForgotPasswordResponse{Accepted: result.Accepted}}, nil
}

// ResetPassword implements POST /v1/auth/reset-password. The token is
// single-use (consumed via ConsumePasswordResetToken's conditional
// UPDATE ... WHERE used_at IS NULL) and expires after the requirement's
// 1-hour window; the new password is subject to the same
// PasswordPolicy (conditional length floor + HIBP) as every other
// password write.
func (d *Deps) ResetPassword(ctx context.Context, in *dto.ResetPasswordInput) (*dto.ResetPasswordOutput, error) {
	q := db.New(d.DB.Write)
	hash := token.HashToken(in.Body.Token)

	row, err := q.GetPasswordResetTokenByHash(ctx, hash)
	if err != nil {
		if isNoRows(err) {
			return nil, apperr.New(400, apperr.CodeResetTokenInvalid, "reset token invalid or expired", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if row.UsedAt.Valid || !row.ExpiresAt.After(d.clock().Now()) {
		return nil, apperr.New(400, apperr.CodeResetTokenInvalid, "reset token invalid or expired", nil)
	}

	mfaRow, mfaErr := q.GetUserMFA(ctx, row.UserID)
	totpActive := mfaErr == nil && mfaRow.EnabledAt.Valid

	if perr := d.PasswordPolicy.Validate(ctx, in.Body.NewPassword, totpActive); perr != nil {
		if pe, ok := perr.(*password.PolicyError); ok {
			return nil, apperr.New(422, pe.Code, pe.Error(), pe.Details)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	newHash, err := password.Hash(in.Body.NewPassword)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if err := q.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{ID: row.UserID, PasswordHash: newHash}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if err := q.ConsumePasswordResetToken(ctx, row.ID); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.ResetPasswordOutput{Body: dto.ResetPasswordResponse{Accepted: true}}, nil
}

// RegisterPasswordReset wires forgot/reset password into api.
func RegisterPasswordReset(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "forgotPassword",
		Method:      "POST",
		Path:        "/v1/auth/forgot-password",
		Summary:     "Request a password reset",
		Tags:        []string{"auth"},
	}, d.ForgotPassword)

	huma.Register(api, huma.Operation{
		OperationID: "resetPassword",
		Method:      "POST",
		Path:        "/v1/auth/reset-password",
		Summary:     "Complete a password reset",
		Tags:        []string{"auth"},
	}, d.ResetPassword)
}
