// Package api assembles the M0 huma API: the chi middleware chain
// (design D-H), route registration, and the per-route middleware step 9
// names (Bearer auth on the authenticated group, the stricter 10/min
// budget on login/reset endpoints). It is the single registration
// function both cmd/vecingest (once its serve subcommand exists) and
// cmd/openapi-gen depend on, so the served API and the generated
// api/openapi/openapi.yaml can never drift apart -- huma emits the spec
// from exactly these registered operations (technical approach: "Go
// structs are the single source").
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/jorgealonsodev/vecingest/internal/health"
	"github.com/jorgealonsodev/vecingest/internal/http/handlers"
	"github.com/jorgealonsodev/vecingest/internal/http/router"
	"github.com/jorgealonsodev/vecingest/internal/platform/limiter"
)

// Config is everything New needs beyond the domain Deps: the global
// middleware chain's own configuration (D-H) and the readiness
// registry (D-Q).
type Config struct {
	Router   router.Config
	Deps     *handlers.Deps
	Registry *health.Registry
	// Title/Version feed the OpenAPI document (api/openapi/openapi.yaml).
	Title   string
	Version string
}

// New builds the full chi.Router + huma.API: D-H's global chain (steps
// 1-8), then every M0 route, each with its own step-9 per-route
// middleware.
func New(cfg Config) (chi.Router, huma.API, error) {
	var hapi huma.API
	r, err := router.New(cfg.Router, func(chiRouter chi.Router) {
		humaConfig := huma.DefaultConfig(cfg.Title, cfg.Version)
		hapi = humachi.New(chiRouter, humaConfig)
		Register(hapi, cfg.Deps, cfg.Registry)
	})
	if err != nil {
		return nil, nil, err
	}
	return r, hapi, nil
}

// Register wires every M0 operation onto api (design D-H step 9 per
// operation, not per chi path): login/reset endpoints get the stricter
// 10 req/min budget (loginRateLimit), the authenticated group gets
// Bearer auth plus the 300 req/min per-user budget
// (bearerAuthAndRateLimit), and health/refresh/refresh-csrf need
// neither.
func Register(hapi huma.API, d *handlers.Deps, registry *health.Registry) {
	loginGroup := huma.NewGroup(hapi)
	loginGroup.UseMiddleware(loginRateLimit())
	handlers.RegisterLogin(loginGroup, d)
	handlers.RegisterSuperadminLogin(loginGroup, d)
	handlers.RegisterPasswordReset(loginGroup, d)

	authGroup := huma.NewGroup(hapi)
	authGroup.UseMiddleware(bearerAuthAndRateLimit(d))
	handlers.RegisterLogout(authGroup, d)
	handlers.RegisterMe(authGroup, d)

	handlers.RegisterRefresh(hapi, d)
	handlers.RegisterRefreshCSRF(hapi, d)
	handlers.RegisterHealth(hapi, registry)
}

// loginRateLimit reuses the already-tested limiter.LoginReset
// http.Handler middleware inside a huma Group middleware via
// humachi.Unwrap, so the exact 10 req/min budget (request-protection:
// Limiter Interface with Rate Budgets) applies to login, superadmin
// login and password-reset requests without a second implementation.
func loginRateLimit() func(huma.Context, func(huma.Context)) {
	limitMW := limiter.LoginReset()
	return func(ctx huma.Context, next func(huma.Context)) {
		req, w := humachi.Unwrap(ctx)
		handler := limitMW(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			next(huma.WithContext(ctx, r.Context()))
		}))
		handler.ServeHTTP(w, req)
	}
}

// bearerAuthAndRateLimit runs the SAME Bearer verification each
// authenticated handler already performs (defense in depth: a handler
// that somehow bypassed this group would still fail its own re-check)
// purely to publish the caller's user id into limiter.UserIDContextKey
// before limiter.PerUser's 300 req/min budget evaluates the request. A
// failed verification here is NOT rendered as a response: it falls
// through to the handler unauthenticated, so the handler's own
// apperr-shaped 401 is what the client actually sees, never a
// differently-shaped middleware error.
func bearerAuthAndRateLimit(d *handlers.Deps) func(huma.Context, func(huma.Context)) {
	limitMW := limiter.PerUser()
	return func(ctx huma.Context, next func(huma.Context)) {
		claims, err := d.Authenticate(ctx.Context(), ctx.Header("Authorization"))
		if err != nil {
			next(ctx)
			return
		}

		req, w := humachi.Unwrap(ctx)
		req = req.WithContext(context.WithValue(req.Context(), limiter.UserIDContextKey, claims.Subject))
		handler := limitMW(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			next(huma.WithContext(ctx, r.Context()))
		}))
		handler.ServeHTTP(w, req)
	}
}
