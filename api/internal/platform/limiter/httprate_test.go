package limiter_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/jorgealonsodev/vecingest/internal/platform/limiter"
)

// resolveClientIP wraps a handler with chi's real ClientIPFromXFF
// middleware so tests exercise the exact resolution mechanism the
// limiter's key function reads from (middleware.GetClientIP), rather
// than a raw header value (request-protection: Trusted-Proxy Client IP
// Resolution is D-H's own concern; this test only needs a resolved IP
// in context to exist).
func resolveClientIP(next http.Handler) http.Handler {
	return middleware.ClientIPFromXFF("10.0.0.1/32")(next)
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func requestFromIP(ip string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	req.Header.Set("X-Forwarded-For", ip+", 10.0.0.1")
	req.RemoteAddr = "10.0.0.1:12345"
	return req
}

// request-protection: Limiter Interface with Rate Budgets — "Login rate
// limit enforced" scenario: the 11th login request in a minute from the
// same client must be rejected with 429.
func TestLoginReset_11thRequestRejected(t *testing.T) {
	handler := resolveClientIP(limiter.LoginReset()(okHandler()))

	for i := 0; i < limiter.LoginResetLimit; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, requestFromIP("203.0.113.9"))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, requestFromIP("203.0.113.9"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected the 11th request within a minute to be 429, got %d", rec.Code)
	}
}

func TestLoginReset_DifferentIPsHaveIndependentBudgets(t *testing.T) {
	handler := resolveClientIP(limiter.LoginReset()(okHandler()))

	for i := 0; i < limiter.LoginResetLimit; i++ {
		handler.ServeHTTP(httptest.NewRecorder(), requestFromIP("203.0.113.9"))
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, requestFromIP("198.51.100.1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("a different resolved client IP must have its own budget, got %d", rec.Code)
	}
}

func TestPerUser_KeysByAuthenticatedUserID(t *testing.T) {
	handler := limiter.PerUser()(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req = req.WithContext(context.WithValue(req.Context(), limiter.UserIDContextKey, "user-1"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
