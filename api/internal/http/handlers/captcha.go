package handlers

import (
	"context"
	"time"

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

// captchaOutage* define what "the verification service is currently
// unavailable" means (review lineage review-e72754dc7521b57a,
// R4-captcha-hard-dependency-no-degradation). A single transport
// failure is already proof for the request that observed it; the
// counter exists for the requests that CANNOT produce that proof --
// during an outage the Turnstile widget itself is down, so clients
// arrive with no token at all, which is otherwise indistinguishable
// from an ordinary omitted token.
const (
	captchaOutageThreshold = 3
	captchaOutageWindow    = 60 * time.Second
	captchaOutageKey       = "captcha:outage"
)

// recordCaptchaOutage notes one verifier TRANSPORT failure (never a
// rejected token) on the shared counter. Best-effort: a counter error
// must not turn a degraded captcha into a failed request.
func (d *Deps) recordCaptchaOutage(ctx context.Context) {
	if d.CaptchaOutages == nil {
		return
	}
	_, _ = d.CaptchaOutages.Fail(ctx, captchaOutageKey, captchaOutageWindow)
}

// captchaUnavailable reports whether the verifier has failed often
// enough within the window to treat it as down. A missing counter
// reports false: no evidence of an outage means no degradation.
func (d *Deps) captchaUnavailable(ctx context.Context) bool {
	if d.CaptchaOutages == nil {
		return false
	}
	count, err := d.CaptchaOutages.Count(ctx, captchaOutageKey, captchaOutageWindow)
	return err == nil && count >= captchaOutageThreshold
}

// captchaRequired is the 400 response every Turnstile-guarded endpoint
// returns when verification fails (public-form-protection). A dedicated
// code (never CodeValidation/CodeTooManyAttempts) so a client can
// distinguish "solve the challenge and retry" from every other 4xx.
func captchaRequired() error {
	return apperr.New(400, apperr.CodeCaptchaRequired, "a valid Turnstile token is required", nil)
}
