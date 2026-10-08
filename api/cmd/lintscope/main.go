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
// Exceptions are recorded per QUERY, never per table (see
// queryExceptions): M1 gives audit_log real per-community rows, so the
// table-wide audit_log exception M0 carried is retired (design D-5), and
// a new tenant-blind query against any scoped table -- audit_log
// included -- fails CI instead of inheriting a blanket pass.
//
// Usage:
//
//	go run ./cmd/lintscope <path/to/queries/dir> [<path/to/schema/dir>]
//	go run ./cmd/lintscope ./internal/db/queries   # run from api/, schema defaults to migrations/schema
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// defaultSchemaDir is used when the caller supplies only the queries
// directory, matching how `make lint-scope` invokes this tool from api/.
const defaultSchemaDir = "migrations/schema"

// tenantColumns are the three canonical tenant-scope column names this
// checker enforces (openspec/config.yaml, PRD §7.3). A table carrying
// any one of these is a "scoped table" for the purpose of this check.
var tenantColumns = []string{"community_id", "office_id", "company_id"}

// queryExceptions is the query-level exception map design.md's
// tenant-scope subsection (D-5) specifies, keyed
// `filepath.Base(file):queryName` so the check is independent of which
// directory lintscope is invoked from. It is the ONLY exception
// mechanism: there is deliberately no table-level map, so "no scope
// filter" is always a recorded decision about one named query and never
// a blanket pass for a whole table. Every entry carries its reason.
var queryExceptions = map[string]string{
	"audit_log.sql:GetAuditLogHead": "design D-5: reads the head of the " +
		"single, global audit hash chain (D-O) to link the next entry; " +
		"the chain spans every tenant by construction, so a " +
		"community_id filter would break the chain, not scope it.",
	"audit_log.sql:ListAuditLogRange": "design D-5: the hash-chain " +
		"verifier walks a time range of the whole global chain; " +
		"verification is meaningful only over every tenant's entries " +
		"in order, so the read is intentionally cross-tenant.",
	"invitations.sql:SweepExpiredInvitations": "design D-6/PRD §7.4: the " +
		"daily invitations.expire sweep job is intentionally cross-" +
		"tenant — it is a periodic maintenance pass over every " +
		"community's past-expiry pending invitations, exactly the same " +
		"class of legitimate whole-table read/write as ListAuditLogRange.",
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

		if reason, excepted := queryExceptions[filepath.Base(q.file)+":"+q.name]; excepted {
			_ = reason // documented, not a silent skip — see queryExceptions' own comment
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
