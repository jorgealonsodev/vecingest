package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/jorgealonsodev/vecingest/internal/config"
	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/platform/queue"
)

// healthCheckTimeout bounds both health modes' own outbound call
// (HTTP GET, or the leader-heartbeat query): the compose healthcheck
// stanza has its own timeout, but this command must not itself hang
// forever if a dependency never responds.
const healthCheckTimeout = 3 * time.Second

// runHealth implements `vecingest health` (platform-bootstrap: CLI
// Subcommands). --ready and --worker are the two compose healthcheck
// invocations (PRD §8.1); each runs only its own check and never any
// other subcommand's behavior.
func runHealth(ctx context.Context, args []string, lookup config.LookupEnv) error {
	fs := flag.NewFlagSet("health", flag.ContinueOnError)
	ready := fs.Bool("ready", false, "check readiness via GET /v1/health/ready on PORT (the api container healthcheck)")
	worker := fs.Bool("worker", false, "check the River client's own leader heartbeat freshness via DATABASE_URL (the worker container healthcheck)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	switch {
	case *ready && *worker:
		return errors.New("health: --ready and --worker are mutually exclusive")
	case *ready:
		return runHealthReady(ctx, lookup)
	case *worker:
		return runHealthWorker(ctx, lookup)
	default:
		return errors.New("health: exactly one of --ready or --worker is required")
	}
}

// runHealthReady is the api container's own healthcheck: it calls the
// already-running server's own /v1/health/ready over loopback, exactly
// as the compose healthcheck does (PRD §8.1 line 1187).
func runHealthReady(ctx context.Context, lookup config.LookupEnv) error {
	cfg, _, err := config.Load(ctx, lookup, config.CommandHealthReady)
	if err != nil {
		return err
	}

	reqCtx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	url := fmt.Sprintf("http://127.0.0.1:%s/v1/health/ready", cfg.Port)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("health: build request: %w", err)
	}
	client := &http.Client{Timeout: healthCheckTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("health: GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health: /v1/health/ready returned status %d", resp.StatusCode)
	}
	return nil
}

// runHealthWorker is the worker container's own healthcheck: it asserts
// this process's own river_leader lease is still fresh (design D-S; see
// internal/platform/queue.LeaderHeartbeatFresh's doc comment for why
// river_leader, not a nonexistent river_client table, is the correct
// heartbeat source against the pinned v0.47.0 schema).
func runHealthWorker(ctx context.Context, lookup config.LookupEnv) error {
	_, holder, err := config.Load(ctx, lookup, config.CommandHealthWorker)
	if err != nil {
		return err
	}

	workerDB, err := db.NewWorkerHandle(ctx, holder.DatabaseURL())
	if err != nil {
		return fmt.Errorf("health: connect to database: %w", err)
	}
	defer workerDB.Close()

	queryCtx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	fresh, err := queue.LeaderHeartbeatFresh(queryCtx, workerDB)
	if err != nil {
		return fmt.Errorf("health: check leader heartbeat: %w", err)
	}
	if !fresh {
		return errors.New("health: no fresh river_leader heartbeat")
	}
	return nil
}
