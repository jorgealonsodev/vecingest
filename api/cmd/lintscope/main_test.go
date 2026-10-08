package main

import (
	"os"
	"path/filepath"
	"sort"
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

// TestLint_RealRepoQueriesPass is a regression test: the actual schema
// and query set this repository ships MUST always pass. If this test
// fails, either a query against a tenant-scoped table needs a real
// filter, or a deliberate new exception needs to be added, with its
// reason, to queryExceptions -- one named query at a time.
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

// TestLint_RealSchemaScopesEveryM1TenantTable is task 8.7's coverage
// half: "green" means something only if lint-scope actually sees every
// M1 tenant table as scoped, with the tenant column design D-5 assigns
// it. offices is absent on purpose: its tenant is its own id, which is
// none of the three tenant columns.
func TestLint_RealSchemaScopesEveryM1TenantTable(t *testing.T) {
	schemaDir := filepath.Join("..", "..", "migrations", "schema")
	if _, err := os.Stat(schemaDir); err != nil {
		t.Skipf("schema dir not found at %s: %v", schemaDir, err)
	}
	tables, err := scanSchemaDir(schemaDir)
	if err != nil {
		t.Fatalf("scanning schema: %v", err)
	}
	want := map[string]string{
		"office_members": "office_id",
		"communities":    "office_id",
		"units":          "community_id",
		"unit_members":   "community_id",
		"invitations":    "community_id",
		"audit_log":      "community_id",
	}
	for table, col := range want {
		if got := tables[table]; got != col {
			t.Errorf("expected table %q to be scoped by %q, got %q", table, col, got)
		}
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

// TestLint_TenantBlindNewAuditQueryFails is task 8.5: with the
// table-wide audit_log exception retired (design D-5), a NEW audit_log
// query that never references community_id fails, while the two
// whole-chain reads stay excepted by name and the writer passes on its
// own merit (it lists community_id).
func TestLint_TenantBlindNewAuditQueryFails(t *testing.T) {
	schemaDir := writeFixture(t, "0002_audit_log.sql", `
CREATE TABLE audit_log (
    id uuid NOT NULL,
    community_id uuid,
    user_id uuid,
    action text NOT NULL
);
`)
	queriesDir := writeFixture(t, "audit_log.sql", `
-- name: InsertAuditLog :one
INSERT INTO audit_log (id, user_id, community_id, action) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: GetAuditLogHead :one
SELECT hash, created_at, id FROM audit_log ORDER BY created_at DESC, id DESC LIMIT 1;

-- name: ListAuditLogRange :many
SELECT * FROM audit_log WHERE created_at >= $1 AND created_at <= $2 ORDER BY created_at ASC, id ASC;

-- name: ListAuditLogByUser :many
SELECT * FROM audit_log WHERE user_id = $1;
`)

	failures, err := lint(schemaDir, queriesDir)
	if err != nil {
		t.Fatalf("lint returned an error: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("expected exactly one failure (the tenant-blind ListAuditLogByUser), got: %v", failures)
	}
	if !strings.Contains(failures[0], "ListAuditLogByUser") {
		t.Errorf("expected the failure to name ListAuditLogByUser, got: %s", failures[0])
	}
}

// TestQueryExceptions_AuditLogHoldsExactlyTheTwoChainReads is task 8.6's
// shape check: the audit_log entries of the query-level exception map
// are exactly GetAuditLogHead and ListAuditLogRange, every entry in the
// map carries a reason, and no table-wide exception mechanism remains.
func TestQueryExceptions_AuditLogHoldsExactlyTheTwoChainReads(t *testing.T) {
	var auditKeys []string
	for key, reason := range queryExceptions {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("query exception %q has no reason", key)
		}
		if strings.HasPrefix(key, "audit_log.sql:") {
			auditKeys = append(auditKeys, key)
		}
	}
	sort.Strings(auditKeys)
	want := []string{"audit_log.sql:GetAuditLogHead", "audit_log.sql:ListAuditLogRange"}
	if strings.Join(auditKeys, ",") != strings.Join(want, ",") {
		t.Fatalf("expected audit_log query exceptions %v, got %v", want, auditKeys)
	}
}

// TestLint_QueryLevelExceptionSkipsEnforcement proves queryExceptions
// (design.md's "file:queryName" exception map, task 8.6's own
// mechanism, introduced early for invitations.sql:SweepExpiredInvitations)
// suppresses enforcement for exactly the named query, keyed by base
// filename regardless of invocation directory, and does NOT blanket-
// exempt every other query against the same scoped table.
func TestLint_QueryLevelExceptionSkipsEnforcement(t *testing.T) {
	schemaDir := writeFixture(t, "0008_invitations.sql", `
CREATE TABLE invitations (
    id uuid PRIMARY KEY,
    community_id uuid NOT NULL,
    status text NOT NULL
);
`)
	queriesDir := writeFixture(t, "invitations.sql", `
-- name: SweepExpiredInvitations :execrows
UPDATE invitations SET status = 'blocked' WHERE status = 'pending';

-- name: ListInvitationsUnscoped :many
SELECT * FROM invitations;
`)

	failures, err := lint(schemaDir, queriesDir)
	if err != nil {
		t.Fatalf("lint returned an error: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("expected exactly one failure (ListInvitationsUnscoped, SweepExpiredInvitations excepted), got: %v", failures)
	}
	if !strings.Contains(failures[0], "ListInvitationsUnscoped") {
		t.Errorf("expected the one remaining failure to name ListInvitationsUnscoped, got: %s", failures[0])
	}
	if strings.Contains(failures[0], "SweepExpiredInvitations") {
		t.Errorf("expected SweepExpiredInvitations to be excepted, got it flagged: %s", failures[0])
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
