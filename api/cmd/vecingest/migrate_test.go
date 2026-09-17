package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"net/url"
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
	// CommandMigrate is a superset of CommandServe's requirement set
	// (serve --migrate reuses it), and TURNSTILE_SECRET is now required
	// outside development -- like JWT_SECRET/SMTP_URL above, migrate
	// never uses it, it just has to be present.
	t.Setenv("TURNSTILE_SECRET", "test-turnstile-secret")

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
	// CommandMigrate is a superset of CommandServe's requirement set
	// (serve --migrate reuses it), and TURNSTILE_SECRET is now required
	// outside development -- like JWT_SECRET/SMTP_URL above, migrate
	// never uses it, it just has to be present.
	t.Setenv("TURNSTILE_SECRET", "test-turnstile-secret")

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

// db-access-control: a password containing a dollar-quote tag must be
// accepted and stored verbatim, not break the bootstrap migration.
//
// Regression guard for a real defect with a smaller blast radius than it
// first appears. The CREATE/ALTER ROLE statement used to be interpolated
// into a `DO $do$ ... END $do$` block. Inside a dollar-quoted string
// Postgres does no quote processing at all and scans only for the closing
// tag, so quoteLiteral's doubled single quotes protect nothing there: an
// APP_DB_PASSWORD containing the literal text `$do$` closed the block
// early and the remainder parsed at top level.
//
// It is NOT privilege escalation, and this was established by experiment
// rather than by reading the code. Both a `--` payload and one that
// reopens a dollar-quoted string to swallow the tail were run against the
// vulnerable version: both fail with a syntax error before anything
// executes. The reason is structural -- closing the block leaves the DO
// body ending in `... PASSWORD 'x`, an unterminated string literal, and
// the attacker cannot close it because every `'` they supply is doubled
// by quoteLiteral. Postgres rejects the PL/pgSQL body at parse time, and
// the simple query protocol parses the whole batch before executing any
// of it, so the injected statements never run.
//
// What it actually was: any operator password containing `$do$` broke
// deployment with an opaque SQL syntax error. The construct was also one
// edit away from being genuinely exploitable, since the guarantee rested
// on an accident of the surrounding statement's shape rather than on the
// escaping doing its job.
//
// The test asserts the property that survives either reading: such a
// password is accepted, stored verbatim (the role can authenticate with
// it), and grants nothing extra.
func TestRunMigrate_AppDBPasswordCannotInjectSQL(t *testing.T) {
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

	// Closes the dollar-quoted block, escalates, and then REOPENS a
	// dollar-quoted string so the tail of the original statement
	// ("'; ELSE ... END ") is swallowed as that string's body, terminated
	// by the original trailing $do$. Without that last step the leftover
	// ELSE is a top-level syntax error, the whole simple-protocol batch is
	// rejected before execution, and the injection looks like it failed --
	// a naive payload understates the vulnerability rather than proving it.
	const hostilePassword = `x$do$; ALTER ROLE app_rw SUPERUSER; SELECT $do$` //nolint:gosec // G101: an injection payload for this test, not a credential -- the identifier name is what the rule matches on

	t.Setenv("BOOTSTRAP_DATABASE_URL", dsn)
	t.Setenv("APP_DB_USER", "app_rw")
	t.Setenv("APP_DB_PASSWORD", hostilePassword)
	t.Setenv("DATABASE_URL", dsn)
	t.Setenv("DOMAIN", "example.com")
	t.Setenv("JWT_SECRET", "test-injection-jwt-secret")
	t.Setenv("JWT_REFRESH_SECRET", "test-injection-jwt-refresh-secret")
	t.Setenv("ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("SMTP_URL", "smtps://user:pw@smtp.example.com:465")
	t.Setenv("MAIL_FROM", "Vecingest <no-reply@example.com>")
	t.Setenv("APP_ENV", "production")
	t.Setenv("PROXY_IP", "172.18.0.2")
	t.Setenv("PORT", "3000")
	t.Setenv("CORS_ORIGINS", "https://app.example.com")
	// CommandMigrate is a superset of CommandServe's requirement set
	// (serve --migrate reuses it), and TURNSTILE_SECRET is now required
	// outside development -- like JWT_SECRET/SMTP_URL above, migrate
	// never uses it, it just has to be present.
	t.Setenv("TURNSTILE_SECRET", "test-turnstile-secret")

	stdout := &strings.Builder{}
	if err := runMigrate(ctx, nil, stdout, os.LookupEnv); err != nil {
		t.Fatalf("runMigrate with a $-containing password: %v", err)
	}

	verifyDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open verify connection: %v", err)
	}
	defer func() { _ = verifyDB.Close() }()

	var isSuperuser bool
	if err := verifyDB.QueryRowContext(ctx,
		`SELECT rolsuper FROM pg_roles WHERE rolname = $1`, "app_rw",
	).Scan(&isSuperuser); err != nil {
		t.Fatalf("reading app_rw's superuser flag: %v", err)
	}
	if isSuperuser {
		t.Fatal("app_rw is SUPERUSER: the payload in APP_DB_PASSWORD escaped the role statement and executed")
	}

	// The password must also have been stored verbatim. A migration that
	// silently mangled or truncated it would leave the deployment unable
	// to authenticate, so passing the check above is not enough on its own.
	// Built through url.UserPassword rather than string substitution:
	// QueryEscape encodes a space as "+", and "+" is NOT decoded back to a
	// space in a URL's userinfo, so a hand-escaped DSN would fail to
	// authenticate for a reason that has nothing to do with the code under
	// test.
	parsedDSN, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parsing container DSN: %v", err)
	}
	parsedDSN.User = url.UserPassword("app_rw", hostilePassword)
	appDSN := parsedDSN.String()

	appDB, err := sql.Open("pgx", appDSN)
	if err != nil {
		t.Fatalf("open app_rw connection: %v", err)
	}
	defer func() { _ = appDB.Close() }()
	if err := appDB.PingContext(ctx); err != nil {
		t.Fatalf("app_rw could not authenticate with the literal password, so it was not stored verbatim: %v", err)
	}
}
