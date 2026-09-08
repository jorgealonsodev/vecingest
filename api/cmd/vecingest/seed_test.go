package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/testhelpers"
)

// platform-bootstrap: seed Refuses To Run Outside Non-Production --
// refused with APP_ENV=production, exits non-zero before opening a
// database connection.
func TestRunSeed_RefusedWithAppEnvProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://poisoned:host-never-dialed@127.0.0.1:1/vecingest")

	if err := runSeed(context.Background(), nil, &strings.Builder{}, os.LookupEnv); err == nil {
		t.Fatalf("expected seed to refuse with APP_ENV=production")
	}
}

// platform-bootstrap: seed Refuses To Run Outside Non-Production --
// refused with APP_ENV unset (fail-closed divergence from the PRD's
// literal != production).
func TestRunSeed_RefusedWithAppEnvUnset(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://poisoned:host-never-dialed@127.0.0.1:1/vecingest")
	// Deliberately do not set APP_ENV; os.LookupEnv must report absent.
	if _, ok := os.LookupEnv("APP_ENV"); ok {
		t.Skip("APP_ENV is set in this process's real environment; cannot exercise the unset case safely")
	}

	if err := runSeed(context.Background(), nil, &strings.Builder{}, os.LookupEnv); err == nil {
		t.Fatalf("expected seed to refuse with APP_ENV unset")
	}
}

// platform-bootstrap: seed Refuses To Run Outside Non-Production -- an
// unrecognised APP_ENV value is also refused (fail-closed).
func TestRunSeed_RefusedWithUnrecognisedAppEnv(t *testing.T) {
	t.Setenv("APP_ENV", "not-a-real-environment")
	t.Setenv("DATABASE_URL", "postgres://poisoned:host-never-dialed@127.0.0.1:1/vecingest")

	if err := runSeed(context.Background(), nil, &strings.Builder{}, os.LookupEnv); err == nil {
		t.Fatalf("expected seed to refuse with an unrecognised APP_ENV value")
	}
}

// platform-bootstrap: seed Refuses To Run Outside Non-Production --
// proceeds and creates the M0 fixture set with APP_ENV=development; a
// second run changes nothing (idempotent).
func TestRunSeed_CreatesFixturesWithDevelopmentAndIsIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, superuserDB := testhelpers.AppRWHandles(t)
	dsn := handles.Write.Config().ConnConfig.ConnString()

	runOnce := func() error {
		t.Setenv("APP_ENV", "development")
		t.Setenv("DATABASE_URL", dsn)
		return runSeed(context.Background(), nil, &strings.Builder{}, os.LookupEnv)
	}

	if err := runOnce(); err != nil {
		t.Fatalf("first seed run: %v", err)
	}

	var userCount int
	if err := superuserDB.QueryRowContext(context.Background(), `SELECT count(*) FROM users`).Scan(&userCount); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount == 0 {
		t.Fatalf("expected the seed fixture set to create at least one user")
	}

	var superadminCount int
	if err := superuserDB.QueryRowContext(context.Background(), `SELECT count(*) FROM users WHERE is_superadmin = true`).Scan(&superadminCount); err != nil {
		t.Fatalf("count superadmin users: %v", err)
	}
	if superadminCount != 1 {
		t.Fatalf("expected exactly one seeded superadmin, got %d", superadminCount)
	}

	var auditCount int
	if err := superuserDB.QueryRowContext(context.Background(), `SELECT count(*) FROM audit_log`).Scan(&auditCount); err != nil {
		t.Fatalf("count audit_log: %v", err)
	}
	if auditCount != userCount {
		t.Fatalf("expected one audit_log entry per seeded user (%d), got %d", userCount, auditCount)
	}

	if err := runOnce(); err != nil {
		t.Fatalf("second seed run: %v", err)
	}

	var userCountAfter int
	if err := superuserDB.QueryRowContext(context.Background(), `SELECT count(*) FROM users`).Scan(&userCountAfter); err != nil {
		t.Fatalf("count users after second run: %v", err)
	}
	if userCountAfter != userCount {
		t.Fatalf("expected a second seed run to create no new users: had %d, now %d", userCount, userCountAfter)
	}

	var auditCountAfter int
	if err := superuserDB.QueryRowContext(context.Background(), `SELECT count(*) FROM audit_log`).Scan(&auditCountAfter); err != nil {
		t.Fatalf("count audit_log after second run: %v", err)
	}
	if auditCountAfter != auditCount {
		t.Fatalf("expected a second seed run to write no new audit_log rows: had %d, now %d", auditCount, auditCountAfter)
	}

	// seed bypasses the password policy/HIBP entirely, but Argon2id
	// hashing must still apply -- the stored shape is identical to a real
	// account (design D-S).
	q := db.New(handles.Write)
	rows, err := superuserDB.QueryContext(context.Background(), `SELECT email FROM users LIMIT 1`)
	if err != nil {
		t.Fatalf("query one seeded email: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var email string
	if rows.Next() {
		if err := rows.Scan(&email); err != nil {
			t.Fatalf("scan email: %v", err)
		}
	}
	user, err := q.GetUserByEmail(context.Background(), email)
	if err != nil {
		t.Fatalf("get seeded user: %v", err)
	}
	if !strings.HasPrefix(user.PasswordHash, "$argon2id$") {
		t.Fatalf("expected a real Argon2id PHC string, got %q", user.PasswordHash)
	}
}
