// Package mail is a log-only stand-in for internal/mail's real,
// SMTP-backed bounded-async-dispatch mailer (design D-N/D-S). It never
// returns an error and never touches the network, so it exists for two
// deliberate uses: (1) tests that want to assert dispatch/log behaviour
// without a real SMTP server, and (2) a deployment that explicitly
// chooses to run with no email delivery at all. cmd/vecingest/serve.go
// wires the real internal/mail.AsyncMailer unconditionally; nothing in
// this codebase falls back to LogMailer automatically, so getting one
// in production requires deliberately constructing it in source, not
// an accidental default.
package mail

import (
	"context"
	"log/slog"
)

// LogMailer logs every message it would have sent instead of sending
// it. It never returns an error: a stand-in mailer failing would be
// worse than a stand-in mailer that silently no-ops, since neither
// lockout.Service nor password.ForgotPasswordService treat a Mailer
// error as anything other than best-effort already (both dispatch it
// off their own call path precisely so it can never become a timing
// oracle or a user-facing failure).
type LogMailer struct {
	Logger *slog.Logger
}

func (m LogMailer) logger() *slog.Logger {
	if m.Logger != nil {
		return m.Logger
	}
	return slog.Default()
}

// SendRaw logs the subject/body length only -- never the recipient
// address, which is PII (D-J applies to this stand-in too). Both
// lockout.Mailer and password's email-sending seam are satisfied by a
// thin, package-local adapter calling this method, since each declares
// its own Message-shaped interface.
func (m LogMailer) SendRaw(ctx context.Context, _ /* to, never logged */ string, subject, body string) error {
	m.logger().InfoContext(ctx, "mail: would send (LogMailer stand-in, no real SMTP sender wired yet)",
		"subject", subject, "body_len", len(body))
	return nil
}
