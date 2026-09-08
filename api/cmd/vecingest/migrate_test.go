package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// platform-bootstrap: Embedded Migration Runner With Advisory Lock --
// `vecingest migrate` applies both the bootstrap and schema sets against
// a fresh database, matching D-I's requirement that MIGRATIONS_DATABASE_URL
// authenticate as a role that can SET ROLE vecingest_owner (a superuser
// satisfies this; see migrations/schema/schema.go's own doc comment).
func TestRunMigrate_AppliesBootstrapAndSchemaSets(t *testing.T) {
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
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	t.Setenv("BOOTSTRAP_DATABASE_URL", dsn)
	t.Setenv("MIGRATIONS_DATABASE_URL", dsn)
	t.Setenv("APP_DB_USER", "app_rw")
	t.Setenv("APP_DB_PASSWORD", "test-migrate-subcommand-password")
	t.Setenv("DATABASE_URL", dsn)
	t.Setenv("DOMAIN", "example.com")
	t.Setenv("JWT_SECRET", "test-migrate-jwt-secret")
	t.Setenv("JWT_REFRESH_SECRET", "test-migrate-jwt-refresh-secret")
	t.Setenv("ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("SMTP_URL", "smtps://user:pw@smtp.example.com:465")
	t.Setenv("MAIL_FROM", "Vecingest <no-reply@example.com>")
	t.Setenv("APP_ENV", "production")
	t.Setenv("PROXY_IP", "172.18.0.2")
	t.Setenv("PORT", "3000")
	t.Setenv("CORS_ORIGINS", "https://app.example.com")

	stdout := &strings.Builder{}
	if err := runMigrate(ctx, nil, stdout, os.LookupEnv); err != nil {
		t.Fatalf("runMigrate: %v", err)
	}

	verifyDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open verify connection: %v", err)
	}
	defer func() { _ = verifyDB.Close() }()

	var exists bool
	if err := verifyDB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users')`).Scan(&exists); err != nil {
		t.Fatalf("query users table: %v", err)
	}
	if !exists {
		t.Fatalf("expected the users table to exist after migrate")
	}

	if err := verifyDB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_rw')`).Scan(&exists); err != nil {
		t.Fatalf("query app_rw role: %v", err)
	}
	if !exists {
		t.Fatalf("expected app_rw role to exist after migrate")
	}
}
