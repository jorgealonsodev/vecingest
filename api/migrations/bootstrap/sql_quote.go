package bootstrap

import (
	"strings"

	"github.com/jackc/pgx/v5"
)

// quoteIdent safely quotes a Postgres identifier (role, schema, function
// name) for interpolation into DDL, which cannot be parameterized the
// way values can.
func quoteIdent(name string) string {
	return pgx.Identifier{name}.Sanitize()
}

// quoteLiteral safely quotes a Postgres string literal for interpolation
// into DDL that has no other way to bind a value (e.g. CREATE ROLE ...
// PASSWORD).
func quoteLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
