package main

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jorgealonsodev/vecingest/internal/testhelpers"
)

// syncBuffer is a mutex-guarded strings.Builder: runServe writes to
// stdout from its own goroutine below while the test polls it from the
// main goroutine, and a bare strings.Builder is not safe for that
// (caught by `go test -race`, not a hypothetical concern).
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// platform-bootstrap: CLI Subcommands -- `serve` binds a real HTTP
// listener, serves the assembled internal/http/api router, and shuts
// down gracefully when its context is cancelled (simulating SIGTERM;
// main.go wires the real signal handler around this same function).
// Added by the orchestrator: no task anywhere else in the plan wires
// internal/http/api into a real listener, so this is the only place
// that behavior is exercised end to end.
func TestRunServe_ListensServesAndShutsDownGracefully(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, _ := testhelpers.AppRWHandles(t)
	dsn := handles.Write.Config().ConnConfig.ConnString()

	env := map[string]string{ //nolint:gosec // G101: fake fixture secrets for a Testcontainers-backed test environment, never real credentials
		"DOMAIN":             "example.com",
		"DATABASE_URL":       dsn,
		"JWT_SECRET":         "test-serve-jwt-secret-32-bytes-ok!!",
		"JWT_REFRESH_SECRET": "test-serve-jwt-refresh-secret-32ok!",
		"ENCRYPTION_KEY":     base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"SMTP_URL":           "smtps://user:pw@smtp.example.com:465",
		"MAIL_FROM":          "Vecingest <no-reply@example.com>",
		"APP_ENV":            "development",
		"CORS_ORIGINS":       "https://app.example.com",
		"PORT":               "0",
	}
	lookup := lookupMap(env)

	ctx, cancel := context.WithCancel(context.Background())
	stdout := &syncBuffer{}
	errCh := make(chan error, 1)
	go func() { errCh <- runServe(ctx, nil, stdout, lookup) }()

	var addr string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(stdout.String(), "serve: listening on ") {
			line := stdout.String()
			start := strings.Index(line, "serve: listening on ") + len("serve: listening on ")
			end := strings.IndexByte(line[start:], ' ')
			addr = line[start : start+end]
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if addr == "" {
		t.Fatalf("serve did not report a listening address within timeout; stdout=%q", stdout.String())
	}

	resp, err := http.Get("http://" + addr + "/v1/health/live") //nolint:noctx // bounded by the surrounding polling deadline and short test timeout
	if err != nil {
		t.Fatalf("GET /v1/health/live: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /v1/health/live, got %d", resp.StatusCode)
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("expected runServe to shut down cleanly, got: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("serve did not shut down gracefully within a reasonable bound")
	}

	if !strings.Contains(stdout.String(), "serve: stopped") {
		t.Fatalf("expected a graceful-shutdown log line, got stdout=%q", stdout.String())
	}
}
