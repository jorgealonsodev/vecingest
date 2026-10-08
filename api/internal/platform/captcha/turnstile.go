// Package captcha implements the phase-A concrete CaptchaVerifier
// (design D-7): a real Cloudflare Turnstile HTTP client, and the
// AlwaysPass test double every non-Turnstile-focused test injects
// instead of a real network call. Turnstile has no documented phase-B
// swap (unlike Limiter/AttemptCounter/Queue), so there is only one real
// implementation here.
package captcha

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// siteVerifyURL is Cloudflare Turnstile's siteverify endpoint (public
// API surface, not a secret).
const siteVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// verifyTimeout bounds how long a single verification call may block a
// request. It is applied to the request CONTEXT, not only to the
// default client, so an injected Client with no Timeout of its own is
// bounded too -- and it is deliberately short: during a siteverify
// outage every one of these holds a request slot on an endpoint that is
// already a credential-stuffing target, and the caller's degraded path
// (handlers.captchaUnavailable) only starts once the call has returned
// (review lineage review-e72754dc7521b57a).
const verifyTimeout = 2 * time.Second

// Turnstile implements captcha.Verifier against Cloudflare's real HTTP
// API. Secret is TURNSTILE_SECRET (internal/config, config.go:67) and is
// never logged: every error path below wraps only the HTTP status code
// or a transport error, never the request body or the secret value.
type Turnstile struct {
	Secret string
	// Client is optional; a zero value gets a bounded-timeout default.
	Client *http.Client
}

func (t Turnstile) client() *http.Client {
	if t.Client != nil {
		return t.Client
	}
	return &http.Client{Timeout: verifyTimeout}
}

type siteVerifyResponse struct {
	Success bool `json:"success"`
}

// Verify calls Cloudflare's siteverify endpoint with token and
// remoteIP. A non-2xx response, a transport failure, or an undecodable
// body is returned as an error -- never coerced into a false "pass" or
// a false "fail" -- so a caller can distinguish "rejected" from
// "verification service unavailable" and fail closed on the latter.
func (t Turnstile) Verify(ctx context.Context, token, remoteIP string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()

	form := url.Values{}
	form.Set("secret", t.Secret)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, siteVerifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false, fmt.Errorf("captcha: build turnstile request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.client().Do(req)
	if err != nil {
		return false, fmt.Errorf("captcha: turnstile request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("captcha: turnstile responded with status %d", resp.StatusCode)
	}

	var parsed siteVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return false, fmt.Errorf("captcha: decode turnstile response: %w", err)
	}
	return parsed.Success, nil
}

// AlwaysPass is the test double captcha.Verifier every test not
// specifically exercising Turnstile injects, so no test depends on
// network access to Cloudflare (design D-7). The zero value always
// accepts; set Fail to true to exercise the rejection path without a
// real invalid token.
type AlwaysPass struct{ Fail bool }

// Verify never inspects token or remoteIP: it is a pure double.
func (a AlwaysPass) Verify(_ context.Context, _, _ string) (bool, error) {
	return !a.Fail, nil
}
