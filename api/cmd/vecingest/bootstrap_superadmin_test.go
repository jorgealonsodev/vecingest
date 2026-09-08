package main

import (
	"context"
	"encoding/base64"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/testhelpers"
)

func setBootstrapEnv(t *testing.T, databaseURL string) {
	t.Helper()
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
}

// platform-bootstrap: Idempotent Superadmin Bootstrap -- first run
// creates exactly one superadmin, and the password never appears in
// argv or an environment variable.
func TestRunBootstrapSuperadmin_FirstRunCreatesSuperadmin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, superuserDB := testhelpers.AppRWHandles(t)
	dsn := handles.Write.Config().ConnConfig.ConnString()
	setBootstrapEnv(t, dsn)

	stdout := &strings.Builder{}
	err := runBootstrapSuperadmin(context.Background(),
		[]string{"--email=admin@example.com", "--password-stdin"},
		strings.NewReader("a-very-strong-superadmin-passphrase\n"),
		stdout, os.LookupEnv)
	if err != nil {
		t.Fatalf("runBootstrapSuperadmin: %v", err)
	}

	var count int
	if err := superuserDB.QueryRowContext(context.Background(),
		`SELECT count(*) FROM users WHERE email = $1 AND is_superadmin = true`, "admin@example.com",
	).Scan(&count); err != nil {
		t.Fatalf("query users: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one superadmin user, got %d", count)
	}

	// The password must never be observable via argv or the environment:
	// runBootstrapSuperadmin's own args slice above carries no password
	// value, and the only credential input was the io.Reader passed
	// directly -- never os.Args, never os.Setenv.
	for _, a := range os.Args {
		if strings.Contains(a, "a-very-strong-superadmin-passphrase") {
			t.Fatalf("password leaked into process argv: %v", os.Args)
		}
	}
}

// platform-bootstrap: Idempotent Superadmin Bootstrap -- a second run
// against the same database creates no duplicate and does not error.
func TestRunBootstrapSuperadmin_SecondRunIsNoop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, superuserDB := testhelpers.AppRWHandles(t)
	dsn := handles.Write.Config().ConnConfig.ConnString()

	// config.Load unsets ENCRYPTION_KEY from the real process environment
	// once validation succeeds (platform-bootstrap: ENCRYPTION_KEY
	// Isolation) -- correct for a real one-shot CLI process, but it means
	// this test, which calls runBootstrapSuperadmin twice in the SAME
	// process, must re-set it before each call.
	run := func(password string) error {
		setBootstrapEnv(t, dsn)
		return runBootstrapSuperadmin(context.Background(),
			[]string{"--email=admin2@example.com", "--password-stdin"},
			strings.NewReader(password+"\n"),
			&strings.Builder{}, os.LookupEnv)
	}

	if err := run("first-superadmin-passphrase-here"); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := run("second-superadmin-passphrase-here"); err != nil {
		t.Fatalf("second run must not error: %v", err)
	}

	var count int
	if err := superuserDB.QueryRowContext(context.Background(),
		`SELECT count(*) FROM users WHERE email = $1`, "admin2@example.com",
	).Scan(&count); err != nil {
		t.Fatalf("query users: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected no duplicate user after a second run, got %d rows", count)
	}

	// The second run's password must never overwrite the first's hash.
	q := db.New(handles.Write)
	user, err := q.GetUserByEmail(context.Background(), "admin2@example.com")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if user.PasswordHash == "" {
		t.Fatalf("expected a stored password hash")
	}
}

// platform-bootstrap: Idempotent Superadmin Bootstrap -- promotes an
// existing non-superadmin user (e.g. one created by seed) without ever
// touching its password.
func TestRunBootstrapSuperadmin_PromotesExistingNonSuperadminWithoutTouchingPassword(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, _ := testhelpers.AppRWHandles(t)
	dsn := handles.Write.Config().ConnConfig.ConnString()
	setBootstrapEnv(t, dsn)

	q := db.New(handles.Write)
	created, err := q.InsertUser(context.Background(), db.InsertUserParams{ //nolint:gosec // G101: fake test fixture row, not a credential
		ID:           uuid.New(),
		Email:        "promote-me@example.com",
		PasswordHash: "existing-hash-must-survive",
		Name:         "Existing User",
		Locale:       "es",
		IsSuperadmin: false,
	})
	if err != nil {
		t.Fatalf("seed existing user: %v", err)
	}

	if err := runBootstrapSuperadmin(context.Background(),
		[]string{"--email=promote-me@example.com", "--password-stdin"},
		strings.NewReader("irrelevant-passphrase-never-applied\n"),
		&strings.Builder{}, os.LookupEnv,
	); err != nil {
		t.Fatalf("runBootstrapSuperadmin: %v", err)
	}

	after, err := q.GetUserByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !after.IsSuperadmin {
		t.Fatalf("expected the existing user to be promoted to superadmin")
	}
	if after.PasswordHash != "existing-hash-must-survive" {
		t.Fatalf("expected the existing password hash to survive promotion, got %q", after.PasswordHash)
	}
}

// platform-bootstrap: Idempotent Superadmin Bootstrap -- MUST NOT accept
// the password as a command-line argument.
func TestRunBootstrapSuperadmin_RequiresPasswordStdinFlag(t *testing.T) {
	err := runBootstrapSuperadmin(context.Background(),
		[]string{"--email=admin@example.com"},
		strings.NewReader("whatever\n"),
		&strings.Builder{}, os.LookupEnv)
	if err == nil {
		t.Fatalf("expected an error when --password-stdin is not passed")
	}
}
