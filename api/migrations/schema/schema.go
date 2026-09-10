// Package schema is the owner-run goose migration set (D-A): it creates
// the M0 auth tables, the partitioned audit_log with its append-only
// grants, the migration-time invariant assertions (D-B I1-I4), and
// River's own schema (D-L). It runs over the schema-set connection
// migrate.SchemaDSN derives from BOOTSTRAP_DATABASE_URL, under this
// set's own advisory lock -- distinct from the bootstrap set's lock id
// so the two sets serialize independently of one another.
//
// vecingest_owner is provisioned NOLOGIN by the bootstrap set
// (migrations/bootstrap/00001_roles.go): it exists to own every schema
// object, never to authenticate directly. The schema-set connection
// therefore connects as the superuser (the only role that can `SET ROLE
// vecingest_owner` regardless of role membership) and assumes the role at
// connection establishment via migrate.SchemaDSN's `options` startup
// parameter. Every .sql migration in this set additionally issues that
// SET ROLE as its own first statement, as defense in depth. This is
// structurally required, not cosmetic: the bootstrap set's `ALTER DEFAULT
// PRIVILEGES FOR ROLE vecingest_owner GRANT SELECT, INSERT ON TABLES TO
// app_rw` (D-B) only fires for objects actually *created by*
// vecingest_owner -- a table created by a different role gets no default
// privileges from that ACL entry at all, which would silently defeat the
// fail-closed baseline. It is also why the `options` startup parameter
// cannot be skipped in favor of relying on the .sql files' own SET ROLE
// alone: goose creates its own goose_db_version_schema tracking table,
// and River's own migrations run through rivermigrate, before or outside
// any statement this package controls.
package schema

import (
	"embed"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var FS embed.FS

// LockID is the pinned PostgreSQL session-advisory-lock id serializing
// concurrent access to this migration set (D-A). It must never change,
// and must never collide with bootstrap.LockID.
const LockID int64 = 5432002

// GoMigrations returns this set's explicitly-registered Go migrations
// (the invariant assertions and the River schema), for registration via
// goose.WithGoMigrations alongside the SQL migrations goose discovers in
// FS. Both kinds interleave by version number.
func GoMigrations() []*goose.Migration {
	return []*goose.Migration{
		assertInvariantsMigration(),
		riverMigration(),
	}
}
