package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/config"
	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/password"
	"github.com/jorgealonsodev/vecingest/internal/platform/hibp"
)

// defaultSuperadminName fills the NOT NULL users.name column: a
// superadmin bootstrapped from the command line has no operator-supplied
// display name, and this is a stable, documented placeholder rather than
// an invented one -- an operator can rename the account later through
// the ordinary profile-update path once one exists.
const defaultSuperadminName = "Superadmin"

var errPasswordStdinRequired = errors.New("bootstrap-superadmin: --password-stdin is required; the password is never accepted as a command-line argument or read from an environment variable (platform-bootstrap: Idempotent Superadmin Bootstrap)")

// runBootstrapSuperadmin implements `vecingest bootstrap-superadmin`
// (platform-bootstrap: Idempotent Superadmin Bootstrap). The password is
// read exclusively from stdin: it never appears in args (no --password
// flag exists at all) and is never read from an environment variable.
func runBootstrapSuperadmin(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer, lookup config.LookupEnv) error {
	fs := flag.NewFlagSet("bootstrap-superadmin", flag.ContinueOnError)
	email := fs.String("email", "", "superadmin email address")
	passwordStdin := fs.Bool("password-stdin", false, "read the password from stdin (required; the password is never accepted any other way)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*passwordStdin {
		return errPasswordStdinRequired
	}
	if *email == "" {
		return errors.New("bootstrap-superadmin: --email is required")
	}

	rawPassword, err := readPasswordLine(stdin)
	if err != nil {
		return fmt.Errorf("bootstrap-superadmin: read password from stdin: %w", err)
	}

	_, holder, err := config.Load(ctx, lookup, config.CommandBootstrapSuperadmin)
	if err != nil {
		return err
	}

	if err := (password.PasswordPolicy{HIBP: hibp.NewClient()}).Validate(ctx, rawPassword, false); err != nil {
		return fmt.Errorf("bootstrap-superadmin: password rejected: %w", err)
	}

	hash, err := password.Hash(rawPassword)
	if err != nil {
		return fmt.Errorf("bootstrap-superadmin: hash password: %w", err)
	}

	// A short-lived, single-purpose connection: this CLI command runs
	// once and exits, so it needs neither the pooled-serve nor the
	// direct-worker handle's own rationale, only a WriteDB it can close
	// when done.
	writeDB, err := db.NewWorkerHandle(ctx, holder.DatabaseURL())
	if err != nil {
		return fmt.Errorf("bootstrap-superadmin: connect to database: %w", err)
	}
	defer writeDB.Close()

	q := db.New(writeDB)
	affected, err := q.InsertUserIgnoreConflict(ctx, db.InsertUserIgnoreConflictParams{
		ID:           uuid.New(),
		Email:        *email,
		PasswordHash: hash,
		Name:         defaultSuperadminName,
		Locale:       "es",
		IsSuperadmin: true,
	})
	if err != nil {
		return fmt.Errorf("bootstrap-superadmin: insert user: %w", err)
	}

	if affected == 0 {
		existing, err := q.GetUserByEmail(ctx, *email)
		if err != nil {
			return fmt.Errorf("bootstrap-superadmin: load existing user: %w", err)
		}
		if !existing.IsSuperadmin {
			if err := q.EnsureSuperadmin(ctx, existing.ID); err != nil {
				return fmt.Errorf("bootstrap-superadmin: promote existing user: %w", err)
			}
		}
		_, _ = fmt.Fprintln(stdout, "bootstrap-superadmin: already exists, no change")
		return nil
	}

	_, _ = fmt.Fprintln(stdout, "bootstrap-superadmin: created")
	return nil
}

// readPasswordLine reads exactly one line from stdin, trimming the
// trailing newline. It deliberately does not trim leading/trailing
// spaces beyond the newline itself: a password may legitimately contain
// spaces.
func readPasswordLine(stdin io.Reader) (string, error) {
	reader := bufio.NewReader(stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return "", errors.New("empty password read from stdin")
	}
	return line, nil
}
