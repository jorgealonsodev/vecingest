package test

import (
	"database/sql"
	"testing"
)

// assertTableExists queries information_schema.tables directly rather
// than a cached catalog snapshot, so it reflects the exact state of db
// at call time (used both after Up and after a subsequent DownTo/Up).
func assertTableExists(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	var exists bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)`, table).Scan(&exists); err != nil {
		t.Fatalf("query table existence for %q: %v", table, err)
	}
	if !exists {
		t.Fatalf("expected table %q to exist", table)
	}
}

func assertTableAbsent(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	var exists bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)`, table).Scan(&exists); err != nil {
		t.Fatalf("query table existence for %q: %v", table, err)
	}
	if exists {
		t.Fatalf("expected table %q to be gone", table)
	}
}

// assertAppRWCanMutate proves the fail-closed default-privilege baseline
// (D-B) was overridden by an explicit GRANT UPDATE, DELETE on table --
// every M1 table is mutable (design D-5), unlike append-only audit_log.
func assertAppRWCanMutate(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	var canUpdate, canDelete bool
	if err := db.QueryRow(`SELECT has_table_privilege('app_rw', $1, 'UPDATE')`, table).Scan(&canUpdate); err != nil {
		t.Fatalf("query UPDATE privilege for %q: %v", table, err)
	}
	if !canUpdate {
		t.Fatalf("expected app_rw to have UPDATE on %q", table)
	}
	if err := db.QueryRow(`SELECT has_table_privilege('app_rw', $1, 'DELETE')`, table).Scan(&canDelete); err != nil {
		t.Fatalf("query DELETE privilege for %q: %v", table, err)
	}
	if !canDelete {
		t.Fatalf("expected app_rw to have DELETE on %q", table)
	}
}

func assertIndexExists(t *testing.T, db *sql.DB, index string) {
	t.Helper()
	var exists bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = $1)`, index).Scan(&exists); err != nil {
		t.Fatalf("query index existence for %q: %v", index, err)
	}
	if !exists {
		t.Fatalf("expected index %q to exist", index)
	}
}
