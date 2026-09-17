// Package schema -- 00004_river.go provisions River's own schema (D-L).
package schema

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
)

// RiverTargetVersion is the pinned River migration version this set
// applies (D-L). It must never track "latest": a dependency bump to
// river must not silently change the schema. Pinned against
// riverqueue/river v0.47.0, whose bundled Postgres "main" migration line
// has exactly 7 versions.
const RiverTargetVersion = 7

// riverMigration registers this set's version-4 Go migration. It must
// run NoTx (RunDB, not RunTx): River's own migration 6 depends on
// migration 4's ALTER TYPE ... ADD VALUE, and PostgreSQL forbids using a
// freshly added enum value inside the same transaction that added it.
// rivermigrate manages its own per-step transactions internally; a
// goose-level transaction wrapped around it would defeat that.
func riverMigration() *goose.Migration {
	return goose.NewGoMigration(4, &goose.GoFunc{RunDB: riverUp}, &goose.GoFunc{RunDB: riverDown})
}

func riverUp(ctx context.Context, db *sql.DB) error {
	driver := riverdatabasesql.New(db)
	migrator, err := rivermigrate.New(driver, nil)
	if err != nil {
		return fmt.Errorf("schema migration: build river migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, &rivermigrate.MigrateOpts{
		TargetVersion: RiverTargetVersion,
	}); err != nil {
		return fmt.Errorf("schema migration: apply river schema up to v%d: %w", RiverTargetVersion, err)
	}
	return grantRiverTables(ctx, db)
}

func riverDown(ctx context.Context, db *sql.DB) error {
	driver := riverdatabasesql.New(db)
	migrator, err := rivermigrate.New(driver, nil)
	if err != nil {
		return fmt.Errorf("schema migration (down): build river migrator: %w", err)
	}
	// TargetVersion -1 is rivermigrate's documented sentinel for "apply
	// all down migrations" (River schema removed completely).
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionDown, &rivermigrate.MigrateOpts{
		TargetVersion: -1,
	}); err != nil {
		return fmt.Errorf("schema migration (down): revert river schema: %w", err)
	}
	return nil
}

// grantRiverTables grants app_rw full CRUD on every table River's
// migration set creates, PLUS USAGE on every sequence backing one of
// those tables' identity/serial columns (river_job.id and friends).
// River tables are explicitly NOT append-only (D-L); invariant I1's
// assertion excludes them entirely because it only scans audit_log's
// own partition tree.
//
// The sequence grant is a fix for a gap invisible until M1: M0/PR1-3
// never called Client.Insert/InsertMany for any job kind (queue_test.go's
// own "river_job must stay empty" assertion), so a missing sequence
// grant never surfaced. Table-level GRANT INSERT does NOT implicitly
// grant USAGE on a table's own backing sequence in PostgreSQL -- they
// are separate privilege objects -- so the first real producer
// (invitations: task 6.17, river.InsertTx) failed with "permission
// denied for sequence river_job_id_seq" until this was added. This is a
// bugfix to an already-embedded Go migration function that has never
// run against a real deployment (Checkpoint A has not shipped yet), not
// a new migration: the pinned RiverTargetVersion and this function's
// migration version number (4) are unchanged.
//
// The grant is discovered by name pattern (river_*) rather than a
// hardcoded table list. The design's own D-L table list ("river_job,
// river_leader, river_client, river_client_queue, river_queue") is
// stale against the actually-pinned v0.47.0 schema, whose Postgres
// "main" migration line creates river_migration, river_job, river_leader,
// river_queue and river_notification -- river_client and
// river_client_queue do not exist as tables in this version. Pattern
// discovery is correct against either list and stays correct across a
// future non-breaking 0.x bump that adds or renames a table.
func grantRiverTables(ctx context.Context, db *sql.DB) error {
	const stmt = `
DO $$
DECLARE
  r RECORD;
BEGIN
  FOR r IN
    SELECT c.relname
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relname LIKE 'river\_%' ESCAPE '\'
  LOOP
    EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON %I TO app_rw', r.relname);
  END LOOP;

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
		return fmt.Errorf("schema migration: grant river tables to app_rw: %w", err)
	}
	return nil
}
