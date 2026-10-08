package schema

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

// riverSequenceGrantsMigration grants app_rw USAGE on the sequences
// backing River's identity columns.
//
// This is a fix for a gap in migration 4, and it is a SEPARATE migration
// rather than an edit to that one on purpose. Migration 4 has already run
// against the deployed stack (docs/pendientes-despliegue.md records it
// running on Portainer since 2026-09-08), and goose records applied
// versions in goose_db_version_schema. Editing 4 in place would therefore
// fix a fresh database and silently leave every existing one broken --
// goose would see version 4 applied and skip it. A new version is the only
// form of the fix that reaches both.
//
// The gap itself: in PostgreSQL a table grant never implies a grant on
// that table's own backing sequence; they are separate privilege objects.
// Migration 4 grants SELECT/INSERT/UPDATE/DELETE on every river_* table
// but nothing on river_job_id_seq and its siblings. It stayed invisible
// through M0 and PR1-PR3 because nothing ever produced a job (queue_test.go
// asserts river_job stays empty). M1's invitation email is the first real
// producer, and it failed with "permission denied for sequence
// river_job_id_seq".
//
// GRANT is idempotent, so this is safe to apply to a database that somehow
// already has the privilege.
func riverSequenceGrantsMigration() *goose.Migration {
	return goose.NewGoMigration(9, &goose.GoFunc{RunDB: riverSequenceGrantsUp}, &goose.GoFunc{RunDB: riverSequenceGrantsDown})
}

func riverSequenceGrantsUp(ctx context.Context, db *sql.DB) error {
	const stmt = `
DO $$
DECLARE
  r RECORD;
BEGIN
  FOR r IN
    SELECT c.relname
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public' AND c.relkind = 'S' AND c.relname LIKE 'river\_%' ESCAPE '\'
  LOOP
    EXECUTE format('GRANT USAGE, SELECT ON SEQUENCE %I TO app_rw', r.relname);
  END LOOP;
END;
$$;
`
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("schema migration: grant river sequences to app_rw: %w", err)
	}
	return nil
}

func riverSequenceGrantsDown(ctx context.Context, db *sql.DB) error {
	const stmt = `
DO $$
DECLARE
  r RECORD;
BEGIN
  FOR r IN
    SELECT c.relname
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public' AND c.relkind = 'S' AND c.relname LIKE 'river\_%' ESCAPE '\'
  LOOP
    EXECUTE format('REVOKE USAGE, SELECT ON SEQUENCE %I FROM app_rw', r.relname);
  END LOOP;
END;
$$;
`
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("schema migration: revoke river sequences from app_rw: %w", err)
	}
	return nil
}
