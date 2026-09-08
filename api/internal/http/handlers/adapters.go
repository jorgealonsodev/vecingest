package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/lockout"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
	"github.com/jorgealonsodev/vecingest/internal/mail"
)

// resetTokenLifetime is the requirement's 1-hour password-reset-token
// expiry (auth-credentials §5.1; task 5.21).
const resetTokenLifetime = time.Hour

// RawSender is the minimal shape both internal/platform/mail.LogMailer
// (the interim, log-only stand-in used by tests and by a deployment
// that deliberately has no SMTP) and internal/mail.AsyncMailer (the
// real SMTP sender) satisfy: send a subject/body, never logging the
// recipient (D-J).
type RawSender interface {
	SendRaw(ctx context.Context, to, subject, body string) error
}

// LockoutMailer adapts a RawSender to lockout.Mailer. The domain layer
// (lockout.go) remains the single source of truth for what the alert
// says (msg.Subject, msg.Body); this adapter only wraps msg.Body in the
// branded login_lockout.html shell (design.md:473, D-N "the alert")
// before handing it to the transport.
type LockoutMailer struct{ Sender RawSender }

func (m LockoutMailer) Send(ctx context.Context, msg lockout.Message) error {
	body, err := mail.RenderLoginLockoutAlert(mail.LoginLockoutData{Message: msg.Body})
	if err != nil {
		return fmt.Errorf("lockout mailer: %w", err)
	}
	return m.Sender.SendRaw(ctx, msg.To, msg.Subject, body)
}

// OpaqueTokenIssuer adapts token.GenerateOpaqueToken to
// password.TokenIssuer.
type OpaqueTokenIssuer struct{}

func (OpaqueTokenIssuer) GenerateOpaqueToken() (string, []byte, error) {
	return token.GenerateOpaqueToken()
}

// DBResetRequester adapts a WriteDB + RawSender pair to
// password.ResetRequester: it persists the reset-token row (1-hour
// expiry, D-D's hashed-storage rule) and dispatches the email through
// Sender. ForgotPasswordService already calls this off its own request
// path, so its DB write and SMTP send never delay the response.
type DBResetRequester struct {
	DB     db.WriteDB
	Sender RawSender
	Clock  Clock
}

func (r DBResetRequester) clock() Clock {
	if r.Clock != nil {
		return r.Clock
	}
	return systemClock{}
}

func (r DBResetRequester) RequestReset(ctx context.Context, userID uuid.UUID, email, rawToken string, tokenHash []byte) error {
	q := db.New(r.DB)
	if _, err := q.InsertPasswordResetToken(ctx, db.InsertPasswordResetTokenParams{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: r.clock().Now().Add(resetTokenLifetime),
	}); err != nil {
		return err
	}
	subject, body, err := mail.RenderPasswordReset(mail.PasswordResetData{RawToken: rawToken})
	if err != nil {
		return fmt.Errorf("password reset requester: %w", err)
	}
	return r.Sender.SendRaw(ctx, email, subject, body)
}
