package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/platform/limiter"
)

// tokenGatedVerifier is a captcha.Verifier double that accepts only one
// specific token value, so tests can distinguish "no token" and "wrong
// token" from "the exact token the test expects" without depending on
// network access to Cloudflare (this session's explicit instruction).
type tokenGatedVerifier struct{ validToken string }

func (v tokenGatedVerifier) Verify(_ context.Context, token, _ string) (bool, error) {
	return token != "" && token == v.validToken, nil
}

// public-form-protection: Turnstile Required After The Third Login
// Failure -- both scenarios. Also covers CaptchaVerifier Interface
// Abstraction's "rejected before any form-specific logic executes":
// the third attempt below uses the CORRECT password with no token and
// must still be rejected as AUTH_CAPTCHA_REQUIRED, never
// AUTH_INVALID_CREDENTIALS -- proof the credential check never even ran.
func TestPublicForm_LoginRequiresTurnstileAfterThirdFailure(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	deps.Captcha = tokenGatedVerifier{validToken: "valid-turnstile-token"}
	client := newClient(srv, nil)

	email := "captcha-login@example.com"
	createUser(t, handlesDB, email, false)

	// Two failed attempts: no captcha required yet.
	for i := 0; i < 2; i++ {
		resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/login", map[string]any{
			"email": email, "password": "wrong-password-here-12345", "platform": "web",
		}, nil)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401 invalid credentials (no captcha required yet), got %d body=%v", i+1, resp.StatusCode, body)
		}
	}

	// Third attempt, CORRECT password, no token: must be rejected as
	// AUTH_CAPTCHA_REQUIRED -- if credential logic ran at all, this
	// would otherwise succeed.
	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/login", map[string]any{
		"email": email, "password": testPassword, "platform": "web",
	}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 captcha-required on the third attempt with no token, got %d body=%v", resp.StatusCode, body)
	}
	if body["code"] != "AUTH_CAPTCHA_REQUIRED" {
		t.Fatalf("expected AUTH_CAPTCHA_REQUIRED, got %v", body)
	}

	// Third attempt, correct password AND a valid token: proceeds to
	// normal credential verification and succeeds.
	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/login", map[string]any{
		"email": email, "password": testPassword, "platform": "web", "turnstile_token": "valid-turnstile-token",
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on the third attempt with a valid token, got %d body=%v", resp.StatusCode, body)
	}
	if body["access_token"] == nil || body["access_token"] == "" {
		t.Fatalf("expected an access_token, got %v", body)
	}
}

// public-form-protection: Turnstile Always Required On Forgot-Password.
func TestPublicForm_ForgotPasswordAlwaysRequiresTurnstile(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	deps.Captcha = tokenGatedVerifier{validToken: "valid-turnstile-token"}
	client := newClient(srv, nil)

	email := "captcha-forgot@example.com"
	createUser(t, handlesDB, email, false)

	// No token at all: rejected, regardless of prior attempt count
	// (this is the FIRST call).
	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/forgot-password", map[string]any{
		"email": email,
	}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 captcha-required with no token, got %d body=%v", resp.StatusCode, body)
	}
	if body["code"] != "AUTH_CAPTCHA_REQUIRED" {
		t.Fatalf("expected AUTH_CAPTCHA_REQUIRED, got %v", body)
	}

	// Valid token: accepted (enumeration-safe shape unchanged).
	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/forgot-password", map[string]any{
		"email": email, "turnstile_token": "valid-turnstile-token",
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with a valid token, got %d body=%v", resp.StatusCode, body)
	}
	if body["accepted"] != true {
		t.Fatalf("expected accepted=true, got %v", body)
	}
}

// public-form-protection: Per-IP Limits Independent Of Turnstile -- an
// IP over its rate limit is still rejected on forgot-password despite a
// valid Turnstile token. limiter.LoginReset's 10 req/min budget wraps
// the WHOLE loginGroup (api.go's Register) BEFORE the handler -- and
// therefore before its captcha check -- ever runs, so this test needs
// no captcha-specific production code; it proves the two mechanisms are
// already structurally independent.
func TestPublicForm_RateLimitIndependentOfValidTurnstile(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	deps.Captcha = tokenGatedVerifier{validToken: "valid-turnstile-token"}
	client := newClient(srv, nil)

	email := "captcha-ratelimit@example.com"
	createUser(t, handlesDB, email, false)

	body := map[string]any{"email": email, "turnstile_token": "valid-turnstile-token"}
	for i := 0; i < limiter.LoginResetLimit; i++ {
		resp, respBody := doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/forgot-password", body, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: expected 200 within budget, got %d body=%v", i+1, resp.StatusCode, respBody)
		}
	}

	resp, respBody := doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/forgot-password", body, nil)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 once the per-IP budget is exceeded, despite a valid Turnstile token, got %d body=%v", resp.StatusCode, respBody)
	}
}

// public-form-protection: Company Registration And Contact Forms Out Of
// M1 Scope -- the M1 API surface carries no register-company or public
// contact-form operation.
func TestPublicForm_NoRegisterCompanyOrContactFormInAPISurface(t *testing.T) {
	srv, _, _ := newTestServer(t)
	client := newClient(srv, nil)

	resp, err := client.Get(srv.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("fetch openapi.json: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 fetching openapi.json, got %d", resp.StatusCode)
	}

	buf := make([]byte, 0, 1<<20)
	tmp := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if rerr != nil {
			break
		}
	}
	spec := string(buf)

	if strings.Contains(spec, "register-company") {
		t.Fatalf("expected no register-company operation in the M1 API surface")
	}
	if strings.Contains(spec, "contact-form") || strings.Contains(spec, "contact_form") {
		t.Fatalf("expected no public contact-form operation in the M1 API surface")
	}

	// Also confirm no such route is even served (belt and suspenders: a
	// Hidden operation would be absent from openapi.json above but
	// still routed -- authz's own A3 check catches that at boot, this
	// just double-checks at the HTTP layer for this specific path).
	resp2, _ := doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/register-company", map[string]any{}, nil)
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for a nonexistent register-company route, got %d", resp2.StatusCode)
	}
}
