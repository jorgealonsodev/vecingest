// Package handlers implements the M0 HTTP request handlers (design D-D,
// D-E, D-N, D-P, D-Q; huma-registered operations). Handlers contain
// wiring only: the domain packages under internal/domain/auth own every
// policy decision (PRD §9: "handlers no logic").
package handlers

import (
	"net/http"
	"time"
)

// RefreshCookieName is the single cookie this API ever sets (D-E: "there
// is exactly one cookie, and the CSRF token is not it").
const RefreshCookieName = "vecingest_refresh"

// RefreshCookiePath scopes the cookie so RFC 6265 §5.1.4 path-matching
// sends it only to /v1/auth/refresh and its subpaths (D-E), never to
// /v1/auth/csrf or /v1/auth/logout.
const RefreshCookiePath = "/v1/auth/refresh"

// RefreshCookie builds the Set-Cookie value for a successful login or
// refresh (auth-csrf-origin: Refresh Cookie Scope and Flags).
func RefreshCookie(domain, rawToken string, expiresAt time.Time) http.Cookie {
	return http.Cookie{
		Name:     RefreshCookieName,
		Value:    rawToken,
		Path:     RefreshCookiePath,
		Domain:   domain,
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	}
}

// ClearedRefreshCookie builds a Set-Cookie that deletes the refresh
// cookie regardless of the request path (auth-session-tokens:
// Bearer-Authenticated Logout — "setting a cookie is not path-matched
// the way sending one is").
func ClearedRefreshCookie(domain string) http.Cookie {
	return http.Cookie{
		Name:     RefreshCookieName,
		Value:    "",
		Path:     RefreshCookiePath,
		Domain:   domain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	}
}
