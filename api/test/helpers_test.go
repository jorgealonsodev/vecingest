// Package test hosts cross-cutting integration tests that exercise more
// than one internal package against a real PostgreSQL 17 container --
// the privilege matrix, the guard's partition-attach behaviour, and the
// migration-time invariant assertions, all of which need the bootstrap
// set, the schema set and several distinct role connections in the same
// test to mean anything.
package test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jorgealonsodev/vecingest/internal/platform/migrate"
)

const (
	testAppDBUser     = "app_rw"
	testAppDBPassword = "test-privilege-matrix-password"
)

// startPostgres starts a real PostgreSQL 17 container (the exact image
// the compose pin uses) and returns an open superuser *sql.DB plus the
// superuser DSN. Every test that calls this MUST carry a
// testing.Short() skip guard: CI's fast job runs with the Docker daemon
// stopped.
func startPostgres(t *testing.T) (db *sql.DB, dsn string) {
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

	sqlDB, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := sqlDB.PingContext(context.Background()); err != nil {
		t.Fatalf("failed to ping db: %v", err)
	}

	return sqlDB, connStr
}

// migrateFully runs the bootstrap set (superuser) and then the schema
// set (over the same connection, which SET ROLEs into vecingest_owner
// per migration -- see migrations/schema/schema.go) against db. It sets
// APP_DB_USER/APP_DB_PASSWORD for the duration of the bootstrap run.
func migrateFully(t *testing.T, db *sql.DB) {
	t.Helper()
	t.Setenv("APP_DB_USER", testAppDBUser)
	t.Setenv("APP_DB_PASSWORD", testAppDBPassword)

	ctx := context.Background()
	if err := migrate.RunBootstrap(ctx, db); err != nil {
		t.Fatalf("RunBootstrap failed: %v", err)
	}
	if err := migrate.RunSchema(ctx, db); err != nil {
		t.Fatalf("RunSchema failed: %v", err)
	}
}

// connectAsAppRW opens a fresh connection authenticated as app_rw
// against the same database the superuser dsn points at.
func connectAsAppRW(t *testing.T, superuserDSN string) *sql.DB {
	t.Helper()
	dsn := replaceUserPassword(superuserDSN, testAppDBUser, testAppDBPassword)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("failed to open app_rw connection: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("failed to ping as app_rw: %v", err)
	}
	return db
}

// replaceUserPassword swaps the userinfo segment of a postgres DSN.
// Testcontainers returns a DSN of the form
// postgres://user:pass@host:port/db?sslmode=disable -- constructing it by
// string replacement avoids pulling in a URL library just for this test
// helper.
func replaceUserPassword(dsn, user, password string) string {
	// The superuser DSN always starts with "postgres://" followed by
	// "user:pass@"; find the '@' and rebuild.
	const scheme = "postgres://"
	if len(dsn) < len(scheme) || dsn[:len(scheme)] != scheme {
		panic(fmt.Sprintf("unexpected DSN shape: %s", dsn))
	}
	rest := dsn[len(scheme):]
	for i := 0; i < len(rest); i++ {
		if rest[i] == '@' {
			return scheme + user + ":" + password + "@" + rest[i+1:]
		}
	}
	panic(fmt.Sprintf("unexpected DSN shape (no '@'): %s", dsn))
}

// withOwnerRole runs fn against db.ExecContext calls issued over a single
// dedicated *sql.Conn that has SET ROLE vecingest_owner for its whole
// lifetime, then RESET ROLEs and releases it. vecingest_owner is NOLOGIN
// (migrations/bootstrap), so "as vecingest_owner" for a test means "the
// effective current_user during DDL", reached via SET ROLE from the
// superuser connection -- the same mechanism the schema migrations
// themselves use.
func withOwnerConn(t *testing.T, superuserDB *sql.DB, fn func(conn *sql.Conn)) {
	t.Helper()
	ctx := context.Background()
	conn, err := superuserDB.Conn(ctx)
	if err != nil {
		t.Fatalf("failed to acquire dedicated connection: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "SET ROLE vecingest_owner"); err != nil {
		t.Fatalf("failed to SET ROLE vecingest_owner: %v", err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "RESET ROLE") }()

	fn(conn)
}
