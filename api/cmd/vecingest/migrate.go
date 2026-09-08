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
// runs the bootstrap set, then MIGRATIONS_DATABASE_URL (vecingest_owner)
// runs the schema set, each under its own pinned advisory lock so
// concurrent api/worker boots never race.
func runMigrate(ctx context.Context, _ []string, stdout io.Writer, lookup config.LookupEnv) error {
	_, holder, err := config.Load(ctx, lookup, config.CommandMigrate)
	if err != nil {
		return err
	}
	return runMigrationsWithHolder(ctx, stdout, holder)
}

type migrateHolder interface {
	BootstrapDatabaseURL() string
	MigrationsDatabaseURL() string
}

// runMigrationsWithHolder is the shared core `serve --migrate` also
// calls, so both entry points run the identical bootstrap-then-schema
// sequence against the identical two DSNs.
func runMigrationsWithHolder(ctx context.Context, stdout io.Writer, holder migrateHolder) error {
	superuserDB, err := sql.Open("pgx", holder.BootstrapDatabaseURL())
	if err != nil {
		return fmt.Errorf("migrate: open bootstrap connection: %w", err)
	}
	defer func() { _ = superuserDB.Close() }()

	ownerDB, err := sql.Open("pgx", holder.MigrationsDatabaseURL())
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
