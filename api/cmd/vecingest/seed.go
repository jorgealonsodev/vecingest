package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/config"
	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/audit"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/password"
)

// envAppEnvName is the PRD's own environment-domain variable name (never
// NODE_ENV). Duplicated here as a literal rather than imported from
// internal/config, which keeps it unexported there: this is an external
// contract name, not an internal implementation detail leaking out.
const envAppEnvName = "APP_ENV"

// seedFixturePassword is deliberately short, well-known, and documented
// (design D-S): running it through PasswordPolicy/HIBPChecker would
// either fail the length floor or require network access in CI. Argon2id
// hashing still applies below, so the stored shape is identical to a
// real account.
const seedFixturePassword = "vecingest-dev-2024" //nolint:gosec // G101: deliberately public, documented dev-only fixture password (design D-S), never used outside seed's own non-production guard

type seedFixture struct {
	email        string
	name         string
	isSuperadmin bool
}

// seedFixtures is M0's degenerate fixture set (design D-S): despachos,
// communities, viviendas and companies are M1 entities with no M0
// tables, so at M0 seed can only create what M0 has -- users. One is
// flagged is_superadmin, the rest are plain.
var seedFixtures = []seedFixture{
	{email: "superadmin@vecingest.dev", name: "Dev Superadmin", isSuperadmin: true},
	{email: "manager@vecingest.dev", name: "Dev Manager", isSuperadmin: false},
	{email: "resident@vecingest.dev", name: "Dev Resident", isSuperadmin: false},
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// runSeed implements `vecingest seed` (platform-bootstrap: seed Refuses
// To Run Outside Non-Production). The APP_ENV guard is the very first
// thing this function does, before config.Load or any database
// connection: an unset or unrecognised APP_ENV is refused exactly like
// "production" -- a deliberate fail-closed divergence from the PRD's
// literal `!= production` check (PRD §8.2 defaults APP_ENV to
// "production", so an absent variable means production in practice).
func runSeed(ctx context.Context, _ []string, stdout io.Writer, lookup config.LookupEnv) error {
	appEnv, _ := lookup(envAppEnvName)
	switch config.AppEnv(appEnv) {
	case config.AppEnvDevelopment, config.AppEnvStaging:
		// proceed
	default:
		return fmt.Errorf("seed: refuses to run outside development/staging (APP_ENV=%q); production, unset, and unrecognised values are all fail-closed refused (platform-bootstrap: seed Refuses To Run Outside Non-Production)", appEnv)
	}

	_, holder, err := config.Load(ctx, lookup, config.CommandSeed)
	if err != nil {
		return err
	}

	writeDB, err := db.NewWorkerHandle(ctx, holder.DatabaseURL())
	if err != nil {
		return fmt.Errorf("seed: connect to database: %w", err)
	}
	defer writeDB.Close()

	hash, err := password.Hash(seedFixturePassword)
	if err != nil {
		return fmt.Errorf("seed: hash fixture password: %w", err)
	}

	created := 0
	for _, fx := range seedFixtures {
		wasCreated, err := seedOneUser(ctx, writeDB, fx, hash)
		if err != nil {
			return fmt.Errorf("seed: create fixture user %s: %w", fx.email, err)
		}
		if wasCreated {
			created++
		}
	}

	_, _ = fmt.Fprintf(stdout, "seed: fixture set applied (%d new user(s) of %d)\n", created, len(seedFixtures))
	return nil
}

// seedOneUser inserts fx idempotently (ON CONFLICT (email) DO NOTHING,
// never touching an existing row's password) and, only for a genuinely
// new row, writes a single audit_log entry through audit.Append in the
// SAME transaction -- so the chain has no gap after seeding and a
// second run writes no new audit rows for users that already existed.
func seedOneUser(ctx context.Context, writeDB db.WriteDB, fx seedFixture, hash string) (created bool, err error) {
	tx, err := writeDB.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := db.New(tx)
	userID := uuid.New()
	affected, err := q.InsertUserIgnoreConflict(ctx, db.InsertUserIgnoreConflictParams{
		ID:           userID,
		Email:        fx.email,
		PasswordHash: hash,
		Name:         fx.name,
		Locale:       "es",
		IsSuperadmin: fx.isSuperadmin,
	})
	if err != nil {
		return false, fmt.Errorf("insert user: %w", err)
	}
	if affected == 0 {
		return false, nil
	}

	if _, err := audit.Append(ctx, tx, systemClock{}, audit.Entry{
		UserID:   &userID,
		Action:   "seed.user_created",
		Entity:   "user",
		EntityID: &userID,
	}); err != nil {
		return false, fmt.Errorf("audit append: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return true, nil
}
