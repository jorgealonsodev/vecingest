package captcha_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/platform/captcha"
)

// fakeTurnstile stands in for Cloudflare's real siteverify endpoint so
// this test never depends on network access (this session's explicit
// instruction: "Tests must not depend on network access").
func fakeTurnstile(t *testing.T, success bool, wantSecret string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.PostForm.Get("secret") != wantSecret {
			t.Fatalf("expected secret %q, got %q", wantSecret, r.PostForm.Get("secret"))
		}
		w.Header().Set("Content-Type", "application/json")
		if success {
			_, _ = w.Write([]byte(`{"success":true}`))
		} else {
			_, _ = w.Write([]byte(`{"success":false,"error-codes":["invalid-input-response"]}`))
		}
	}))
}

// redirectTransport rewrites every outgoing request's scheme/host to
// target, so captcha.Turnstile's unconditional real siteVerifyURL
// constant is exercised against the local fake server below instead of
// a real network call to Cloudflare -- these tests cover the SAME
// Verify implementation production code runs, not a copy of it.
type redirectTransport struct{ target string }

func (r redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	targetURL, err := url.Parse(r.target)
	if err != nil {
		return nil, err
	}
	req = req.Clone(req.Context())
	req.URL.Scheme = targetURL.Scheme
	req.URL.Host = targetURL.Host
	req.Host = targetURL.Host
	return http.DefaultTransport.RoundTrip(req)
}

func clientTo(target string) *http.Client {
	return &http.Client{Transport: redirectTransport{target: target}}
}

// TestTurnstile_ValidTokenAccepted proves the real captcha.Turnstile
// type accepts a token Cloudflare's siteverify endpoint reports as
// valid (public-form-protection: CaptchaVerifier Interface Abstraction).
func TestTurnstile_ValidTokenAccepted(t *testing.T) {
	srv := fakeTurnstile(t, true, "test-secret")
	defer srv.Close()

	verifier := captcha.Turnstile{Secret: "test-secret", Client: clientTo(srv.URL)}

	ok, err := verifier.Verify(context.Background(), "valid-token", "203.0.113.9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected a valid token to be accepted")
	}
}

// TestTurnstile_InvalidTokenRejected proves an invalid token is
// rejected, never silently accepted (public-form-protection: Invalid
// Turnstile token rejected before form logic runs).
func TestTurnstile_InvalidTokenRejected(t *testing.T) {
	srv := fakeTurnstile(t, false, "test-secret")
	defer srv.Close()

	verifier := captcha.Turnstile{Secret: "test-secret", Client: clientTo(srv.URL)}

	ok, err := verifier.Verify(context.Background(), "invalid-token", "203.0.113.9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("expected an invalid token to be rejected")
	}
}

// TestTurnstile_NonOKStatusIsAnError proves a Cloudflare-side failure
// (non-200) is surfaced as an error, never coerced into "valid" --
// callers must fail closed on infrastructure trouble.
func TestTurnstile_NonOKStatusIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	verifier := captcha.Turnstile{Secret: "test-secret", Client: clientTo(srv.URL)}

	ok, err := verifier.Verify(context.Background(), "any-token", "203.0.113.9")
	if err == nil {
		t.Fatalf("expected an error on a non-200 response")
	}
	if ok {
		t.Fatalf("expected ok=false alongside the error")
	}
}

// TestAlwaysPass_DefaultAcceptsAndFailFlagRejects covers the test
// double every non-Turnstile-focused test injects (design D-7).
func TestAlwaysPass_DefaultAcceptsAndFailFlagRejects(t *testing.T) {
	pass := captcha.AlwaysPass{}
	ok, err := pass.Verify(context.Background(), "anything", "203.0.113.9")
	if err != nil || !ok {
		t.Fatalf("expected the zero-value AlwaysPass to accept, got ok=%v err=%v", ok, err)
	}

	fail := captcha.AlwaysPass{Fail: true}
	ok, err = fail.Verify(context.Background(), "anything", "203.0.113.9")
	if err != nil || ok {
		t.Fatalf("expected AlwaysPass{Fail: true} to reject, got ok=%v err=%v", ok, err)
	}
}
