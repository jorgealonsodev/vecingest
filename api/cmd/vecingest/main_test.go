package main

import (
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/config"
	"github.com/jorgealonsodev/vecingest/internal/testhelpers"
)

func lookupMap(m map[string]string) config.LookupEnv {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

func emptyLookup(string) (string, bool) { return "", false }

// platform-bootstrap: CLI Subcommands -- "Subcommand dispatch": each
// named subcommand runs only its own flow, distinguishable by its own
// error message, and no other subcommand's guard or behavior ever fires
// for it.
func TestRun_DispatchesToExactlyOneSubcommand(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		lookup          config.LookupEnv
		wantExit        int
		wantErrContains string
	}{
		{
			name:            "bootstrap-superadmin without --password-stdin fails with its own error, not another subcommand's",
			args:            []string{"bootstrap-superadmin", "--email=x@example.com"},
			lookup:          emptyLookup,
			wantExit:        1,
			wantErrContains: "bootstrap-superadmin",
		},
		{
			name:            "seed refuses outside non-production with its own error, not another subcommand's",
			args:            []string{"seed"},
			lookup:          lookupMap(map[string]string{"APP_ENV": "production", "DATABASE_URL": "postgres://poisoned:host-never-dialed@127.0.0.1:1/vecingest"}), //nolint:gosec // G101: fake unreachable DSN, the guard must refuse before ever dialing it
			wantExit:        1,
			wantErrContains: "seed:",
		},
		{
			name:            "health with neither --ready nor --worker fails with its own error",
			args:            []string{"health"},
			lookup:          emptyLookup,
			wantExit:        1,
			wantErrContains: "health:",
		},
		{
			name:     "unknown subcommand is rejected before dispatch",
			args:     []string{"not-a-real-subcommand"},
			lookup:   emptyLookup,
			wantExit: 2,
		},
		{
			name:     "no subcommand at all is rejected",
			args:     []string{},
			lookup:   emptyLookup,
			wantExit: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := &strings.Builder{}
			stderr := &strings.Builder{}
			exit := run(tt.args, strings.NewReader(""), stdout, stderr, tt.lookup)
			if exit != tt.wantExit {
				t.Fatalf("expected exit code %d, got %d (stderr=%q)", tt.wantExit, exit, stderr.String())
			}
			if tt.wantErrContains != "" && !strings.Contains(stderr.String(), tt.wantErrContains) {
				t.Fatalf("expected stderr to contain %q, got %q", tt.wantErrContains, stderr.String())
			}
		})
	}
}

// platform-bootstrap: CLI Subcommands -- "health --ready invoked as the
// api container healthcheck": exits 0/1 based on /v1/health/ready and
// performs no other subcommand's behavior.
func TestRun_HealthReady_ExitsBasedOnReadinessEndpoint(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ready := true
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/health/ready" {
			t.Fatalf("expected only /v1/health/ready to be called, got %s", r.URL.Path)
		}
		if ready {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	srv.Listener.Close() //nolint:errcheck // replaced by our own pre-bound listener below
	srv.Listener = ln
	srv.Start()
	defer srv.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	lookup := lookupMap(map[string]string{"PORT": strconv.Itoa(port)})

	stdout, stderr := &strings.Builder{}, &strings.Builder{}
	if exit := run([]string{"health", "--ready"}, strings.NewReader(""), stdout, stderr, lookup); exit != 0 {
		t.Fatalf("expected exit 0 while ready, got %d (stderr=%q)", exit, stderr.String())
	}

	ready = false
	stdout, stderr = &strings.Builder{}, &strings.Builder{}
	if exit := run([]string{"health", "--ready"}, strings.NewReader(""), stdout, stderr, lookup); exit == 0 {
		t.Fatalf("expected a non-zero exit once readiness fails")
	}
}

// platform-bootstrap: CLI Subcommands -- "bootstrap-superadmin runs only
// its own flow", proven end-to-end through the real dispatcher against a
// real database: no fixture users (seed's own flow) and no
// river_leader/river_job rows (worker's own flow) are ever created by
// dispatching to bootstrap-superadmin.
func TestRun_BootstrapSuperadminThroughDispatchTouchesOnlyItsOwnFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, superuserDB := testhelpers.AppRWHandles(t)
	dsn := handles.Write.Config().ConnConfig.ConnString()

	lookup := lookupMap(map[string]string{
		"DATABASE_URL":   dsn,
		"ENCRYPTION_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32)),
	})

	stdout, stderr := &strings.Builder{}, &strings.Builder{}
	exit := run(
		[]string{"bootstrap-superadmin", "--email=dispatch-admin@example.com", "--password-stdin"},
		strings.NewReader("a-very-strong-dispatch-passphrase\n"),
		stdout, stderr, lookup,
	)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d (stderr=%q)", exit, stderr.String())
	}

	var userCount int
	if err := superuserDB.QueryRowContext(context.Background(), `SELECT count(*) FROM users`).Scan(&userCount); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 1 {
		t.Fatalf("expected exactly the one bootstrapped superadmin user, got %d", userCount)
	}

	var leaderCount, jobCount int
	if err := superuserDB.QueryRowContext(context.Background(), `SELECT count(*) FROM river_leader`).Scan(&leaderCount); err != nil {
		t.Fatalf("count river_leader: %v", err)
	}
	if err := superuserDB.QueryRowContext(context.Background(), `SELECT count(*) FROM river_job`).Scan(&jobCount); err != nil {
		t.Fatalf("count river_job: %v", err)
	}
	if leaderCount != 0 || jobCount != 0 {
		t.Fatalf("expected worker's flow to never run: river_leader=%d river_job=%d", leaderCount, jobCount)
	}
}
