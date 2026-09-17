// Package captcha declares the CaptchaVerifier seam (design D-7,
// public-form-protection: CaptchaVerifier Interface Abstraction): a
// Turnstile token MUST be verified before any protected public-form
// logic runs, through an interface so the concrete verifier (a real
// Cloudflare Turnstile HTTP client in production, an AlwaysPass double
// in tests) is swappable without changing any caller. This mirrors the
// existing phase-A/phase-B seam shape (Limiter, AttemptCounter, Queue):
// the interface lives here in a domain package; internal/platform/captcha
// holds the concrete implementations.
package captcha

import "context"

// Verifier is the CaptchaVerifier interface. Verify reports whether
// token is a valid, unexpired Turnstile response for a submission from
// remoteIP. A transport or API failure is returned as err, never
// silently treated as a pass -- callers fail closed on infrastructure
// trouble, exactly like every other AttemptCounter/Limiter seam in this
// codebase.
type Verifier interface {
	Verify(ctx context.Context, token, remoteIP string) (bool, error)
}
