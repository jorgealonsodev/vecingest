package migrate_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/pressly/goose/v3"
)

// startPostgres starts a real PostgreSQL 17 container (the exact image
// D-K/D-K's compose pin uses) and returns an open superuser *sql.DB plus
// the superuser DSN. Every test that calls this MUST carry a
// testing.Short() skip guard: CI's fast job runs with the Docker daemon
// stopped, and an unguarded container acquisition must fail loudly there
// rather than silently hang.
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

	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatalf("failed to ping db: %v", err)
	}

	return sqlDB, connStr
}

// goMigration is a small test-only convenience wrapper for constructing a
// trivial goose.Migration from RunDB up/down functions.
func goMigration(version int64, up, down func(context.Context, *sql.DB) error) *goose.Migration {
	return goose.NewGoMigration(version, &goose.GoFunc{RunDB: up}, &goose.GoFunc{RunDB: down})
}
