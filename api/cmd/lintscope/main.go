// Command lintscope fails loudly when a sqlc query against a
// tenant-scoped table omits its tenant column (openspec/config.yaml:
// "No ORM. Every sqlc query against a scoped table MUST filter by its
// tenant column (community_id/office_id/company_id) — this is enforced
// by `make lint-scope`."). No ORM means no query builder stops a
// hand-written SELECT/UPDATE/DELETE from silently reading or writing
// across tenants; this tool is that missing guard, run structurally
// against the raw `.sql` query files sqlc consumes (no database
// connection required, so it works in the fast CI lane too).
//
// At M0 the auth-subset schema has exactly one table carrying a tenant
// column: audit_log.community_id, and design.md documents it as a
// deliberate M0 exception (see the exceptions map below) — every other
// M0 table is identity-scoped (user_id/id), not tenant-scoped, and
// carries none of the three columns this tool enforces. That makes
// today's run trivially green, which is the point: the check exists and
// is wired into CI now, so the first M1+ query against a real scoped
// table (community_id on an incident, office_id on a document, …) that
// forgets its tenant filter fails CI instead of leaking across
// communities in production.
//
// Usage:
//
//	go run ./cmd/lintscope <path/to/queries/dir> [<path/to/schema/dir>]
//	go run ./cmd/lintscope ./internal/db/queries   # run from api/, schema defaults to migrations/schema
package main

import (
	"fmt"
	"os"
)

// defaultSchemaDir is used when the caller supplies only the queries
// directory, matching how `make lint-scope` invokes this tool from api/.
const defaultSchemaDir = "migrations/schema"

// tenantColumns are the three canonical tenant-scope column names this
// checker enforces (openspec/config.yaml, PRD §7.3). A table carrying
// any one of these is a "scoped table" for the purpose of this check.
var tenantColumns = []string{"community_id", "office_id", "company_id"}

// exceptions documents tables that carry a tenant column but are
// deliberately excluded from enforcement, so "no scope filter" is
// always a recorded decision and never a silent oversight — the same
// discipline design.md's own tenant-scope subsection applies to "no
// scope column at all".
var exceptions = map[string]string{
	"audit_log": "design.md (tenant-scope subsection): community_id is " +
		"nullable and legitimately NULL for M0 authentication events — " +
		"there is no tenant context to filter by yet. InsertAuditLog " +
		"already carries the column explicitly; the two read queries " +
		"(GetAuditLogHead, ListAuditLogRange) intentionally read the " +
		"whole hash chain. Revisit when M1 gives audit_log real " +
		"per-community rows.",
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: lintscope <path/to/queries/dir> [<path/to/schema/dir>]")
		os.Exit(2)
	}

	queriesDir := os.Args[1]
	schemaDir := defaultSchemaDir
	if len(os.Args) >= 3 {
		schemaDir = os.Args[2]
	}

	failures, err := lint(schemaDir, queriesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lintscope: %v\n", err)
		os.Exit(1)
	}

	if len(failures) > 0 {
		fmt.Fprintln(os.Stderr, "lintscope: tenant-scope gaps found:")
		for _, f := range failures {
			fmt.Fprintf(os.Stderr, "  - %s\n", f)
		}
		os.Exit(1)
	}

	fmt.Println("lintscope: OK — every query against a tenant-scoped table references its tenant column")
}

// lint parses every CREATE TABLE in schemaDir's *.sql files to find
// which tables carry a tenant column, then checks every named query in
// queriesDir's *.sql files against that map. It returns one failure
// string per violation and is exported at package scope (unexported,
// lowercase — this is package main) so tests can drive it directly
// against fixture directories.
func lint(schemaDir, queriesDir string) ([]string, error) {
	tableTenantCol, err := scanSchemaDir(schemaDir)
	if err != nil {
		return nil, fmt.Errorf("scanning schema dir %q: %w", schemaDir, err)
	}

	queries, err := scanQueriesDir(queriesDir)
	if err != nil {
		return nil, fmt.Errorf("scanning queries dir %q: %w", queriesDir, err)
	}

	var failures []string
	for _, q := range queries {
		if q.table == "" {
			// No FROM/INTO/UPDATE target could be extracted (e.g. a
			// pure DO block or an unsupported statement shape) — not
			// this tool's concern, sqlc itself will reject unparsable
			// SQL long before this check runs.
			continue
		}

		tenantCol, scoped := tableTenantCol[q.table]
		if !scoped {
			continue // not a tenant-scoped table at all
		}

		if reason, excepted := exceptions[q.table]; excepted {
			_ = reason // documented, not a silent skip — see the exceptions map's own comment
			continue
		}

		if !containsWord(q.body, tenantCol) {
			failures = append(failures, fmt.Sprintf(
				"%s: query %q against tenant-scoped table %q does not reference its tenant column %q",
				q.file, q.name, q.table, tenantCol))
		}
	}

	return failures, nil
}
