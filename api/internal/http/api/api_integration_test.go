package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/config"
	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/lockout"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/password"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/session"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
	"github.com/jorgealonsodev/vecingest/internal/health"
	httpapi "github.com/jorgealonsodev/vecingest/internal/http/api"
	"github.com/jorgealonsodev/vecingest/internal/http/handlers"
	"github.com/jorgealonsodev/vecingest/internal/http/router"
	"github.com/jorgealonsodev/vecingest/internal/platform/attempts"
	"github.com/jorgealonsodev/vecingest/internal/platform/cache"
	"github.com/jorgealonsodev/vecingest/internal/platform/mail"
	"github.com/jorgealonsodev/vecingest/internal/testhelpers"
)

const (
	testCookieDomain = "127.0.0.1"
	testOrigin       = "https://app.example.com"
	testPassword     = "correct-horse-battery-staple-1"
)

// xffTransport injects a two-hop X-Forwarded-For header on every
// request so the dev-only ClientIPFromXFFTrustedProxies(1) fallback
// resolves a stable, realistic "client" IP (203.0.113.9) instead of ""
// -- exercising the exact D-H mechanism a real reverse-proxy deployment
// would, not a bypass of it.
type xffTransport struct{ base http.RoundTripper }

func (t xffTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("X-Forwarded-For", "10.0.0.1, 203.0.113.9")
	return t.base.RoundTrip(r)
}

func newTestServer(t *testing.T) (*httptest.Server, *handlers.Deps, db.Handles) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}

	handlesDB, _ := testhelpers.AppRWHandles(t)

	accessSecret := []byte("test-jwt-secret-32-bytes-long-enough")
	refreshSecret := []byte("test-jwt-refresh-secret-32-bytes-ok")
	csrfKey, err := token.DeriveCSRFKey(refreshSecret)
	if err != nil {
		t.Fatalf("derive csrf key: %v", err)
	}

	logMailer := mail.LogMailer{}
	deps := &handlers.Deps{
		DB:           handlesDB,
		AccessIssuer: token.Issuer{Secret: accessSecret},
		CSRFKey:      csrfKey,
		Rotator:      session.Rotator{},
		Lockout: lockout.Service{
			Counter: attempts.NewCounter(nil),
			Mailer:  handlers.LockoutMailer{Sender: logMailer},
		},
		PasswordPolicy:  password.PasswordPolicy{},
		MFACounter:      attempts.NewCounter(nil),
		RevocationCache: cache.New(cache.SessionsFamilyChecker{DB: handlesDB.Write}, token.AccessTTL, nil),
		CookieDomain:    testCookieDomain,
		AllowedOrigins:  []string{testOrigin},
		ResetRequester:  handlers.DBResetRequester{DB: handlesDB.Write, Sender: logMailer},
		TokenIssuer:     handlers.OpaqueTokenIssuer{},
	}

	registry := health.NewRegistry(health.PostgresCheck{DB: handlesDB.Write})

	r, _, err := httpapi.New(httpapi.Config{
		Router: router.Config{
			AppEnv:      config.AppEnvDevelopment,
			CorsOrigins: []string{testOrigin},
		},
		Deps:     deps,
		Registry: registry,
		Title:    "Vecingest API (test)",
		Version:  "test",
	})
	if err != nil {
		t.Fatalf("build api: %v", err)
	}

	srv := httptest.NewTLSServer(r)
	t.Cleanup(srv.Close)
	return srv, deps, handlesDB
}

// createUser inserts a real user row with a real Argon2id hash so login
// exercises the actual verification path, not a shortcut.
func createUser(t *testing.T, handlesDB db.Handles, email string, isSuperadmin bool) uuid.UUID {
	t.Helper()
	hash, err := password.Hash(testPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	q := db.New(handlesDB.Write)
	user, err := q.InsertUser(t.Context(), db.InsertUserParams{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: hash,
		Name:         "Test User",
		Locale:       "es",
		IsSuperadmin: isSuperadmin,
	})
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return user.ID
}

func newClient(srv *httptest.Server, jar *cookiejar.Jar) *http.Client {
	base := srv.Client()
	client := &http.Client{
		Transport: xffTransport{base: base.Transport},
		Timeout:   10 * time.Second,
	}
	// A typed-nil *cookiejar.Jar assigned directly to the Jar interface
	// field would be a non-nil interface wrapping a nil pointer, which
	// panics inside net/http.Client.send. Only set it when non-nil.
	if jar != nil {
		client.Jar = jar
	}
	return client
}

func doJSON(t *testing.T, client *http.Client, method, url string, body any, headers map[string]string) (*http.Response, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var parsed map[string]any
	dec := json.NewDecoder(resp.Body)
	_ = dec.Decode(&parsed) // best-effort; some responses have no body
	return resp, parsed
}

func TestAuthFlow_LoginRefreshLogout(t *testing.T) {
	srv, _, handlesDB := newTestServer(t)
	email := "web-user@example.com"
	createUser(t, handlesDB, email, false)

	jar, _ := cookiejar.New(nil)
	client := newClient(srv, jar)

	// --- Login (web transport) ---
	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/login", map[string]any{
		"email": email, "password": testPassword, "platform": "web",
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on login, got %d body=%v", resp.StatusCode, body)
	}
	accessToken, _ := body["access_token"].(string)
	csrfToken, _ := body["csrf_token"].(string)
	if accessToken == "" || csrfToken == "" {
		t.Fatalf("expected access_token and csrf_token in body, got %v", body)
	}
	if _, ok := body["refresh_token"]; ok {
		t.Fatalf("web transport must not carry refresh_token in the body, got %v", body)
	}

	refreshURLReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/auth/refresh", nil)
	found := false
	for _, c := range jar.Cookies(refreshURLReq.URL) {
		if c.Name == handlers.RefreshCookieName {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the refresh cookie to be attached to a request under /v1/auth/refresh")
	}

	// --- GET /v1/me ---
	resp, body = doJSON(t, client, http.MethodGet, srv.URL+"/v1/me", nil, map[string]string{
		"Authorization": "Bearer " + accessToken,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on /v1/me, got %d body=%v", resp.StatusCode, body)
	}
	if body["email"] != email || body["is_superadmin"] != false {
		t.Fatalf("unexpected /v1/me body: %v", body)
	}
	if _, ok := body["role"]; ok {
		t.Fatalf("M0 /v1/me MUST NOT include role, got %v", body)
	}

	// --- Refresh: missing CSRF header rejected ---
	resp, _ = doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/refresh", map[string]any{}, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 on refresh with cookie but no CSRF header, got %d", resp.StatusCode)
	}

	// --- Refresh: valid cookie + valid CSRF rotates ---
	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/refresh", map[string]any{}, map[string]string{
		"X-CSRF-Token": csrfToken,
		"Origin":       testOrigin,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on valid refresh, got %d body=%v", resp.StatusCode, body)
	}
	newAccessToken, _ := body["access_token"].(string)
	newCSRFToken, _ := body["csrf_token"].(string)
	if newAccessToken == "" || newCSRFToken == "" || newCSRFToken == csrfToken {
		t.Fatalf("expected a fresh access token and a DIFFERENT csrf token, got %v", body)
	}

	// --- Refresh: foreign Origin rejected ---
	resp, _ = doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/refresh", map[string]any{}, map[string]string{
		"X-CSRF-Token": newCSRFToken,
		"Origin":       "https://attacker.example",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for a foreign Origin, got %d", resp.StatusCode)
	}

	// --- GET /v1/auth/refresh/csrf: valid cookie restores a token ---
	resp, body = doJSON(t, client, http.MethodGet, srv.URL+"/v1/auth/refresh/csrf", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on GET refresh/csrf, got %d body=%v", resp.StatusCode, body)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("expected Cache-Control: no-store, got %q", resp.Header.Get("Cache-Control"))
	}
	if resp.Header.Get("Vary") == "" {
		t.Fatalf("expected a Vary header naming Origin, got empty")
	}
	if resp.Header.Get("Cross-Origin-Resource-Policy") != "same-origin" {
		t.Fatalf("expected Cross-Origin-Resource-Policy: same-origin, got %q", resp.Header.Get("Cross-Origin-Resource-Policy"))
	}

	// --- Logout: Bearer-authenticated, clears cookie ---
	resp, _ = doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/logout", nil, map[string]string{
		"Authorization": "Bearer " + newAccessToken,
	})
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 200/204 on logout, got %d", resp.StatusCode)
	}

	// --- The just-revoked access token is rejected immediately ---
	resp, _ = doJSON(t, client, http.MethodGet, srv.URL+"/v1/me", nil, map[string]string{
		"Authorization": "Bearer " + newAccessToken,
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for an access token from a just-revoked session, got %d", resp.StatusCode)
	}
}

func TestAuthFlow_RefreshReuseRevokesFamily(t *testing.T) {
	srv, _, handlesDB := newTestServer(t)
	email := "reuse-user@example.com"
	createUser(t, handlesDB, email, false)

	jar, _ := cookiejar.New(nil)
	client := newClient(srv, jar)

	_, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/login", map[string]any{
		"email": email, "password": testPassword, "platform": "web",
	}, nil)
	accessToken, _ := body["access_token"].(string)
	csrfToken, _ := body["csrf_token"].(string)

	// Capture the FIRST-generation cookie before rotating, so it can be
	// replayed after rotation makes it stale.
	reqURL, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/auth/refresh", nil)
	var staleCookie *http.Cookie
	for _, c := range jar.Cookies(reqURL.URL) {
		if c.Name == handlers.RefreshCookieName {
			cp := *c
			staleCookie = &cp
		}
	}
	if staleCookie == nil {
		t.Fatalf("expected a refresh cookie after login")
	}

	// Rotate once (this replaces the cookie in the jar).
	resp, _ := doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/refresh", map[string]any{}, map[string]string{
		"X-CSRF-Token": csrfToken, "Origin": testOrigin,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected first rotation to succeed, got %d", resp.StatusCode)
	}

	// Replay the STALE (already-rotated-away) cookie manually.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/auth/refresh", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(staleCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 203.0.113.9")
	resp2, err := newClient(srv, nil).Do(req)
	if err != nil {
		t.Fatalf("replay request: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 AUTH_REFRESH_REUSED on stale-token replay, got %d", resp2.StatusCode)
	}

	// The whole family is now revoked: even the FIRST access token must
	// be rejected immediately.
	resp3, _ := doJSON(t, client, http.MethodGet, srv.URL+"/v1/me", nil, map[string]string{
		"Authorization": "Bearer " + accessToken,
	})
	if resp3.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected reuse to revoke the whole family, but the original access token still works (status %d)", resp3.StatusCode)
	}
}

func TestAuthFlow_RefreshBothTransportsRejected(t *testing.T) {
	srv, _, handlesDB := newTestServer(t)
	email := "ambiguous-user@example.com"
	createUser(t, handlesDB, email, false)

	jar, _ := cookiejar.New(nil)
	client := newClient(srv, jar)
	_, _ = doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/login", map[string]any{
		"email": email, "password": testPassword, "platform": "web",
	}, nil)

	resp, _ := doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/refresh", map[string]any{
		"refresh_token": "irrelevant-body-token",
	}, map[string]string{"X-CSRF-Token": "irrelevant"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 AUTH_AMBIGUOUS_TOKEN_TRANSPORT when both cookie and body are present, got %d", resp.StatusCode)
	}
}

func TestAuthFlow_MobileLoginGetsBodyRefreshToken(t *testing.T) {
	srv, _, handlesDB := newTestServer(t)
	email := "mobile-user@example.com"
	createUser(t, handlesDB, email, false)

	client := newClient(srv, nil)
	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/login", map[string]any{
		"email": email, "password": testPassword, "platform": "ios",
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%v", resp.StatusCode, body)
	}
	if _, ok := body["csrf_token"]; ok {
		t.Fatalf("native transport must never receive a csrf_token, got %v", body)
	}
	rt, _ := body["refresh_token"].(string)
	if rt == "" {
		t.Fatalf("expected refresh_token in body for native transport, got %v", body)
	}

	// Body-transport refresh: no CSRF required.
	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/refresh", map[string]any{
		"refresh_token": rt,
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on body-transport refresh with no CSRF header, got %d body=%v", resp.StatusCode, body)
	}
}

func TestAuthFlow_WrongPasswordRejected(t *testing.T) {
	srv, _, handlesDB := newTestServer(t)
	email := "badpass-user@example.com"
	createUser(t, handlesDB, email, false)

	client := newClient(srv, nil)
	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/login", map[string]any{
		"email": email, "password": "totally-wrong-password-here", "platform": "web",
	}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%v", resp.StatusCode, body)
	}
	if body["code"] != "AUTH_INVALID_CREDENTIALS" {
		t.Fatalf("expected AUTH_INVALID_CREDENTIALS, got %v", body)
	}
}

func TestAuthFlow_SuperadminWithoutMFAEnrollmentRequired(t *testing.T) {
	srv, _, handlesDB := newTestServer(t)
	email := "super-user@example.com"
	createUser(t, handlesDB, email, true)

	client := newClient(srv, nil)
	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/superadmin/login", map[string]any{
		"email": email, "password": testPassword, "platform": "web", "totp_code": "000000",
	}, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 MFA enrollment required, got %d body=%v", resp.StatusCode, body)
	}
	if body["code"] != "AUTH_MFA_ENROLLMENT_REQUIRED" {
		t.Fatalf("expected AUTH_MFA_ENROLLMENT_REQUIRED, got %v", body)
	}
}

func TestAuthFlow_RegularLoginRejectsSuperadmin(t *testing.T) {
	srv, _, handlesDB := newTestServer(t)
	email := "super-regular-login@example.com"
	createUser(t, handlesDB, email, true)

	client := newClient(srv, nil)
	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/login", map[string]any{
		"email": email, "password": testPassword, "platform": "web",
	}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected the regular login route to refuse a superadmin account, got %d body=%v", resp.StatusCode, body)
	}
}

func TestAuthFlow_ForgotPasswordEnumerationSafe(t *testing.T) {
	srv, _, handlesDB := newTestServer(t)
	email := "forgot-user@example.com"
	createUser(t, handlesDB, email, false)

	client := newClient(srv, nil)
	resp1, body1 := doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/forgot-password", map[string]any{"email": email}, nil)
	resp2, body2 := doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/forgot-password", map[string]any{"email": "no-such-user@example.com"}, nil)

	if resp1.StatusCode != resp2.StatusCode || resp1.StatusCode != http.StatusOK {
		t.Fatalf("expected identical 200 status for both, got %d and %d", resp1.StatusCode, resp2.StatusCode)
	}
	if body1["accepted"] != true || body2["accepted"] != true {
		t.Fatalf("expected accepted:true for both regardless of registration, got %v / %v", body1, body2)
	}
}

func TestAuthFlow_SessionsListAndRevoke(t *testing.T) {
	srv, _, handlesDB := newTestServer(t)
	emailA := "sessions-user-a@example.com"
	emailB := "sessions-user-b@example.com"
	createUser(t, handlesDB, emailA, false)
	createUser(t, handlesDB, emailB, false)

	clientA := newClient(srv, nil)
	_, bodyA := doJSON(t, clientA, http.MethodPost, srv.URL+"/v1/auth/login", map[string]any{
		"email": emailA, "password": testPassword, "platform": "ios",
	}, nil)
	accessA, _ := bodyA["access_token"].(string)

	clientB := newClient(srv, nil)
	_, bodyB := doJSON(t, clientB, http.MethodPost, srv.URL+"/v1/auth/login", map[string]any{
		"email": emailB, "password": testPassword, "platform": "ios",
	}, nil)
	accessB, _ := bodyB["access_token"].(string)

	// A lists their own sessions.
	resp, body := doJSON(t, clientA, http.MethodGet, srv.URL+"/v1/me/sessions", nil, map[string]string{
		"Authorization": "Bearer " + accessA,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%v", resp.StatusCode, body)
	}
	sessions, _ := body["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("expected exactly 1 live session for A, got %v", body)
	}
	first, _ := sessions[0].(map[string]any)
	familyID, _ := first["family_id"].(string)
	if familyID == "" {
		t.Fatalf("expected a family_id in the session summary, got %v", first)
	}
	if _, ok := first["refresh_token_hash"]; ok {
		t.Fatalf("session summary must never include a token hash, got %v", first)
	}

	// B cannot revoke A's session: 404, never 403.
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/me/sessions/"+familyID, nil)
	req.Header.Set("Authorization", "Bearer "+accessB)
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 203.0.113.9")
	resp, err := clientB.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for a foreign family id, got %d", resp.StatusCode)
	}

	// A revokes their own session.
	req, _ = http.NewRequest(http.MethodDelete, srv.URL+"/v1/me/sessions/"+familyID, nil)
	req.Header.Set("Authorization", "Bearer "+accessA)
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 203.0.113.9")
	resp, err = clientA.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 200/204 revoking own session, got %d", resp.StatusCode)
	}
}

func TestHealth_LiveAndReady(t *testing.T) {
	srv, _, _ := newTestServer(t)
	client := newClient(srv, nil)

	resp, body := doJSON(t, client, http.MethodGet, srv.URL+"/v1/health/live", nil, nil)
	if resp.StatusCode != http.StatusOK || body["ok"] != true {
		t.Fatalf("expected 200 {ok:true} from /v1/health/live, got %d %v", resp.StatusCode, body)
	}

	resp, body = doJSON(t, client, http.MethodGet, srv.URL+"/v1/health/ready", nil, nil)
	if resp.StatusCode != http.StatusOK || body["ok"] != true {
		t.Fatalf("expected 200 {ok:true} from /v1/health/ready, got %d %v", resp.StatusCode, body)
	}
}

// api-contract-generation / task 5.10: every dto input struct rejects
// an unknown field at request time (huma's registry-wide
// AllowAdditionalPropertiesByDefault=false), not merely documenting
// additionalProperties:false in the schema.
func TestLogin_UnknownFieldRejected(t *testing.T) {
	srv, _, handlesDB := newTestServer(t)
	email := "unknownfield-user@example.com"
	createUser(t, handlesDB, email, false)

	client := newClient(srv, nil)
	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/login", map[string]any{
		"email": email, "password": testPassword, "platform": "web",
		"totally_unexpected_field": "should be rejected",
	}, nil)
	if resp.StatusCode != http.StatusUnprocessableEntity && resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected an unknown field to be rejected (422/400), got %d body=%v", resp.StatusCode, body)
	}
}
