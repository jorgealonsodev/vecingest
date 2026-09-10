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
// a fresh database. Both DSNs used to be set independently
// (BOOTSTRAP_DATABASE_URL and MIGRATIONS_DATABASE_URL), and this test
// used to point both at the SAME superuser DSN from Testcontainers -- the
// exact false green documented in docs/pendientes-despliegue.md §9: it
// never actually exercised a distinct vecingest_owner connection, so it
// stayed green while production died with `password authentication
// failed for user "vecingest_owner"` (vecingest_owner is NOLOGIN and has
// no password to authenticate with in the first place). Only
// BOOTSTRAP_DATABASE_URL is set now; runMigrate derives the schema-set
// connection itself (migrate.SchemaDSN), and
// TestRunMigrate_SchemaSetOwnershipIsRestricted below is the regression
// guard: it asserts the actual ownership property this design depends on,
// not just "migrate exits 0".
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

// TestRunMigrate_SchemaSetOwnershipIsRestricted is the regression guard
// docs/pendientes-despliegue.md §9 asks for: "migrate exits 0" cannot
// distinguish a correctly-restricted schema set from one that silently
// ran (and now owns everything) as the superuser -- exactly the failure
// mode that broke a real deploy with `password authentication failed for
// user "vecingest_owner"` while every existing test stayed green. This
// test queries the catalog after a real migrate run and asserts the two
// properties that actually matter:
//
//  1. vecingest_owner stays NOLOGIN (never becomes directly connectable).
//  2. Every table the schema set created is owned by vecingest_owner, not
//     the superuser, with goose_db_version_bootstrap asserted as the one
//     documented exception (it is created by the BOOTSTRAP set while
//     still connected as the superuser) rather than silently excluded.
//
// Without assertion 2, a future regression to "everything runs as
// superuser" would pass assertAppRwNotOwnerMember
// (migrations/bootstrap/00001_roles.go) too: that check is about role
// MEMBERSHIP, not table OWNERSHIP, and stays green either way.
func TestRunMigrate_SchemaSetOwnershipIsRestricted(t *testing.T) {
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
	t.Setenv("APP_DB_USER", "app_rw")
	t.Setenv("APP_DB_PASSWORD", "test-ownership-password")
	t.Setenv("DATABASE_URL", dsn)
	t.Setenv("DOMAIN", "example.com")
	t.Setenv("JWT_SECRET", "test-ownership-jwt-secret")
	t.Setenv("JWT_REFRESH_SECRET", "test-ownership-jwt-refresh-secret")
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

	var canLogin bool
	if err := verifyDB.QueryRowContext(ctx, `SELECT rolcanlogin FROM pg_roles WHERE rolname = 'vecingest_owner'`).Scan(&canLogin); err != nil {
		t.Fatalf("query vecingest_owner rolcanlogin: %v", err)
	}
	if canLogin {
		t.Fatalf("expected vecingest_owner to stay rolcanlogin = false, got true")
	}

	rows, err := verifyDB.QueryContext(ctx, `
		SELECT c.relname, r.rolname
		FROM pg_class c
		JOIN pg_roles r ON r.oid = c.relowner
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'r'
	`)
	if err != nil {
		t.Fatalf("query table ownership: %v", err)
	}
	defer func() { _ = rows.Close() }()

	const bootstrapException = "goose_db_version_bootstrap"
	seenBootstrapException := false
	tableCount := 0
	for rows.Next() {
		var relname, owner string
		if err := rows.Scan(&relname, &owner); err != nil {
			t.Fatalf("scan table ownership row: %v", err)
		}
		tableCount++

		if relname == bootstrapException {
			seenBootstrapException = true
			if owner != "postgres" {
				t.Errorf("expected the documented exception %s to be owned by the superuser (postgres), got %q", bootstrapException, owner)
			}
			continue
		}

		if owner != "vecingest_owner" {
			t.Errorf("expected table %s to be owned by vecingest_owner, got %q -- the schema set may have run as the superuser instead of assuming the role", relname, owner)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table ownership rows: %v", err)
	}

	if !seenBootstrapException {
		t.Fatalf("expected to find the documented exception table %s", bootstrapException)
	}
	// A vacuous pass (only the exception table found) would defeat the
	// point of this test: assert the schema set actually created
	// something to check ownership on.
	if tableCount <= 1 {
		t.Fatalf("expected more than just %s in public -- got %d table(s), the schema set may not have run", bootstrapException, tableCount)
	}
}
