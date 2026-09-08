// Package queue wires River (design D-L) for the `worker` subcommand:
// a client with leader election active and, at M0, zero real job
// producers or consumers -- the schema, the client and the compose
// service are real deliverables, but nothing ever enqueues a job
// (platform-bootstrap: CLI Subcommands; design D-S "vecingest worker --
// a real deliverable with zero jobs").
package queue

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/jorgealonsodev/vecingest/internal/db"
)

// noopArgs/noopWorker exist ONLY to satisfy river v0.47.0's own startup
// contract, not to do any work. This pinned version's public API has no
// "leadership only, no queues" mode: Config.validate rejects a
// non-nil Queues with a nil Workers bundle ("Workers must be set if
// Queues is set"), and Client.Start refuses to start at all unless
// willExecuteJobs() is true, i.e. len(Queues) > 0 ("client Queues and
// Workers must be configured for a client to start working"). River's
// own leader-election service is internal (internal/leadership),
// unreachable from outside the module.
//
// This is a deliberate, documented deviation from design.md/tasks.md's
// literal "zero registered workers" wording: it is not achievable
// against the actually-pinned v0.47.0 API (the same kind of drift
// already documented in migrations/schema/00004_river.go's
// grantRiverTables comment, where the design's river_client/
// river_client_queue table names also do not exist in this version).
// noopWorker.Work is never invoked in practice because nothing in this
// codebase ever calls Client.Insert/InsertMany for any job kind at M0 --
// the meaningful invariant is "zero producers", tested directly by
// test/privilege_matrix_test.go asserting river_job stays empty after a
// full migration run.
type noopArgs struct{}

func (noopArgs) Kind() string { return "vecingest_noop_placeholder" }

type noopWorker struct {
	river.WorkerDefaults[noopArgs]
}

func (noopWorker) Work(context.Context, *river.Job[noopArgs]) error { return nil }

// NewClient builds a River client over pool (riverpgxv5, the pgx-native
// driver -- distinct from migrations/schema's own riverdatabasesql,
// which only goose's database/sql-shaped migration runner needs). A nil
// logger lets river.Config.WithDefaults install its own default logger.
func NewClient(pool *pgxpool.Pool, logger *slog.Logger) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &noopWorker{})

	driver := riverpgxv5.New(pool)
	cfg := &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 1},
		},
		Workers: workers,
	}
	if logger != nil {
		cfg.Logger = logger
	}
	return river.NewClient(driver, cfg)
}

// LeaderHeartbeatFresh reports whether the single 'default' river_leader
// row has an unexpired lease. This is the closest available proxy, in
// the actually-pinned v0.47.0 schema, for "this worker's own heartbeat
// is fresh" (platform-bootstrap: CLI Subcommands, health --worker
// scenarios): v0.47.0 has no river_client table at all (confirmed
// against its embedded migration SQL; the same mismatch is already
// documented in 00004_river.go), and at M0 there is exactly one worker
// replica, so an unexpired leader lease IS this worker's own heartbeat.
// No row yet (leader election has not completed its first cycle) is
// reported as not-fresh, not an error.
func LeaderHeartbeatFresh(ctx context.Context, dbtx db.DBTX) (bool, error) {
	const q = `SELECT expires_at > now() FROM river_leader WHERE name = 'default'`
	var fresh bool
	err := dbtx.QueryRow(ctx, q).Scan(&fresh)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return fresh, nil
}
