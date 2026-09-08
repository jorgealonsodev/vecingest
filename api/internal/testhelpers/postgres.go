// Package testhelpers provides a shared, fully-migrated PostgreSQL 17
// Testcontainers harness for domain-package integration tests (session,
// audit, mfa) that need a real app_rw connection rather than a mock --
// per this project's testing discipline, the database is not one of the
// mockable system boundaries (Clock, Mailer, AttemptCounter,
// HIBPChecker); it is exercised for real, exactly as api/test's
// privilege-matrix suite already does for the schema and bootstrap
// migrations themselves.
package testhelpers

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

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/platform/migrate"
)

const (
	appDBUser     = "app_rw"
	appDBPassword = "test-domain-integration-password"
)

// AppRWHandles starts a real PostgreSQL 17 container, runs the bootstrap
// and schema migration sets against it, and returns db.Handles connected
// as app_rw -- the role every domain service actually runs under in
// production -- plus the raw superuser *sql.DB for tests that need to
// simulate a hostile operator with direct database access (PRD §6.1's
// threat model), bypassing every app_rw grant. Every test that calls
// this MUST already carry its own testing.Short() skip guard.
func AppRWHandles(t *testing.T) (db.Handles, *sql.DB) {
	t.Helper()

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
		t.Fatalf("testhelpers: failed to start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("testhelpers: failed to terminate postgres container: %v", err)
		}
	})

	superuserDSN, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("testhelpers: failed to get connection string: %v", err)
	}

	superuserDB, err := sql.Open("pgx", superuserDSN)
	if err != nil {
		t.Fatalf("testhelpers: failed to open superuser db: %v", err)
	}
	t.Cleanup(func() { _ = superuserDB.Close() })
	if err := superuserDB.PingContext(ctx); err != nil {
		t.Fatalf("testhelpers: failed to ping superuser db: %v", err)
	}

	t.Setenv("APP_DB_USER", appDBUser)
	t.Setenv("APP_DB_PASSWORD", appDBPassword)
	if err := migrate.RunBootstrap(ctx, superuserDB); err != nil {
		t.Fatalf("testhelpers: RunBootstrap failed: %v", err)
	}
	if err := migrate.RunSchema(ctx, superuserDB); err != nil {
		t.Fatalf("testhelpers: RunSchema failed: %v", err)
	}

	appRWDSN := replaceUserPassword(superuserDSN, appDBUser, appDBPassword)
	handles, err := db.NewServeHandles(ctx, appRWDSN, appRWDSN)
	if err != nil {
		t.Fatalf("testhelpers: failed to build app_rw handles: %v", err)
	}
	t.Cleanup(handles.Close)

	return handles, superuserDB
}

// replaceUserPassword swaps the userinfo segment of a postgres DSN of
// the form postgres://user:pass@host:port/db?sslmode=disable.
func replaceUserPassword(dsn, user, password string) string {
	const scheme = "postgres://"
	if len(dsn) < len(scheme) || dsn[:len(scheme)] != scheme {
		panic(fmt.Sprintf("testhelpers: unexpected DSN shape: %s", dsn))
	}
	rest := dsn[len(scheme):]
	for i := 0; i < len(rest); i++ {
		if rest[i] == '@' {
			return scheme + user + ":" + password + "@" + rest[i+1:]
		}
	}
	panic(fmt.Sprintf("testhelpers: unexpected DSN shape (no '@'): %s", dsn))
}
