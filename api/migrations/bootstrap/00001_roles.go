// Package bootstrap is the superuser-run goose migration set (D-A): it
// provisions the three-role model, the append-only registry table and
// event trigger, and the fail-closed default-privilege baseline. It
// contains exactly one Go migration because only Go code can read
// APP_DB_USER/APP_DB_PASSWORD from the environment (db-access-control:
// Three-Role Provisioning) -- SQL alone cannot.
package bootstrap

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"strings"

	"github.com/pressly/goose/v3"
)

// FS embeds this package's directory. No .sql file exists in this set at
// M0 -- the sole migration is Go -- but goose's own doc comment confirms
// a Go migration registered via WithGoMigrations needs no matching fsys
// entry at all: "The path field may be empty for manually registered
// migrations, such as Go migrations registered using the WithGoMigrations
// option." Embedding the package directory (rather than a `*.sql` glob
// that would fail to compile with zero matches) keeps this package ready
// to gain a real .sql file later with no structural change.
//
//go:embed *
var FS embed.FS

// LockID is the pinned PostgreSQL session-advisory-lock id serializing
// concurrent access to this migration set (D-A). It must never change:
// distinct, stable lock ids are what let the bootstrap and schema sets
// serialize independently of one another.
const LockID int64 = 5432001

// DefaultAppDBUser is used when APP_DB_USER is not set. internal/config
// also defaults it in the same way once Phase 6 wires that requirement
// set; this constant lets the migration run standalone in a test.
const DefaultAppDBUser = "app_rw"

// Migration returns this set's single Go migration for explicit
// registration via goose.WithGoMigrations. It is not registered through
// goose's global registry: internal/platform/migrate wires every
// provider explicitly so two providers in one binary can never collide
// over a shared global migration table.
func Migration() *goose.Migration {
	return goose.NewGoMigration(1, &goose.GoFunc{RunDB: Up}, &goose.GoFunc{RunDB: Down})
}

// Up provisions vecingest_owner and app_rw, narrows schema ownership and
// default privileges to the fail-closed SELECT+INSERT baseline (D-B),
// and installs the append-only guard's registry table, function and
// event trigger (D-B mechanism 2). It runs with no transaction (NoTx):
// the GoFunc's RunDB signature is what selects that mode.
func Up(ctx context.Context, db *sql.DB) error {
	appDBUser := os.Getenv("APP_DB_USER")
	if appDBUser == "" {
		appDBUser = DefaultAppDBUser
	}
	appDBPassword := os.Getenv("APP_DB_PASSWORD")
	if appDBPassword == "" {
		return fmt.Errorf("bootstrap migration: APP_DB_PASSWORD is required and was empty")
	}

	quotedUser := quoteIdent(appDBUser)
	quotedPassword := quoteLiteral(appDBPassword)

	stmts := []string{
		`DO $do$ BEGIN
			IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'vecingest_owner') THEN
				CREATE ROLE vecingest_owner NOLOGIN;
			END IF;
		END $do$;`,

		`CREATE EXTENSION IF NOT EXISTS btree_gist;`,
		`CREATE EXTENSION IF NOT EXISTS pg_stat_statements;`,

		`ALTER SCHEMA public OWNER TO vecingest_owner;`,
		`REVOKE ALL ON SCHEMA public FROM PUBLIC;`,
		fmt.Sprintf(`GRANT USAGE ON SCHEMA public TO %s;`, quotedUser),

		// Fail-closed baseline (D-B): a brand-new table is append-only
		// shaped by default; a mutable table's own migration must grant
		// UPDATE/DELETE explicitly.
		fmt.Sprintf(`ALTER DEFAULT PRIVILEGES FOR ROLE vecingest_owner GRANT SELECT, INSERT ON TABLES TO %s;`, quotedUser),

		`CREATE TABLE IF NOT EXISTS append_only_relations (
			relname text PRIMARY KEY
		);`,
		`ALTER TABLE append_only_relations OWNER TO vecingest_owner;`,
		fmt.Sprintf(`GRANT SELECT ON append_only_relations TO %s;`, quotedUser),
		`INSERT INTO append_only_relations (relname) VALUES ('audit_log') ON CONFLICT DO NOTHING;`,

		guardFunctionSQL,
		`DROP EVENT TRIGGER IF EXISTS vecingest_append_only_guard;`,
		`CREATE EVENT TRIGGER vecingest_append_only_guard ON ddl_command_end
			WHEN TAG IN ('CREATE TABLE', 'CREATE TABLE AS', 'ALTER TABLE')
			EXECUTE FUNCTION vecingest_append_only_guard_fn();`,
		// An event-trigger function cannot be called directly from SQL,
		// but the revoke costs nothing and keeps the I2 scan clean.
		`REVOKE EXECUTE ON FUNCTION vecingest_append_only_guard_fn() FROM PUBLIC;`,
	}

	// gosec's G701 taint analysis flags this ExecContext because every
	// element of stmts traces back to fmt.Sprintf/string interpolation
	// (DDL cannot be parameterized the way DML values can, e.g. CREATE
	// ROLE %I LOGIN PASSWORD %L). The interpolated values are proven safe
	// by construction, not assumed: every dynamic identifier goes through
	// quoteIdent (pgx.Identifier{}.Sanitize(), sql_quote.go), and the only
	// dynamic literal (the role password) is no longer interpolated here
	// at all -- it moved to ensureAppRole, because quote-doubling is NOT
	// sufficient inside a dollar-quoted block (see that function). What
	// remains in stmts interpolates only quoteIdent'd identifiers, whose
	// doubled double-quotes are correct in the ordinary parsing context
	// these statements use. quoteLiteral doubles embedded single quotes --
	// the standard, sufficient escaping for a string literal in that same
	// ordinary context, under Postgres's standard_conforming_strings
	// = on default (in effect on every Postgres release since 9.1, and
	// never altered by any migration in this set, so backslash sequences
	// are never re-interpreted). Both appDBUser and appDBPassword also
	// come from this process's own environment (APP_DB_USER/APP_DB_PASSWORD),
	// set by the operator deploying the stack, not from any external or
	// request-controlled input.
	// The app role is created OUTSIDE the stmts loop, and deliberately not
	// inside a DO $do$...$do$ block. See ensureAppRole for why that
	// distinction is load-bearing rather than stylistic.
	if err := ensureAppRole(ctx, db, appDBUser, quotedUser, quotedPassword); err != nil {
		return err
	}

	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil { //nolint:gosec // G701: every dynamic value in stmt is quoted via quoteIdent/quoteLiteral -- see comment above
			return fmt.Errorf("bootstrap migration: %w (statement: %s)", err, firstLine(stmt))
		}
	}

	return assertAppRwNotOwnerMember(ctx, db, appDBUser)
}

// Down reverses Up, for local development rollback only
// (`goose down` to zero; drop DB is the documented rollback boundary).
func Down(ctx context.Context, db *sql.DB) error {
	appDBUser := os.Getenv("APP_DB_USER")
	if appDBUser == "" {
		appDBUser = DefaultAppDBUser
	}
	quotedUser := quoteIdent(appDBUser)

	stmts := []string{
		`DROP EVENT TRIGGER IF EXISTS vecingest_append_only_guard;`,
		`DROP FUNCTION IF EXISTS vecingest_append_only_guard_fn();`,
		`DROP TABLE IF EXISTS append_only_relations;`,
		`ALTER DEFAULT PRIVILEGES FOR ROLE vecingest_owner REVOKE SELECT, INSERT ON TABLES FROM ` + quotedUser + `;`,
		`REVOKE USAGE ON SCHEMA public FROM ` + quotedUser + `;`,
		`DROP ROLE IF EXISTS ` + quotedUser + `;`,
		`DROP ROLE IF EXISTS vecingest_owner;`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("bootstrap migration (down): %w (statement: %s)", err, firstLine(stmt))
		}
	}
	return nil
}

// assertAppRwNotOwnerMember is the data-flow diagram's own explicit
// "assert: app_rw NOT member of vecingest_owner" check (db-access-control:
// Three-Role Provisioning / I3). Nothing above ever grants that
// membership; this fails the migration loudly if it is ever introduced
// by accident.
func assertAppRwNotOwnerMember(ctx context.Context, db *sql.DB, appDBUser string) error {
	const q = `
		SELECT EXISTS (
			SELECT 1
			FROM pg_auth_members m
			JOIN pg_roles owner ON owner.oid = m.roleid AND owner.rolname = 'vecingest_owner'
			JOIN pg_roles member ON member.oid = m.member AND member.rolname = $1
		)`
	var isMember bool
	if err := db.QueryRowContext(ctx, q, appDBUser).Scan(&isMember); err != nil {
		return fmt.Errorf("bootstrap migration: assert app_rw not owner member: %w", err)
	}
	if isMember {
		return fmt.Errorf("bootstrap migration: invariant violated: %s is a member of vecingest_owner", appDBUser)
	}
	return nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}

// ensureAppRole creates or updates the application login role.
//
// It is a separate function, and deliberately not wrapped in a
// DO $do$ ... END $do$ block, because it used to be. Inside a
// dollar-quoted string Postgres performs NO quote processing and scans
// only for the closing tag, so quoteLiteral's doubled single quotes
// protect nothing there: an APP_DB_PASSWORD containing the literal text
// `$do$` closed the block early and the rest parsed at top level.
//
// The injected SQL did not actually execute, which was established by
// running it rather than by reasoning about it (see
// TestRunMigrate_AppDBPasswordCannotInjectSQL). Closing the block leaves
// the DO body ending in `... PASSWORD 'x` -- an unterminated string
// literal -- and the attacker cannot close it, because quoteLiteral
// doubles every `'` they supply. Postgres rejects the PL/pgSQL body at
// parse time, and the simple query protocol parses the whole batch before
// executing any of it.
//
// So the observed defect was narrower than it looks: any operator
// password containing `$do$` broke deployment with an opaque syntax
// error. The reason to fix it structurally anyway is that the safety
// rested on an accident -- the shape of the surrounding statement -- and
// not on the escaping being correct. Reorder or reword that statement so
// the body still parses, and the same construct becomes remote-code-shaped
// against the bootstrap SUPERUSER connection, which owns vecingest_owner,
// the append-only event trigger and every default privilege in D-B.
//
// Asking pg_roles with a BOUND parameter and then issuing bare DDL removes
// the dollar-quoted context altogether. Outside it, doubling single quotes
// is correct and sufficient under standard_conforming_strings = on.
// Rejecting `$` outright was considered and dropped: passwords legitimately
// contain it, and generated ones frequently do.
func ensureAppRole(ctx context.Context, db *sql.DB, appDBUser, quotedUser, quotedPassword string) error {
	// $1 is a real bound parameter, so this lookup uses the extended
	// protocol and the role name cannot influence how it is parsed.
	var exists bool
	if err := db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, appDBUser,
	).Scan(&exists); err != nil {
		return fmt.Errorf("bootstrap migration: looking up role %q: %w", appDBUser, err)
	}

	verb := "CREATE"
	if exists {
		verb = "ALTER"
	}

	stmt := fmt.Sprintf(`%s ROLE %s LOGIN PASSWORD %s;`, verb, quotedUser, quotedPassword)
	if _, err := db.ExecContext(ctx, stmt); err != nil { //nolint:gosec // G701: identifier via quoteIdent, password via quoteLiteral, and no dollar-quoted context -- see the doc comment above
		return fmt.Errorf("bootstrap migration: %s ROLE %q: %w", verb, appDBUser, err)
	}
	return nil
}
