package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver goose's providers need

	"github.com/jorgealonsodev/vecingest/internal/config"
	"github.com/jorgealonsodev/vecingest/internal/platform/migrate"
)

// runMigrate implements `vecingest migrate` (platform-bootstrap: Embedded
// Migration Runner With Advisory Lock). It shares its config requirement
// set with `serve --migrate` (D-I): BOOTSTRAP_DATABASE_URL (superuser)
// runs the bootstrap set, then a schema-set connection derived from that
// same DSN (migrate.SchemaDSN) runs the schema set as vecingest_owner,
// each under its own pinned advisory lock so concurrent api/worker boots
// never race. There is no separate MIGRATIONS_DATABASE_URL: vecingest_owner
// is provisioned NOLOGIN, so no DSN could ever authenticate as it
// directly -- the only reachable connection was always the superuser's,
// with the role assumed after connecting.
func runMigrate(ctx context.Context, _ []string, stdout io.Writer, lookup config.LookupEnv) error {
	_, holder, err := config.Load(ctx, lookup, config.CommandMigrate)
	if err != nil {
		return err
	}
	return runMigrationsWithHolder(ctx, stdout, holder)
}

type migrateHolder interface {
	BootstrapDatabaseURL() string
}

// runMigrationsWithHolder is the shared core `serve --migrate` also
// calls, so both entry points run the identical bootstrap-then-schema
// sequence against the identical two DSNs.
func runMigrationsWithHolder(ctx context.Context, stdout io.Writer, holder migrateHolder) error {
	bootstrapDSN := holder.BootstrapDatabaseURL()

	superuserDB, err := sql.Open("pgx", bootstrapDSN)
	if err != nil {
		return fmt.Errorf("migrate: open bootstrap connection: %w", err)
	}
	defer func() { _ = superuserDB.Close() }()

	schemaDSN, err := migrate.SchemaDSN(bootstrapDSN)
	if err != nil {
		return fmt.Errorf("migrate: derive schema connection: %w", err)
	}
	ownerDB, err := sql.Open("pgx", schemaDSN)
	if err != nil {
		return fmt.Errorf("migrate: open schema connection: %w", err)
	}
	defer func() { _ = ownerDB.Close() }()

	if err := migrate.Run(ctx, superuserDB, ownerDB); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	_, _ = fmt.Fprintln(stdout, "migrate: applied")
	return nil
}
