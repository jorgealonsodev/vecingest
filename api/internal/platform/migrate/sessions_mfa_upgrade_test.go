package migrate_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/platform/migrate"
)

// The schema-set version that introduced sessions.mfa_at, and the last
// one before it. The upgrade path between those two is what this file
// tests.
const (
	versionBeforeSessionsMFA = 9
	versionSessionsMFA       = 10
)

// TestSchemaSet_SessionsMFAUpgradePathLeavesExistingSessionsUnelevated
// exercises the path the RUNNING stack will actually take (docs/
// pendientes-despliegue.md: this deployment already exists), not the
// fresh-database path every other Testcontainers test takes: migrate up
// to 00009, create a session exactly as the pre-00010 code did, then
// apply 00010 on top.
//
// The property under test is that the pre-existing session comes out NOT
// second-factor authenticated. It is the whole reason mfa_at is nullable
// with no default: every session row that exists when this migration
// runs was issued without any TOTP challenge, so a DEFAULT now() would
// hand every live session -- including an attacker's -- the exact
// elevation the column exists to withhold. A fix that only holds on a
// fresh database is not a fix.
func TestSchemaSet_SessionsMFAUpgradePathLeavesExistingSessionsUnelevated(t *testing.T) {
	superuserDB, dsn := startPostgres(t)
	ctx := context.Background()

	t.Setenv("APP_DB_USER", "app_rw")
	t.Setenv("APP_DB_PASSWORD", "test-sessions-mfa-upgrade-password")
	if err := migrate.RunBootstrap(ctx, superuserDB); err != nil {
		t.Fatalf("RunBootstrap: %v", err)
	}

	ownerDSN, err := migrate.SchemaDSN(dsn)
	if err != nil {
		t.Fatalf("SchemaDSN: %v", err)
	}
	ownerDB, err := sql.Open("pgx", ownerDSN)
	if err != nil {
		t.Fatalf("open owner connection: %v", err)
	}
	t.Cleanup(func() { _ = ownerDB.Close() })

	provider, err := migrate.SchemaProvider(ownerDB)
	if err != nil {
		t.Fatalf("SchemaProvider: %v", err)
	}

	// The database as it stands in production today.
	if _, err := provider.UpTo(ctx, versionBeforeSessionsMFA); err != nil {
		t.Fatalf("migrate up to %d: %v", versionBeforeSessionsMFA, err)
	}

	var columnExists bool
	const columnQuery = `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name = 'sessions' AND column_name = 'mfa_at')`
	if err := ownerDB.QueryRowContext(ctx, columnQuery).Scan(&columnExists); err != nil {
		t.Fatalf("query mfa_at column: %v", err)
	}
	if columnExists {
		t.Fatalf("expected sessions.mfa_at NOT to exist at version %d -- this test's whole premise is upgrading a database that predates it", versionBeforeSessionsMFA)
	}

	// A session issued by the pre-00010 code: no TOTP challenge was ever
	// possible for it, and the INSERT could not have named a column that
	// did not exist.
	userID := uuid.New()
	if _, err := ownerDB.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, name, locale) VALUES ($1, $2, $3, $4, $5)`,
		userID, "pre-upgrade-admin@example.com", "argon2id$placeholder", "Pre Upgrade Admin", "es",
	); err != nil {
		t.Fatalf("insert pre-upgrade user: %v", err)
	}
	sessionID := uuid.New()
	if _, err := ownerDB.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, refresh_token_hash, family_id, platform, expires_at)
		 VALUES ($1, $2, $3, $4, 'web', now() + interval '30 days')`,
		sessionID, userID, []byte("pre-upgrade-refresh-hash"), uuid.New(),
	); err != nil {
		t.Fatalf("insert pre-upgrade session: %v", err)
	}

	// The deploy.
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("migrate up to head: %v", err)
	}

	current, err := provider.GetDBVersion(ctx)
	if err != nil {
		t.Fatalf("GetDBVersion: %v", err)
	}
	if current < versionSessionsMFA {
		t.Fatalf("expected the schema set to reach at least version %d, got %d", versionSessionsMFA, current)
	}

	var mfaAt sql.NullTime
	if err := ownerDB.QueryRowContext(ctx, `SELECT mfa_at FROM sessions WHERE id = $1`, sessionID).Scan(&mfaAt); err != nil {
		t.Fatalf("read mfa_at of the pre-upgrade session: %v", err)
	}
	if mfaAt.Valid {
		t.Fatalf("expected the pre-upgrade session to read as NOT second-factor authenticated (mfa_at IS NULL), got %v -- the migration just elevated every session that existed before it", mfaAt.Time)
	}
}
