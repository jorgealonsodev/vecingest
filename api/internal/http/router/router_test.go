package router_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/jorgealonsodev/vecingest/internal/config"
	"github.com/jorgealonsodev/vecingest/internal/http/router"
)

// request-protection: Trusted-Proxy Client IP Resolution — the
// middleware order itself is the thing under test (design D-H): if
// step 2 (client-ip) is ever dropped or moved, the chain silently
// reverts to spoofable behaviour, so this asserts the exact sequence
// rather than merely that each step exists somewhere.
func TestSteps_OrderMatchesD_H(t *testing.T) {
	steps, err := router.Steps(router.Config{
		AppEnv:      config.AppEnvProduction,
		ProxyIP:     "172.18.0.2",
		CorsOrigins: []string{"https://app.example.com"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{
		"request-id",
		"peer-trust-guard",
		"client-ip",
		"logger",
		"recoverer",
		"security-headers",
		"cors",
		"cross-origin-protection",
		"limiter",
	}
	if len(steps) != len(want) {
		t.Fatalf("expected %d steps, got %d", len(want), len(steps))
	}
	for i, name := range want {
		if steps[i].Name != name {
			t.Fatalf("step %d: expected %q, got %q (full order: %v)", i, name, steps[i].Name, names(steps))
		}
	}
	// The order-defining property this whole test exists for: client-ip
	// MUST precede logger, limiter and (per-route) CSRF verification.
	// peer-trust-guard MUST precede client-ip -- it strips a
	// caller-supplied X-Forwarded-For when the peer isn't the trusted
	// proxy, so it has to run before ClientIPFromXFF ever reads that
	// header. A future reordering that puts it after client-ip must
	// fail this assertion loudly, not silently reopen the spoof.
	peerTrustIdx := indexOf(steps, "peer-trust-guard")
	clientIPIdx := indexOf(steps, "client-ip")
	loggerIdx := indexOf(steps, "logger")
	limiterIdx := indexOf(steps, "limiter")
	if !(peerTrustIdx < clientIPIdx && clientIPIdx < loggerIdx && clientIPIdx < limiterIdx) {
		t.Fatalf("peer-trust-guard must precede client-ip, and client-ip must precede logger and limiter, got order %v", names(steps))
	}
}

// buildClientIPChain composes the real, ordered peer-trust-guard ->
// client-ip sub-chain from steps, so tests exercise the exact
// production sequence instead of a single middleware in isolation --
// the untrusted-peer scenario only reproduces when both steps run
// together, in order.
func buildClientIPChain(t *testing.T, steps []router.NamedMiddleware, final http.Handler) http.Handler {
	t.Helper()
	handler := final
	names := []string{"client-ip", "peer-trust-guard"}
	for _, name := range names {
		idx := indexOf(steps, name)
		if idx < 0 {
			t.Fatalf("step %q not found in chain", name)
		}
	}
	// Apply in reverse chain order (client-ip wraps final, then
	// peer-trust-guard wraps client-ip) so ServeHTTP runs
	// peer-trust-guard first, exactly as router.New's r.Use loop does.
	handler = steps[indexOf(steps, "client-ip")].MW(handler)
	handler = steps[indexOf(steps, "peer-trust-guard")].MW(handler)
	return handler
}

func names(steps []router.NamedMiddleware) []string {
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = s.Name
	}
	return out
}

func indexOf(steps []router.NamedMiddleware, name string) int {
	for i, s := range steps {
		if s.Name == name {
			return i
		}
	}
	return -1
}

// request-protection: Trusted-Proxy Client IP Resolution — "Spoofed
// leftmost entry ignored" scenario. RemoteAddr here IS the trusted
// proxy, so peer-trust-guard is a no-op and client-ip's own XFF walk
// is what's under test.
func TestClientIPStep_SpoofedLeftmostEntryIgnored(t *testing.T) {
	steps, err := router.Steps(router.Config{
		AppEnv:      config.AppEnvProduction,
		ProxyIP:     "172.18.0.2",
		CorsOrigins: []string{"https://app.example.com"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resolved string
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resolved = chimiddleware.GetClientIP(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := buildClientIPChain(t, steps, final)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 172.18.0.2")
	req.RemoteAddr = "172.18.0.2:12345"
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if resolved != "203.0.113.9" {
		t.Fatalf("expected resolved client IP to be the leftmost untrusted entry 203.0.113.9, got %q", resolved)
	}
}

// request-protection: Trusted-Proxy Client IP Resolution — "Untrusted
// intermediary rejected as source" scenario (design.md:646's missing
// row). Unlike the test above, RemoteAddr here is NOT the trusted
// proxy: an attacker reaching the API directly claims an arbitrary
// X-Forwarded-For. Before the peer-trust-guard fix, chi's
// ClientIPFromXFF never consults RemoteAddr at all (chi v5.3.2
// middleware/client_ip.go:92-117), so it accepted the forged header
// verbatim. This must resolve to the PEER's real address instead.
func TestPeerTrustGuard_UntrustedPeerRejectsForgedXFF(t *testing.T) {
	steps, err := router.Steps(router.Config{
		AppEnv:      config.AppEnvProduction,
		ProxyIP:     "172.18.0.2",
		CorsOrigins: []string{"https://app.example.com"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resolved string
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resolved = chimiddleware.GetClientIP(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := buildClientIPChain(t, steps, final)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.RemoteAddr = "198.51.100.7:54321" // NOT the trusted proxy
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if resolved != "198.51.100.7" {
		t.Fatalf("expected resolved client IP to be the untrusted peer's own address 198.51.100.7 (forged X-Forwarded-For must be ignored), got %q", resolved)
	}
}

// D-H: a fixed hop-count fallback is a documented development
// convenience only and MUST be refused outside APP_ENV=development.
func TestSteps_DevHopCountFallbackRefusedInProduction(t *testing.T) {
	_, err := router.Steps(router.Config{
		AppEnv:      config.AppEnvProduction,
		ProxyIP:     "",
		CorsOrigins: []string{"https://app.example.com"},
	})
	if err == nil {
		t.Fatalf("expected an error when APP_ENV=production has no PROXY_IP")
	}
}

func TestSteps_DevHopCountFallbackAllowedInDevelopment(t *testing.T) {
	_, err := router.Steps(router.Config{
		AppEnv:      config.AppEnvDevelopment,
		ProxyIP:     "",
		CorsOrigins: []string{"https://app.example.com"},
	})
	if err != nil {
		t.Fatalf("expected the dev hop-count fallback to be allowed in development, got: %v", err)
	}
}
