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
