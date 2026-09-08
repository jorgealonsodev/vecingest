package audit_test

import (
	"context"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/jorgealonsodev/vecingest/internal/domain/audit"
	"github.com/jorgealonsodev/vecingest/internal/testhelpers"
)

type realClock struct{ t time.Time }

func (c realClock) Now() time.Time { return c.t }

// db-access-control: audit_log Hash Chain -- 50 concurrent Append calls
// from 10 goroutines must produce a gap-free, fork-free chain, every
// prev_hash equal to its predecessor's hash (D-O's advisory-lock
// serialization).
func TestAppend_ConcurrentAppendsProduceAGapFreeChain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, _ := testhelpers.AppRWHandles(t)
	ctx := context.Background()

	const goroutines = 10
	const perGoroutine = 5

	g, gctx := errgroup.WithContext(ctx)
	for i := 0; i < goroutines; i++ {
		g.Go(func() error {
			for j := 0; j < perGoroutine; j++ {
				tx, err := handles.Write.Begin(gctx)
				if err != nil {
					return err
				}
				_, err = audit.Append(gctx, tx, realClock{time.Now().UTC()}, audit.Entry{
					Action: "auth.login",
					Entity: "session",
				})
				if err != nil {
					_ = tx.Rollback(gctx)
					return err
				}
				if err := tx.Commit(gctx); err != nil {
					return err
				}
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatalf("concurrent Append calls failed: %v", err)
	}

	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	ok, broken, err := audit.VerifyChain(ctx, handles.Read, from, to)
	if err != nil {
		t.Fatalf("VerifyChain: unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected a gap-free, fork-free chain after 50 concurrent appends, broken at %+v", broken)
	}
}

// A chain spanning a month partition boundary must verify unbroken: a
// month boundary carries no chain semantics (D-O).
func TestAppend_ChainSpanningMonthPartitionBoundaryVerifiesUnbroken(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, _ := testhelpers.AppRWHandles(t)
	ctx := context.Background()

	now := time.Now().UTC()
	nextMonthStart := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	beforeBoundary := nextMonthStart.Add(-time.Minute)
	afterBoundary := nextMonthStart.Add(time.Minute)

	appendAt := func(when time.Time) {
		t.Helper()
		tx, err := handles.Write.Begin(ctx)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if _, err := audit.Append(ctx, tx, realClock{when}, audit.Entry{Action: "auth.login", Entity: "session"}); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("Append at %v: %v", when, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("Commit: %v", err)
		}
	}

	appendAt(beforeBoundary)
	appendAt(afterBoundary)

	ok, broken, err := audit.VerifyChain(ctx, handles.Read, beforeBoundary.Add(-time.Second), afterBoundary.Add(time.Second))
	if err != nil {
		t.Fatalf("VerifyChain: unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected the chain to verify across the month partition boundary, broken at %+v", broken)
	}
}

// A hostile owner-connection UPDATE (bypassing app_rw's revoke,
// simulating PRD §6.1's "operador con acceso a la BD") must make
// VerifyChain fail at exactly that row.
func TestAppend_HostileOwnerUpdateBreaksTheChainAtThatRow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, superuserDB := testhelpers.AppRWHandles(t)
	ctx := context.Background()

	now := time.Now().UTC()
	var rowIDs []string
	for i := 0; i < 3; i++ {
		tx, err := handles.Write.Begin(ctx)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		row, err := audit.Append(ctx, tx, realClock{now.Add(time.Duration(i) * time.Second)}, audit.Entry{
			Action: "auth.login",
			Entity: "session",
		})
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("Append: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		rowIDs = append(rowIDs, row.ID.String())
	}

	// Tamper with the middle row directly as the superuser -- exactly
	// the threat the append-only grants exist to make loud, not
	// possible to prevent from inside the database itself.
	if _, err := superuserDB.ExecContext(ctx,
		`UPDATE audit_log SET action = 'auth.tampered' WHERE id = $1`, rowIDs[1]); err != nil {
		t.Fatalf("hostile UPDATE: %v", err)
	}

	ok, broken, err := audit.VerifyChain(ctx, handles.Read, now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("VerifyChain: unexpected error: %v", err)
	}
	if ok || broken == nil {
		t.Fatalf("expected VerifyChain to detect the hostile UPDATE, got ok=%v", ok)
	}
	if broken.Row.ID.String() != rowIDs[1] {
		t.Fatalf("VerifyChain flagged row %s, want the tampered row %s", broken.Row.ID, rowIDs[1])
	}
}
