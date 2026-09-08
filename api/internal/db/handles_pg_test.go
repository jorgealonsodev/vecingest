package db_test

import (
	"context"
	"database/sql"
	"net/url"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/platform/migrate"
)

// startBootstrappedPostgres starts a real PostgreSQL 17 container, runs
// the bootstrap migration to provision app_rw with a known password, and
// returns a DSN string authenticating as app_rw against that database.
// Every test that calls this MUST carry a testing.Short() skip guard.
func startBootstrappedPostgres(t *testing.T, appRwPassword string) (appRwDSN string) {
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

	superuserDSN, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	superuserDB, err := sql.Open("pgx", superuserDSN)
	if err != nil {
		t.Fatalf("failed to open superuser db: %v", err)
	}
	defer func() { _ = superuserDB.Close() }()

	t.Setenv("APP_DB_USER", "app_rw")
	t.Setenv("APP_DB_PASSWORD", appRwPassword)
	if err := migrate.RunBootstrap(ctx, superuserDB); err != nil {
		t.Fatalf("RunBootstrap failed: %v", err)
	}

	u, err := url.Parse(superuserDSN)
	if err != nil {
		t.Fatalf("failed to parse superuser dsn: %v", err)
	}
	u.User = url.UserPassword("app_rw", appRwPassword)
	return u.String()
}

// platform-bootstrap: Per-Role Database Write and Read Handles -- "A
// configured read override cannot escalate the runtime role" and "A
// configured worker override cannot escalate the runtime role".
func TestNewServeHandles_ReadOverrideAuthenticatesAsAppRw(t *testing.T) {
	appRwDSN := startBootstrappedPostgres(t, "read-override-password")

	handles, err := db.NewServeHandles(context.Background(), appRwDSN, appRwDSN)
	if err != nil {
		t.Fatalf("NewServeHandles failed: %v", err)
	}
	defer handles.Close()

	var currentUser string
	if err := handles.Read.QueryRow(context.Background(), "SELECT current_user").Scan(&currentUser); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if currentUser != "app_rw" {
		t.Fatalf("expected the read handle to authenticate as app_rw, got %q", currentUser)
	}
}

func TestNewWorkerHandle_WorkerOverrideAuthenticatesAsAppRw(t *testing.T) {
	appRwDSN := startBootstrappedPostgres(t, "worker-override-password")

	handle, err := db.NewWorkerHandle(context.Background(), appRwDSN)
	if err != nil {
		t.Fatalf("NewWorkerHandle failed: %v", err)
	}
	defer handle.Close()

	var currentUser string
	if err := handle.QueryRow(context.Background(), "SELECT current_user").Scan(&currentUser); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if currentUser != "app_rw" {
		t.Fatalf("expected the worker handle to authenticate as app_rw, got %q", currentUser)
	}
}
