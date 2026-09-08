// Package limiter implements the phase-A HTTP request-rate budgets
// (design D-H step 8; request-protection: Limiter Interface with Rate
// Budgets). httprate performs NO trusted-proxy resolution of its own
// and its own LimitByRealIP/KeyByRealIP helpers are deprecated as
// spoofable -- never used here. Every key function in this package
// reads the ALREADY-RESOLVED client IP from middleware.GetClientIP,
// which middleware.ClientIPFromXFF (D-H step 2) must have set earlier
// in the chain. Phase B swaps the in-process httprate counter for
// httprate-redis via httprate.WithLimitCounter, with no change to any
// caller of this package -- that option IS the phase-A/phase-B seam,
// so no separate hand-rolled Limiter interface is declared.
package limiter

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
)

// Rate budgets (PRD §6; request-protection: Limiter Interface with Rate
// Budgets).
const (
	// LoginResetLimit/LoginResetWindow: 10 req/min on login and
	// password-reset endpoints.
	LoginResetLimit  = 10
	LoginResetWindow = time.Minute

	// PerUserLimit/PerUserWindow: 300 req/min per authenticated user.
	PerUserLimit  = 300
	PerUserWindow = time.Minute

	// PerIPLimit/PerIPWindow: 60 req/min per IP on public endpoints.
	PerIPLimit  = 60
	PerIPWindow = time.Minute
)

// canonicalClientIPKey keys by the resolved client IP, canonicalized via
// httprate.CanonicalizeIP so an attacker cannot rotate within a
// delegated IPv6 /64 prefix (D-H step 8's exact key construction).
func canonicalClientIPKey(r *http.Request) (string, error) {
	ip := middleware.GetClientIP(r.Context())
	return httprate.CanonicalizeIP(ip), nil
}

// PerIP enforces the public 60 req/min-per-IP budget. Mounted globally
// at D-H step 8, after middleware.ClientIPFromXFF (step 2) so
// canonicalClientIPKey reads a trust-resolved IP, never a raw header.
func PerIP() func(http.Handler) http.Handler {
	return httprate.LimitBy(PerIPLimit, PerIPWindow, canonicalClientIPKey)
}

// LoginReset enforces the stricter 10 req/min budget on login and
// password-reset endpoints. Mounted only on those routes, stacked on
// top of PerIP.
func LoginReset() func(http.Handler) http.Handler {
	return httprate.LimitBy(LoginResetLimit, LoginResetWindow, canonicalClientIPKey)
}

// userIDContextKeyType is an unexported type for UserIDContextKey so no
// other package can collide with this context key by using the same
// underlying type.
type userIDContextKeyType struct{}

// UserIDContextKey is where the Bearer-auth middleware (D-H step 9)
// publishes the authenticated user id so PerUser can key on it. It is
// exported because the auth middleware and this package are in
// different packages and both must agree on the exact key.
var UserIDContextKey = userIDContextKeyType{}

func userIDKey(r *http.Request) (string, error) {
	if v, ok := r.Context().Value(UserIDContextKey).(string); ok && v != "" {
		return v, nil
	}
	// No authenticated user id in context: fall back to the resolved
	// client IP rather than an empty key, which would otherwise pool
	// every unauthenticated caller into a single shared budget.
	return canonicalClientIPKey(r)
}

// PerUser enforces the 300 req/min-per-authenticated-user budget.
// Mounted inside the authenticated route group, after the Bearer-auth
// middleware has published the user id via UserIDContextKey.
func PerUser() func(http.Handler) http.Handler {
	return httprate.LimitBy(PerUserLimit, PerUserWindow, userIDKey)
}
