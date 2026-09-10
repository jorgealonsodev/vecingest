// Package migrate wires goose's embedded migration providers behind
// PostgreSQL session-level advisory locks (design D-A). goose does not
// lock by default; every provider built here goes through
// NewLockedProvider so that api and worker booting concurrently against
// an unmigrated database never race (platform-bootstrap: Embedded
// Migration Runner With Advisory Lock).
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"net/url"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/jorgealonsodev/vecingest/migrations/bootstrap"
	"github.com/jorgealonsodev/vecingest/migrations/schema"
)

// ownerRoleOption is libpq's startup-parameter form of `SET ROLE
// vecingest_owner`, pre-percent-encoded (%20 is a space, %3D an `=`) to
// match exactly what docker-compose.yml already runs in production. It
// has to be a connection-establishment parameter, not a `SET ROLE`
// statement this package issues itself, because it must cover every
// physical connection the schema provider's pool opens -- goose's own
// session-locked advisory-lock connection, whichever connection ends up
// creating goose_db_version_schema itself (before any of our own
// migrations run), and every connection River's own rivermigrate library
// opens through the *sql.DB it is handed. A `SET ROLE` issued after
// connecting only affects that one connection; each *.sql migration in
// migrations/schema additionally issues its own `SET ROLE
// vecingest_owner;` as defense in depth, but that cannot reach goose's or
// River's internal statements.
//
// This is deliberately NOT built with url.Values.Encode(): that encodes a
// space as `+`, and pgconn's own DSN query-string parser -- unlike
// net/url's decoder -- does not translate `+` back to a space in the
// options value, so a `+`-encoded DSN fails to connect with `unrecognized
// configuration parameter "+role"`. Percent-encoding (%20) round-trips
// correctly; that mismatch was caught by hand, running this exact
// derivation against a real Postgres container.
const ownerRoleOption = "-c%20role%3Dvecingest_owner"

// SchemaDSN derives the schema-set connection string from the bootstrap
// (superuser) DSN, appending the options startup parameter above so every
// connection opened from the result assumes vecingest_owner. This is the
// only DSN that can ever reach vecingest_owner's privileges:
// vecingest_owner is provisioned NOLOGIN by the bootstrap set
// (migrations/bootstrap/00001_roles.go), so no DSN can authenticate as it
// directly, and a superuser can SET ROLE to it regardless of role
// membership (migrations/schema/schema.go's own doc comment).
func SchemaDSN(bootstrapDSN string) (string, error) {
	u, err := url.Parse(bootstrapDSN)
	if err != nil {
		return "", fmt.Errorf("migrate: parse bootstrap dsn: %w", err)
	}
	if u.RawQuery == "" {
		u.RawQuery = "options=" + ownerRoleOption
	} else {
		u.RawQuery += "&options=" + ownerRoleOption
	}
	return u.String(), nil
}

// NewLockedProvider builds a goose provider guarded by a PostgreSQL
// session advisory lock pinned to lockID, tracking its own applied-version
// history in tableName. It never touches goose's global migration
// registry (WithDisableGlobalRegistry(true)): every migration this
// binary knows about is registered explicitly through goMigrations, so
// two providers in the same process can never collide over a shared
// global table.
//
// tableName MUST be distinct per independent migration set. Bootstrap
// and schema are numbered from 1 independently of each other (D-A: two
// embedded goose migration sets); if both tracked their applied versions
// in goose's shared default table name, the schema set's own version 1
// (0001_auth_schema.sql, which creates users/sessions/etc.) would look
// "already applied" the instant the bootstrap set's unrelated version 1
// committed, and goose would silently skip it -- exactly the failure
// mode a Testcontainers run against a freshly bootstrapped database
// caught (session package's rotation tests: "relation users does not
// exist" after both sets reported success).
func NewLockedProvider(db *sql.DB, fsys fs.FS, lockID int64, tableName string, goMigrations ...*goose.Migration) (*goose.Provider, error) {
	locker, err := lock.NewPostgresSessionLocker(lock.WithLockID(lockID))
	if err != nil {
		return nil, fmt.Errorf("migrate: build session locker for lock %d: %w", lockID, err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, fsys,
		goose.WithSessionLocker(locker),
		goose.WithDisableGlobalRegistry(true),
		goose.WithGoMigrations(goMigrations...),
		goose.WithTableName(tableName),
	)
	if err != nil {
		return nil, fmt.Errorf("migrate: build provider for lock %d: %w", lockID, err)
	}
	return provider, nil
}

// BootstrapProvider builds the superuser-run provider (D-A): exactly one
// Go migration at M0, guarded by the pinned bootstrap lock id, tracked
// in its own goose_db_version_bootstrap table (see NewLockedProvider's
// doc comment for why this must not be shared with the schema set).
func BootstrapProvider(db *sql.DB) (*goose.Provider, error) {
	return NewLockedProvider(db, bootstrap.FS, bootstrap.LockID, "goose_db_version_bootstrap", bootstrap.Migration())
}

// RunBootstrap applies every pending bootstrap-set migration.
func RunBootstrap(ctx context.Context, db *sql.DB) error {
	provider, err := BootstrapProvider(db)
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("migrate: run bootstrap set: %w", err)
	}
	return nil
}

// SchemaProvider builds the owner-run provider (D-A): the auth schema,
// audit_log, the D-B invariant assertions and River's schema, tracked in
// its own goose_db_version_schema table, guarded by
// the pinned schema-set lock id.
func SchemaProvider(db *sql.DB) (*goose.Provider, error) {
	return NewLockedProvider(db, schema.FS, schema.LockID, "goose_db_version_schema", schema.GoMigrations()...)
}

// RunSchema applies every pending schema-set migration.
func RunSchema(ctx context.Context, db *sql.DB) error {
	provider, err := SchemaProvider(db)
	if err != nil {
		return err
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate: run schema set: %w", err)
	}
	return nil
}

// Run applies the bootstrap set (over superuserDB) and then the schema
// set (over ownerDB, opened against the DSN SchemaDSN derives), in that
// order, matching the Data Flow diagram in design.md: provider A
// (superuser) completes and releases its lock before provider B (owner)
// acquires its own.
func Run(ctx context.Context, superuserDB, ownerDB *sql.DB) error {
	if err := RunBootstrap(ctx, superuserDB); err != nil {
		return err
	}
	if err := RunSchema(ctx, ownerDB); err != nil {
		return err
	}
	return nil
}
