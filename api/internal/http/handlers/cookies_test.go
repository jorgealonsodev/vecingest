package handlers_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/jorgealonsodev/vecingest/internal/http/handlers"
)

// auth-csrf-origin: Refresh Cookie Scope and Flags — "Cookie attributes
// on login" scenario.
func TestRefreshCookie_Attributes(t *testing.T) {
	exp := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	c := handlers.RefreshCookie("api.example.com", "raw-token-value", exp)

	if c.Name != handlers.RefreshCookieName {
		t.Fatalf("expected cookie name %q, got %q", handlers.RefreshCookieName, c.Name)
	}
	if c.Value != "raw-token-value" {
		t.Fatalf("expected the raw token as the cookie value, got %q", c.Value)
	}
	if !c.HttpOnly {
		t.Fatalf("expected HttpOnly")
	}
	if !c.Secure {
		t.Fatalf("expected Secure")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Fatalf("expected SameSite=Strict, got %v", c.SameSite)
	}
	if c.Path != "/v1/auth/refresh" {
		t.Fatalf("expected Path=/v1/auth/refresh, got %q", c.Path)
	}
	if c.Domain != "api.example.com" {
		t.Fatalf("expected Domain=api.example.com, got %q", c.Domain)
	}
	if !c.Expires.Equal(exp) {
		t.Fatalf("expected Expires to match, got %v", c.Expires)
	}
}

// auth-session-tokens: Bearer-Authenticated Logout — "Refresh cookie
// cleared regardless of request path" scenario.
func TestClearedRefreshCookie_MaxAgeZero(t *testing.T) {
	c := handlers.ClearedRefreshCookie("api.example.com")
	if c.Name != handlers.RefreshCookieName {
		t.Fatalf("expected cookie name %q, got %q", handlers.RefreshCookieName, c.Name)
	}
	if c.Path != "/v1/auth/refresh" {
		t.Fatalf("expected Path=/v1/auth/refresh, got %q", c.Path)
	}
	if c.MaxAge != -1 {
		t.Fatalf("expected MaxAge<0 (immediate expiry), got %d", c.MaxAge)
	}
}
