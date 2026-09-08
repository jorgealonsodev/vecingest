package schema

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jorgealonsodev/vecingest/migrations/bootstrap"
)

// startPostgres starts a real PostgreSQL 17 container. Every test that
// calls this MUST carry a testing.Short() skip guard.
func startPostgres(t *testing.T) *sql.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	container, err := postgres.Run(ctx,
		"postgres:17-alpine",
		postgres.WithDatabase("vecingest"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate postgres container: %v", err)
		}
	})

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("failed to ping db: %v", err)
	}
	return db
}

// provisionBaseline runs the bootstrap set (roles, fail-closed default
// privileges, append-only guard) and then a minimal hand-rolled
// audit_log with one partition -- enough for I1-I4 to have something to
// check, without pulling in the full schema-set provider (which would
// import internal/platform/migrate and create an import cycle, since
// that package already imports this one).
func provisionBaseline(t *testing.T, db *sql.DB) {
	t.Helper()
	t.Setenv("APP_DB_USER", "app_rw")
	t.Setenv("APP_DB_PASSWORD", "invariants-test-password")

	ctx := context.Background()
	if err := bootstrap.Up(ctx, db); err != nil {
		t.Fatalf("bootstrap.Up failed: %v", err)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("failed to acquire connection: %v", err)
	}
	defer func() { _ = conn.Close() }()

	stmts := []string{
		`SET ROLE vecingest_owner`,
		`CREATE TABLE audit_log (
			id uuid NOT NULL,
			user_id uuid,
			community_id uuid,
			action text NOT NULL,
			entity text NOT NULL,
			entity_id uuid,
			before jsonb,
			after jsonb,
			ip inet,
			request_id uuid,
			prev_hash bytea,
			hash bytea,
			created_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (created_at, id)
		) PARTITION BY RANGE (created_at)`,
		`REVOKE UPDATE, DELETE, TRUNCATE ON audit_log FROM app_rw, PUBLIC`,
		`CREATE TABLE audit_log_2026_01 PARTITION OF audit_log FOR VALUES FROM ('2026-01-01') TO ('2026-02-01')`,
		`RESET ROLE`,
	}
	for _, stmt := range stmts {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("baseline provisioning failed at %q: %v", stmt, err)
		}
	}
}

// db-access-control: SECURITY DEFINER EXECUTE Invariant -- "Correctly
// revoked function passes" and the sibling I1/I3/I4 clean-baseline cases.
func TestAssertInvariantsUp_PassesOnCleanBaseline(t *testing.T) {
	db := startPostgres(t)
	provisionBaseline(t, db)

	if err := assertInvariantsUp(context.Background(), db); err != nil {
		t.Fatalf("expected the clean baseline to pass every invariant, got: %v", err)
	}
}

// db-access-control: SECURITY DEFINER EXECUTE Invariant -- "Migration-time
// assertion catches a violation", extended to all four invariants (D-B).
func TestAssertInvariantsUp_CatchesPlantedViolations(t *testing.T) {
	tests := []struct {
		name       string
		plant      func(t *testing.T, db *sql.DB)
		wantSubstr string
	}{
		{
			name: "I1_write_privilege_granted_on_partition",
			plant: func(t *testing.T, db *sql.DB) {
				t.Helper()
				if _, err := db.Exec(`GRANT UPDATE ON audit_log_2026_01 TO app_rw`); err != nil {
					t.Fatalf("failed to plant I1 violation: %v", err)
				}
			},
			wantSubstr: "invariant I1",
		},
		{
			name: "I2_security_definer_function_execute_to_public",
			plant: func(t *testing.T, db *sql.DB) {
				t.Helper()
				ctx := context.Background()
				conn, err := db.Conn(ctx)
				if err != nil {
					t.Fatalf("failed to acquire connection: %v", err)
				}
				defer func() { _ = conn.Close() }()
				stmts := []string{
					`SET ROLE vecingest_owner`,
					`CREATE FUNCTION plant_i2_violation() RETURNS void
						LANGUAGE plpgsql SECURITY DEFINER AS $$
						BEGIN
							INSERT INTO audit_log (id, action, entity, created_at)
							VALUES ('00000000-0000-0000-0000-000000000001'::uuid, 'planted', 'test', now());
						END;
						$$`,
					// New functions get EXECUTE granted to PUBLIC by
					// default (research §7) -- leaving it unrevoked is
					// exactly the violation I2 exists to catch.
					`RESET ROLE`,
				}
				for _, stmt := range stmts {
					if _, err := conn.ExecContext(ctx, stmt); err != nil {
						t.Fatalf("failed to plant I2 violation at %q: %v", stmt, err)
					}
				}
			},
			wantSubstr: "invariant I2",
		},
		{
			name: "I3_app_rw_member_of_owner",
			plant: func(t *testing.T, db *sql.DB) {
				t.Helper()
				// WITH INHERIT FALSE isolates the membership violation
				// I3 checks (pg_auth_members) from I1's write-privilege
				// scan: without it, app_rw would also *inherit*
				// vecingest_owner's privileges on audit_log, which I1
				// would (correctly) catch first, masking which
				// invariant this case is meant to exercise.
				if _, err := db.Exec(`GRANT vecingest_owner TO app_rw WITH INHERIT FALSE`); err != nil {
					t.Fatalf("failed to plant I3 violation: %v", err)
				}
			},
			wantSubstr: "invariant I3",
		},
		{
			name: "I4_default_privilege_baseline_widened",
			plant: func(t *testing.T, db *sql.DB) {
				t.Helper()
				if _, err := db.Exec(`ALTER DEFAULT PRIVILEGES FOR ROLE vecingest_owner GRANT UPDATE ON TABLES TO app_rw`); err != nil {
					t.Fatalf("failed to plant I4 violation: %v", err)
				}
			},
			wantSubstr: "invariant I4",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := startPostgres(t)
			provisionBaseline(t, db)
			tc.plant(t, db)

			err := assertInvariantsUp(context.Background(), db)
			if err == nil {
				t.Fatalf("expected a %s violation to fail the migration-time assertion, got nil", tc.wantSubstr)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("expected error to mention %q, got: %v", tc.wantSubstr, err)
			}
		})
	}
}
