package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// createTableRe finds the start of a CREATE TABLE statement and
// captures its (possibly quoted) name. Column definitions are then
// extracted by a paren-depth walk from the opening `(` that follows,
// which is the only reliable way to split columns given nested parens
// inside a column definition (e.g. `platform text CHECK (platform IN
// ('ios', 'android'))`).
var createTableRe = regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?"?([a-zA-Z_][a-zA-Z0-9_]*)"?\s*\(`)

// columnClauseKeywords are the leading tokens of a table constraint
// clause rather than a column definition (PRIMARY KEY (...), FOREIGN
// KEY (...), etc.) — these are skipped when collecting column names.
var columnClauseKeywords = map[string]bool{
	"primary":    true,
	"foreign":    true,
	"unique":     true,
	"check":      true,
	"constraint": true,
	"exclude":    true,
}

// scanSchemaDir walks every *.sql file in dir and returns a map of
// table name -> tenant column name, for every table whose column list
// contains one of tenantColumns. Tables with none of the three are
// simply absent from the map (not "scoped" at all).
func scanSchemaDir(dir string) (map[string]string, error) {
	result := make(map[string]string)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name())) //nolint:gosec // G304: dir/entry come from a fixed, repo-committed migrations path (or a test-controlled fixture dir), never untrusted request input — same pattern as cmd/lintcompose's os.ReadFile.
		if err != nil {
			return nil, err
		}

		for table, cols := range extractTables(string(data)) {
			for _, col := range cols {
				if tenant, ok := matchTenantColumn(col); ok {
					result[table] = tenant
					break
				}
			}
		}
	}

	return result, nil
}

// extractTables finds every CREATE TABLE statement in sql and returns a
// map of table name -> column names (constraint clauses excluded).
func extractTables(sql string) map[string][]string {
	tables := make(map[string][]string)

	for _, loc := range createTableRe.FindAllStringSubmatchIndex(sql, -1) {
		name := sql[loc[2]:loc[3]]
		// loc[1] is the index right after the opening '(' captured by
		// the trailing `\(` in createTableRe.
		body, ok := balancedParenBody(sql, loc[1]-1)
		if !ok {
			continue
		}
		tables[name] = splitColumnClauses(body)
	}

	return tables
}

// balancedParenBody returns the content between the '(' at openIdx
// (inclusive) and its matching ')', exclusive of both delimiters.
func balancedParenBody(s string, openIdx int) (string, bool) {
	if openIdx < 0 || openIdx >= len(s) || s[openIdx] != '(' {
		return "", false
	}
	depth := 0
	for i := openIdx; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[openIdx+1 : i], true
			}
		}
	}
	return "", false
}

// splitColumnClauses splits a CREATE TABLE body into its top-level
// comma-separated clauses (respecting nested parens), then returns the
// leading identifier of every clause that is a column definition
// rather than a table-level constraint.
func splitColumnClauses(body string) []string {
	var clauses []string
	depth := 0
	start := 0
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				clauses = append(clauses, body[start:i])
				start = i + 1
			}
		}
	}
	clauses = append(clauses, body[start:])

	var columns []string
	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}
		fields := strings.Fields(clause)
		if len(fields) == 0 {
			continue
		}
		first := strings.ToLower(strings.Trim(fields[0], `"`))
		if columnClauseKeywords[first] {
			continue
		}
		columns = append(columns, first)
	}
	return columns
}

// matchTenantColumn reports whether col is one of tenantColumns,
// case-insensitively, and returns the canonical (lowercase) column name.
func matchTenantColumn(col string) (string, bool) {
	lower := strings.ToLower(strings.Trim(col, `"`))
	for _, tc := range tenantColumns {
		if lower == tc {
			return tc, true
		}
	}
	return "", false
}

// sqlcQuery is one `-- name: X :cmd` block from a sqlc query file.
type sqlcQuery struct {
	file  string
	name  string
	table string // best-effort primary table this query touches, "" if none found
	body  string // the query's own SQL text (used for the tenant-column reference check)
}

// sqlcNameRe matches sqlc's own query-annotation comment,
// e.g. "-- name: InsertAuditLog :one".
var sqlcNameRe = regexp.MustCompile(`(?m)^--\s*name:\s*([A-Za-z0-9_]+)\s*:\S+\s*$`)

// queryTableRe finds the first FROM/INTO/UPDATE target in a query body.
// This is a lexical, not a SQL-parser-grade, extraction: sufficient for
// the single-table INSERT/SELECT/UPDATE/DELETE shapes every M0 query
// uses (api/internal/db/queries/*.sql has no joins or subqueries).
var queryTableRe = regexp.MustCompile(`(?i)\b(?:FROM|INTO|UPDATE)\s+"?([a-zA-Z_][a-zA-Z0-9_]*)"?`)

// scanQueriesDir walks every *.sql file in dir and returns one
// sqlcQuery per `-- name:` annotated block.
func scanQueriesDir(dir string) ([]sqlcQuery, error) {
	var queries []sqlcQuery

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path) //nolint:gosec // G304: dir/entry come from a fixed, repo-committed queries path (or a test-controlled fixture dir), never untrusted request input.
		if err != nil {
			return nil, err
		}

		queries = append(queries, splitQueries(path, string(data))...)
	}

	return queries, nil
}

// splitQueries splits one sqlc query file's contents into its
// individual `-- name:` blocks.
func splitQueries(file, content string) []sqlcQuery {
	names := sqlcNameRe.FindAllStringSubmatchIndex(content, -1)
	if len(names) == 0 {
		return nil
	}

	var queries []sqlcQuery
	for i, loc := range names {
		nameStart, nameEnd := loc[2], loc[3]
		bodyStart := loc[1]
		bodyEnd := len(content)
		if i+1 < len(names) {
			bodyEnd = names[i+1][0]
		}
		body := content[bodyStart:bodyEnd]

		queries = append(queries, sqlcQuery{
			file:  file,
			name:  content[nameStart:nameEnd],
			table: firstTable(body),
			body:  body,
		})
	}
	return queries
}

// firstTable returns the first FROM/INTO/UPDATE target found in body,
// or "" if none matches.
func firstTable(body string) string {
	m := queryTableRe.FindStringSubmatch(body)
	if m == nil {
		return ""
	}
	return m[1]
}

// containsWord reports whether word appears in s as a whole identifier
// (word-boundary match, case-insensitive) rather than as a substring of
// a longer identifier.
func containsWord(s, word string) bool {
	re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(word) + `\b`)
	return re.MatchString(s)
}
