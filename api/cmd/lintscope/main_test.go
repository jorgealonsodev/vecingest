package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFixture writes a single-file schema or queries fixture directory
// containing exactly one file with the given contents, returning the
// directory path.
func writeFixture(t *testing.T, filename, contents string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(contents), 0o600); err != nil {
		t.Fatalf("writing fixture %s: %v", filename, err)
	}
	return dir
}

// TestLint_RealRepoQueriesPass is a regression test: the actual M0
// schema and query set this repository ships MUST always pass. If this
// test fails, either a new tenant-scoped table needs a real filter, or
// a deliberate new exception needs to be added and documented in the
// exceptions map above, exactly like audit_log already is.
func TestLint_RealRepoQueriesPass(t *testing.T) {
	schemaDir := filepath.Join("..", "..", "migrations", "schema")
	queriesDir := filepath.Join("..", "..", "internal", "db", "queries")
	if _, err := os.Stat(schemaDir); err != nil {
		t.Skipf("schema dir not found at %s: %v", schemaDir, err)
	}
	if _, err := os.Stat(queriesDir); err != nil {
		t.Skipf("queries dir not found at %s: %v", queriesDir, err)
	}

	failures, err := lint(schemaDir, queriesDir)
	if err != nil {
		t.Fatalf("lint returned an error: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("expected the real M0 query set to pass, got failures: %v", failures)
	}
}

// TestLint_MissingTenantFilterFails proves the check is not a no-op: a
// query against a table that carries a tenant column, but never
// references that column anywhere in its own SQL text, MUST fail, named
// by table and column so the failure is actionable.
func TestLint_MissingTenantFilterFails(t *testing.T) {
	schemaDir := writeFixture(t, "0001_incidents.sql", `
CREATE TABLE incidents (
    id uuid PRIMARY KEY,
    community_id uuid NOT NULL,
    title text NOT NULL
);
`)
	queriesDir := writeFixture(t, "incidents.sql", `
-- name: ListAllIncidentsUnscoped :many
SELECT * FROM incidents ORDER BY id;
`)

	failures, err := lint(schemaDir, queriesDir)
	if err != nil {
		t.Fatalf("lint returned an error: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("expected exactly one failure, got: %v", failures)
	}
	if !strings.Contains(failures[0], "incidents") || !strings.Contains(failures[0], "community_id") {
		t.Errorf("expected the failure to name the table and its tenant column, got: %s", failures[0])
	}
	if !strings.Contains(failures[0], "ListAllIncidentsUnscoped") {
		t.Errorf("expected the failure to name the offending query, got: %s", failures[0])
	}
}

// TestLint_WhereFilteredQueryPasses proves a SELECT/UPDATE/DELETE that
// filters on the tenant column in its WHERE clause is accepted.
func TestLint_WhereFilteredQueryPasses(t *testing.T) {
	schemaDir := writeFixture(t, "0001_incidents.sql", `
CREATE TABLE incidents (
    id uuid PRIMARY KEY,
    community_id uuid NOT NULL,
    title text NOT NULL
);
`)
	queriesDir := writeFixture(t, "incidents.sql", `
-- name: ListIncidentsByCommunity :many
SELECT * FROM incidents WHERE community_id = $1 ORDER BY id;

-- name: DeleteIncidentScoped :exec
DELETE FROM incidents WHERE id = $1 AND community_id = $2;
`)

	failures, err := lint(schemaDir, queriesDir)
	if err != nil {
		t.Fatalf("lint returned an error: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("expected both tenant-filtered queries to pass, got failures: %v", failures)
	}
}

// TestLint_InsertWithTenantColumnInValuesPasses proves an INSERT that
// carries the tenant column explicitly in its column list (there is no
// WHERE clause to filter on for a write) counts as constraining it.
func TestLint_InsertWithTenantColumnInValuesPasses(t *testing.T) {
	schemaDir := writeFixture(t, "0001_incidents.sql", `
CREATE TABLE incidents (
    id uuid PRIMARY KEY,
    community_id uuid NOT NULL,
    title text NOT NULL
);
`)
	queriesDir := writeFixture(t, "incidents.sql", `
-- name: InsertIncident :one
INSERT INTO incidents (id, community_id, title)
VALUES ($1, $2, $3)
RETURNING *;
`)

	failures, err := lint(schemaDir, queriesDir)
	if err != nil {
		t.Fatalf("lint returned an error: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("expected the tenant-column INSERT to pass, got failures: %v", failures)
	}
}

// TestLint_UnscopedTableNeverFlagged proves a table with none of the
// three tenant columns (e.g. an identity-scoped M0 table like users) is
// never flagged, regardless of what its queries filter on.
func TestLint_UnscopedTableNeverFlagged(t *testing.T) {
	schemaDir := writeFixture(t, "0001_users.sql", `
CREATE TABLE users (
    id uuid PRIMARY KEY,
    email text NOT NULL UNIQUE
);
`)
	queriesDir := writeFixture(t, "users.sql", `
-- name: ListAllUsers :many
SELECT * FROM users;
`)

	failures, err := lint(schemaDir, queriesDir)
	if err != nil {
		t.Fatalf("lint returned an error: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("expected an unscoped table to never be flagged, got failures: %v", failures)
	}
}

// TestLint_DocumentedExceptionSkipsEnforcement proves the exceptions
// map actually takes effect: a query against a scoped-but-excepted
// table (audit_log at M0) is never flagged even when it references
// nothing about its tenant column.
func TestLint_DocumentedExceptionSkipsEnforcement(t *testing.T) {
	schemaDir := writeFixture(t, "0002_audit_log.sql", `
CREATE TABLE audit_log (
    id uuid NOT NULL,
    community_id uuid,
    action text NOT NULL
);
`)
	queriesDir := writeFixture(t, "audit_log.sql", `
-- name: GetAuditLogHead :one
SELECT hash, created_at, id FROM audit_log ORDER BY created_at DESC, id DESC LIMIT 1;

-- name: ListAuditLogRange :many
SELECT * FROM audit_log WHERE created_at >= $1 AND created_at <= $2 ORDER BY created_at ASC, id ASC;
`)

	failures, err := lint(schemaDir, queriesDir)
	if err != nil {
		t.Fatalf("lint returned an error: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("expected the documented audit_log exception to suppress enforcement, got failures: %v", failures)
	}
}

// TestExtractTables_HandlesNestedParensInCheckConstraint proves the
// paren-depth column splitter is not confused by a CHECK constraint
// containing its own parens on the same line as a column definition —
// exactly the shape api/migrations/schema/00001_auth_schema.sql uses
// for `sessions.platform`.
func TestExtractTables_HandlesNestedParensInCheckConstraint(t *testing.T) {
	sql := `
CREATE TABLE sessions (
    id uuid PRIMARY KEY,
    community_id uuid NOT NULL,
    platform text NOT NULL CHECK (platform IN ('ios', 'android', 'web')),
    PRIMARY KEY (id)
);
`
	tables := extractTables(sql)
	cols, ok := tables["sessions"]
	if !ok {
		t.Fatalf("expected to find table 'sessions', got tables: %v", tables)
	}

	want := map[string]bool{"id": true, "community_id": true, "platform": true}
	got := map[string]bool{}
	for _, c := range cols {
		got[c] = true
	}
	for name := range want {
		if !got[name] {
			t.Errorf("expected column %q to be extracted, got columns: %v", name, cols)
		}
	}
	if got["primary"] {
		t.Errorf("expected the trailing table-level PRIMARY KEY(...) constraint to be excluded, got columns: %v", cols)
	}
}

// TestLint_MultipleScopedColumnsAcrossFiles proves office_id and
// company_id are recognized the same way as community_id.
func TestLint_MultipleScopedColumnsAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	schema := `
CREATE TABLE offices (
    id uuid PRIMARY KEY,
    office_id uuid NOT NULL
);
CREATE TABLE invoices (
    id uuid PRIMARY KEY,
    company_id uuid NOT NULL
);
`
	if err := os.WriteFile(filepath.Join(dir, "schema.sql"), []byte(schema), 0o600); err != nil {
		t.Fatalf("writing schema fixture: %v", err)
	}
	queriesDir := t.TempDir()
	queries := `
-- name: ListOfficesUnscoped :many
SELECT * FROM offices;

-- name: ListInvoicesScoped :many
SELECT * FROM invoices WHERE company_id = $1;
`
	if err := os.WriteFile(filepath.Join(queriesDir, "q.sql"), []byte(queries), 0o600); err != nil {
		t.Fatalf("writing queries fixture: %v", err)
	}

	failures, err := lint(dir, queriesDir)
	if err != nil {
		t.Fatalf("lint returned an error: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("expected exactly one failure (offices, unscoped), got: %v", failures)
	}
	if !strings.Contains(failures[0], "offices") || !strings.Contains(failures[0], "office_id") {
		t.Errorf("expected the failure to name offices/office_id, got: %s", failures[0])
	}
}
