package queue_test

import (
	"context"
	"testing"
	"time"

	"github.com/jorgealonsodev/vecingest/internal/platform/queue"
	"github.com/jorgealonsodev/vecingest/internal/testhelpers"
)

// platform-bootstrap: CLI Subcommands / D-S -- `worker` starts a River
// client with zero registered [job-producing] workers plus leader
// election. River v0.47.0's public Client.Start requires at least one
// configured queue and worker KIND to start at all (see queue.go's own
// doc comment on NewClient for the full explanation of this deviation
// from the literal "zero registered workers" wording); what actually
// matters and is asserted here is that leader election runs for real
// (a river_leader row appears while the client is up, and clears once it
// stops) and that nothing enqueues a job.
func TestNewClient_AcquiresAndReleasesLeadership(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, superuserDB := testhelpers.AppRWHandles(t)

	client, err := queue.NewClient(handles.Write.Pool(), nil)
	if err != nil {
		t.Fatalf("queue.NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Start(ctx); err != nil {
		t.Fatalf("client.Start: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	var leaderCount int
	for time.Now().Before(deadline) {
		if err := superuserDB.QueryRowContext(ctx, `SELECT count(*) FROM river_leader`).Scan(&leaderCount); err != nil {
			t.Fatalf("query river_leader: %v", err)
		}
		if leaderCount == 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if leaderCount != 1 {
		t.Fatalf("expected exactly one river_leader row after Start, got %d", leaderCount)
	}

	var jobCount int
	if err := superuserDB.QueryRowContext(ctx, `SELECT count(*) FROM river_job`).Scan(&jobCount); err != nil {
		t.Fatalf("query river_job: %v", err)
	}
	if jobCount != 0 {
		t.Fatalf("expected zero producers at M0: river_job must stay empty, got %d rows", jobCount)
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	if err := client.Stop(stopCtx); err != nil {
		t.Fatalf("client.Stop: %v", err)
	}

	if err := superuserDB.QueryRowContext(context.Background(), `SELECT count(*) FROM river_leader`).Scan(&leaderCount); err != nil {
		t.Fatalf("query river_leader after stop: %v", err)
	}
	if leaderCount != 0 {
		t.Fatalf("expected river_leader to be released after Stop, got %d rows", leaderCount)
	}
}

// platform-bootstrap: CLI Subcommands -- `health --worker` exits 0/1
// based on this client's own heartbeat freshness (D-S).
func TestLeaderHeartbeatFresh(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, _ := testhelpers.AppRWHandles(t)
	ctx := context.Background()

	fresh, err := queue.LeaderHeartbeatFresh(ctx, handles.Write)
	if err != nil {
		t.Fatalf("LeaderHeartbeatFresh (no leader yet): %v", err)
	}
	if fresh {
		t.Fatalf("expected no fresh heartbeat before any leader is elected")
	}

	if _, err := handles.Write.Exec(ctx, `INSERT INTO river_leader (leader_id, elected_at, expires_at) VALUES ('test-leader', now(), now() + interval '1 minute')`); err != nil {
		t.Fatalf("insert fresh river_leader row: %v", err)
	}
	fresh, err = queue.LeaderHeartbeatFresh(ctx, handles.Write)
	if err != nil {
		t.Fatalf("LeaderHeartbeatFresh (fresh row): %v", err)
	}
	if !fresh {
		t.Fatalf("expected a fresh (unexpired) river_leader row to report fresh=true")
	}

	if _, err := handles.Write.Exec(ctx, `UPDATE river_leader SET expires_at = now() - interval '1 minute'`); err != nil {
		t.Fatalf("expire river_leader row: %v", err)
	}
	fresh, err = queue.LeaderHeartbeatFresh(ctx, handles.Write)
	if err != nil {
		t.Fatalf("LeaderHeartbeatFresh (expired row): %v", err)
	}
	if fresh {
		t.Fatalf("expected an expired river_leader row to report fresh=false")
	}
}
