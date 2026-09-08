// Package router builds the M0 HTTP middleware chain (design D-H). The
// order is load-bearing: httprate performs no trusted-proxy resolution
// of its own, so a peer-trust guard and middleware.ClientIPFromXFF MUST
// run before the request logger and the rate limiter for any of them to
// key off a trust-resolved client IP instead of a raw, spoofable header.
// Steps returns the exact ordered, named chain New builds from, so the
// order assertion in router_test.go exercises the same slice production
// code actually uses -- not a parallel, hand-duplicated sequence.
//
// peer-trust-guard exists because middleware.ClientIPFromXFF (chi
// v5.3.2, middleware/client_ip.go:92-117) never inspects r.RemoteAddr:
// it only walks X-Forwarded-For and trusts whatever entries fall within
// the configured CIDRs, so a caller reaching this server directly
// (bypassing the reverse proxy) could claim any X-Forwarded-For value
// and have it accepted verbatim. peer-trust-guard runs first and strips
// the header entirely whenever the immediate TCP peer is not the
// trusted proxy, so ClientIPFromXFF then has nothing spoofed left to
// walk and falls through to no client IP resolved from the header --
// see clientIPMiddleware / GetClientIP callers, which treat an empty
// resolved IP the same as any other untrusted request.
package router

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/jorgealonsodev/vecingest/internal/config"
	"github.com/jorgealonsodev/vecingest/internal/platform/limiter"
)

// ErrProxyIPRequired is returned when APP_ENV is not "development" and
// no PROXY_IP is configured: the dev-only hop-count fallback
// (ClientIPFromXFFTrustedProxies) is strictly weaker than an enumerated
// trusted-proxy CIDR and MUST NOT be used in staging or production
// (D-H).
var ErrProxyIPRequired = errors.New("router: PROXY_IP is required outside APP_ENV=development")

// Config carries what the middleware chain needs to build itself.
type Config struct {
	AppEnv config.AppEnv
	// ProxyIP is the reverse proxy's own IP, installed as a trusted /32
	// (or /128) prefix for middleware.ClientIPFromXFF. Empty is only
	// valid when AppEnv == development.
	ProxyIP string
	// CorsOrigins is the exact allowlist CORS credentials are ever
	// emitted for (D-E: "Access-Control-Allow-Credentials only for
	// https://app.DOMAIN").
	CorsOrigins []string
	// Logger defaults to slog.Default() when nil.
	Logger *slog.Logger
}

// NamedMiddleware pairs a step's design-D-H name with its
// implementation, so the order itself can be asserted by name rather
// than by comparing unnamed function values.
type NamedMiddleware struct {
	Name string
	MW   func(http.Handler) http.Handler
}

// Steps returns D-H's ordered middleware chain, steps 1-8. Step 9
// (per-route Bearer auth, then CSRF on the refresh cookie path only) is
// deliberately NOT part of this global chain: it differs per route and
// is applied at route-registration time instead.
func Steps(cfg Config) ([]NamedMiddleware, error) {
	peerTrustGuardMW, err := peerTrustGuardMiddleware(cfg)
	if err != nil {
		return nil, err
	}
	clientIPMW, err := clientIPMiddleware(cfg)
	if err != nil {
		return nil, err
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	corsOrigins := cfg.CorsOrigins

	return []NamedMiddleware{
		{"request-id", middleware.RequestID},
		{"peer-trust-guard", peerTrustGuardMW},
		{"client-ip", clientIPMW},
		{"logger", requestLogger(logger)},
		{"recoverer", middleware.Recoverer},
		{"security-headers", securityHeaders},
		{"cors", corsMiddleware(corsOrigins)},
		{"cross-origin-protection", crossOriginProtection(corsOrigins)},
		{"limiter", limiter.PerIP()},
	}, nil
}

// New builds a chi.Router with D-H's global chain (steps 1-8) applied in
// order, then invokes mount to register routes (including any per-route
// step-9 middleware).
func New(cfg Config, mount func(r chi.Router)) (chi.Router, error) {
	steps, err := Steps(cfg)
	if err != nil {
		return nil, err
	}
	r := chi.NewRouter()
	for _, step := range steps {
		r.Use(step.MW)
	}
	mount(r)
	return r, nil
}

// trustedProxyPrefix is the single source of truth for the trusted
// reverse-proxy CIDR, consulted by both peerTrustGuardMiddleware and
// clientIPMiddleware so the two mechanisms can never drift into two
// different trust lists. ok is false only for the documented
// APP_ENV=development hop-count fallback, where there is no PROXY_IP
// to enumerate. Outside development, an empty PROXY_IP is refused
// (ErrProxyIPRequired) rather than silently falling back.
func trustedProxyPrefix(cfg Config) (prefix netip.Prefix, ok bool, err error) {
	if cfg.ProxyIP == "" {
		if cfg.AppEnv != config.AppEnvDevelopment {
			return netip.Prefix{}, false, ErrProxyIPRequired
		}
		return netip.Prefix{}, false, nil
	}
	addr, err := netip.ParseAddr(cfg.ProxyIP)
	if err != nil {
		return netip.Prefix{}, false, fmt.Errorf("router: invalid PROXY_IP %q: %w", cfg.ProxyIP, err)
	}
	return netip.PrefixFrom(addr, addr.BitLen()), true, nil
}

// clientIPMiddleware implements D-H step 2 (design.md's own numbering
// is unchanged; peerTrustGuardMiddleware below is a new guard added
// immediately ahead of it in the actual chain, not a renumbering of
// this step). A configured PROXY_IP is installed as a trusted /32
// (v4) or /128 (v6) CIDR for
// middleware.ClientIPFromXFF -- the documented, non-spoofable mechanism.
// Its absence is refused outside APP_ENV=development; inside
// development it falls back to ClientIPFromXFFTrustedProxies(1), a
// strictly weaker hop-count convenience.
func clientIPMiddleware(cfg Config) (func(http.Handler) http.Handler, error) {
	prefix, ok, err := trustedProxyPrefix(cfg)
	if err != nil {
		return nil, err
	}
	if !ok {
		return middleware.ClientIPFromXFFTrustedProxies(1), nil
	}
	return middleware.ClientIPFromXFF(prefix.String()), nil
}

// peerTrustGuardMiddleware implements the new D-H step that runs
// immediately before client-ip (request-protection: "Untrusted
// intermediary rejected as source"). middleware.ClientIPFromXFF never
// inspects r.RemoteAddr, so on its own it accepts a caller-supplied
// X-Forwarded-For value verbatim whenever that caller reaches this
// server directly instead of through the trusted reverse proxy. This
// guard closes that gap by validating trust against the one thing an
// attacker cannot forge -- the TCP peer address: whenever the peer
// isn't the trusted proxy, it strips X-Forwarded-For entirely (so
// client-ip's XFF walk has nothing spoofed left to trust) and resolves
// the client IP from RemoteAddr instead, via
// middleware.ClientIPFromRemoteAddr -- matching the spec's own wording,
// "the system uses the peer's actual connection IP", rather than
// merely leaving no client IP resolved at all. client-ip's own
// ClientIPFromXFF step, immediately after, only overwrites the context
// when it finds a trusted-and-valid XFF entry, so it leaves this
// guard's RemoteAddr-derived value untouched when the header is empty.
//
// It reuses trustedProxyPrefix, the exact same PROXY_IP configuration
// clientIPMiddleware trusts, so the two can never drift apart into two
// lists.
//
// The APP_ENV=development hop-count fallback (no PROXY_IP configured)
// is preserved unchanged: with no enumerable trusted peer to validate
// against, this guard is a no-op in that mode, exactly matching
// clientIPMiddleware's own documented fallback behaviour.
func peerTrustGuardMiddleware(cfg Config) (func(http.Handler) http.Handler, error) {
	prefix, ok, err := trustedProxyPrefix(cfg)
	if err != nil {
		return nil, err
	}
	if !ok {
		return func(next http.Handler) http.Handler { return next }, nil
	}
	return func(next http.Handler) http.Handler {
		fromRemoteAddr := middleware.ClientIPFromRemoteAddr(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !peerIsTrusted(r.RemoteAddr, prefix) {
				r.Header.Del("X-Forwarded-For")
				fromRemoteAddr.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

// peerIsTrusted reports whether host (r.RemoteAddr, "ip:port" or a bare
// IP as some tests set it) falls within prefix. An unparseable
// RemoteAddr is never trusted (fail-closed).
func peerIsTrusted(remoteAddr string, prefix netip.Prefix) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr // RemoteAddr may already be a bare IP (e.g. in tests).
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return prefix.Contains(peer.Unmap())
}

// requestLogger implements D-H step 3: it logs the RESOLVED client IP
// (set by step 2), never a raw header. Redaction of any PII the request
// path/query might carry is the request-protection slogpii Handler's
// job, wired in at composition time around the logger passed here.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.LogAttrs(r.Context(), slog.LevelInfo, "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.Status()),
				slog.String("client_ip", middleware.GetClientIP(r.Context())),
				slog.String("request_id", middleware.GetReqID(r.Context())),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}

// securityHeaders implements D-H step 5 (§6 "middleware secure").
// X-Content-Type-Options: nosniff blocks script-destination EXECUTION
// only -- see D-E's own note that it does not stop the request, the
// cookies, or the response; it is set here for its own, narrower
// purpose and is not relied on to close any wider class by itself.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware implements D-H step 6. Credentials are only ever
// emitted for an Origin present in allowedOrigins (D-E: "credentials:
// true only for https://app.DOMAIN"). Vary: Origin is always set,
// because Access-Control-Allow-Origin is computed FROM the request
// Origin, and omitting Vary would let a shared cache serve one origin's
// credentialed variant to another (the exact cache-mediated CORS bypass
// D-E's refresh/csrf endpoint also guards against).
func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			w.Header().Add("Vary", "Origin")
			if _, ok := allowed[origin]; ok && origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token, Authorization")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// crossOriginProtection implements D-H step 7: stdlib
// net/http.CrossOriginProtection (Go 1.25+) reinforces the Origin/
// Sec-Fetch-Site leg on unsafe methods and is transparent to a native
// client sending neither header (D-E). It issues no token, so it never
// replaces the CSRF token verified at step 9 on the cookie transport.
func crossOriginProtection(trustedOrigins []string) func(http.Handler) http.Handler {
	cop := http.NewCrossOriginProtection()
	for _, origin := range trustedOrigins {
		_ = cop.AddTrustedOrigin(origin)
	}
	return func(next http.Handler) http.Handler {
		return cop.Handler(next)
	}
}
