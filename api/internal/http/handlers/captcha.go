package handlers

import (
	"context"

	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
)

// captchaAfterFailures is the failure count AFTER WHICH the next login
// attempt (the third) must carry a valid Turnstile token
// (public-form-protection: Turnstile Required After The Third Login
// Failure -- "starting on the third failed attempt", i.e. after 2 prior
// failures).
const captchaAfterFailures = 2

// verifyCaptcha reports whether token is a valid Turnstile response.
// d.Captcha == nil or an empty token both fail closed (false, nil
// error) -- a deployment that forgets to wire Captcha, or a caller who
// omits the token entirely, is rejected exactly like an invalid token,
// never silently allowed through.
func (d *Deps) verifyCaptcha(ctx context.Context, token, remoteIP string) (bool, error) {
	if d.Captcha == nil || token == "" {
		return false, nil
	}
	return d.Captcha.Verify(ctx, token, remoteIP)
}

// captchaRequired is the 400 response every Turnstile-guarded endpoint
// returns when verification fails (public-form-protection). A dedicated
// code (never CodeValidation/CodeTooManyAttempts) so a client can
// distinguish "solve the challenge and retry" from every other 4xx.
func captchaRequired() error {
	return apperr.New(400, apperr.CodeCaptchaRequired, "a valid Turnstile token is required", nil)
}
