package migrate_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/platform/migrate"
	"github.com/jorgealonsodev/vecingest/migrations/bootstrap"
)

func runBootstrapWithCreds(t *testing.T, db *sql.DB, appDBUser, appDBPassword string) {
	t.Helper()
	t.Setenv("APP_DB_USER", appDBUser)
	t.Setenv("APP_DB_PASSWORD", appDBPassword)
	if err := migrate.RunBootstrap(context.Background(), db); err != nil {
		t.Fatalf("RunBootstrap failed: %v", err)
	}
}

// db-access-control: Three-Role Provisioning.
func TestRunBootstrap_ProvisionsThreeDistinctRoles(t *testing.T) {
	db, _ := startPostgres(t)
	runBootstrapWithCreds(t, db, "app_rw", "test-password-123")

	var exists bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'vecingest_owner')`).Scan(&exists); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if !exists {
		t.Fatalf("expected vecingest_owner to exist after bootstrap")
	}

	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_rw')`).Scan(&exists); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if !exists {
		t.Fatalf("expected app_rw to exist after bootstrap")
	}

	// The bootstrap connection itself authenticates as the image
	// superuser -- confirm all three identities are distinct.
	var superuserName string
	if err := db.QueryRow(`SELECT current_user`).Scan(&superuserName); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if superuserName == "vecingest_owner" || superuserName == "app_rw" {
		t.Fatalf("expected the bootstrap connection identity %q to differ from the two provisioned roles", superuserName)
	}

	// I3: app_rw MUST NOT be a member of vecingest_owner.
	var isMember bool
	const q = `
		SELECT EXISTS (
			SELECT 1 FROM pg_auth_members m
			JOIN pg_roles owner ON owner.oid = m.roleid AND owner.rolname = 'vecingest_owner'
			JOIN pg_roles member ON member.oid = m.member AND member.rolname = 'app_rw'
		)`
	if err := db.QueryRow(q).Scan(&isMember); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if isMember {
		t.Fatalf("expected app_rw to NOT be a member of vecingest_owner")
	}
}

// db-access-control: SECURITY DEFINER EXECUTE Invariant setup / D-B
// mechanism 2 -- the registry table and event trigger the guard depends
// on must exist after the bootstrap migration.
func TestRunBootstrap_AppendOnlyGuardInstalled(t *testing.T) {
	db, _ := startPostgres(t)
	runBootstrapWithCreds(t, db, "app_rw", "test-password-456")

	var seededRelname string
	if err := db.QueryRow(`SELECT relname FROM append_only_relations WHERE relname = 'audit_log'`).Scan(&seededRelname); err != nil {
		t.Fatalf("expected append_only_relations to be seeded with 'audit_log': %v", err)
	}
	if seededRelname != "audit_log" {
		t.Fatalf("expected seeded relname 'audit_log', got %q", seededRelname)
	}

	var triggerExists bool
	const q = `SELECT EXISTS (SELECT 1 FROM pg_event_trigger WHERE evtname = 'vecingest_append_only_guard')`
	if err := db.QueryRow(q).Scan(&triggerExists); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if !triggerExists {
		t.Fatalf("expected event trigger vecingest_append_only_guard to exist")
	}

	// Triangulation: the event trigger fires ON ddl_command_end for the
	// exact tag set the design pins -- CREATE TABLE/CREATE TABLE
	// AS/ALTER TABLE, never a literal (nonexistent) ATTACH tag.
	var events string
	const evq = `SELECT evtevent FROM pg_event_trigger WHERE evtname = 'vecingest_append_only_guard'`
	if err := db.QueryRow(evq).Scan(&events); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if events != "ddl_command_end" {
		t.Fatalf("expected the guard to fire on ddl_command_end, got %q", events)
	}

	var funcExists bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_proc WHERE proname = 'vecingest_append_only_guard_fn')`).Scan(&funcExists); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if !funcExists {
		t.Fatalf("expected vecingest_append_only_guard_fn to exist")
	}
}

// platform-bootstrap: Embedded Migration Runner With Advisory Lock.
// Proves the generic locking mechanism NewLockedProvider wires in: two
// concurrent callers using the SAME pinned lock id and the SAME migration
// set converge on one execution and no race, exactly like two processes
// (api, worker) booting simultaneously against an unmigrated database.
func TestNewLockedProvider_ConcurrentCallersConverge(t *testing.T) {
	db, _ := startPostgres(t)

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS concurrency_marker (id int PRIMARY KEY, hits int NOT NULL DEFAULT 0)`); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	markUp := func(_ context.Context, db *sql.DB) error {
		_, err := db.Exec(`INSERT INTO concurrency_marker (id, hits) VALUES (1, 1)
			ON CONFLICT (id) DO UPDATE SET hits = concurrency_marker.hits + 1`)
		return err
	}
	markDown := func(_ context.Context, _ *sql.DB) error { return nil }

	run := func() error {
		provider, err := migrate.NewLockedProvider(db, nil, bootstrap.LockID, "goose_db_version",
			goMigration(1, markUp, markDown))
		if err != nil {
			return err
		}
		_, err = provider.Up(context.Background())
		return err
	}

	errCh := make(chan error, 2)
	go func() { errCh <- run() }()
	go func() { errCh <- run() }()

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("concurrent locked provider run failed: %v", err)
		}
	}

	var hits int
	if err := db.QueryRow(`SELECT hits FROM concurrency_marker WHERE id = 1`).Scan(&hits); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected the migration to run exactly once despite two concurrent callers, got %d executions", hits)
	}
}
