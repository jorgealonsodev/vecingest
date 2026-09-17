// invitations.go wires the FIRST real River job producer/worker this
// project has (design's Interfaces/Contracts table: "Queue -- yes --
// first producers in the project"). Everything M0 built in queue.go
// (NewClient, noopWorker) stays a legitimate "zero real job producers"
// deployment shape for a build that does not wire a RawSender; this
// file only ADDS a registered worker/job kind, it never changes that
// baseline behaviour.
package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
	"github.com/jorgealonsodev/vecingest/internal/domain/invitations"
	"github.com/jorgealonsodev/vecingest/internal/mail"
)

// invitationEmailJobArgs adapts invitations.EmailArgs to river's JobArgs
// contract (Kind() string). It is unexported and package-local so no
// caller outside this package needs to know river's own JobArgs shape
// exists (design: "No M1 handler or domain service imports ... river").
//
// It deliberately does NOT mirror invitations.EmailArgs field for
// field: this shape is a DURABLE river_job row, so the short code
// crosses it sealed, never in plaintext (review lineage
// review-e72754dc7521b57a).
type invitationEmailJobArgs struct {
	InvitationID uuid.UUID `json:"invitation_id"`
	Email        string    `json:"email"`
	// ShortCodeEncrypted is the one-time-visible short code sealed with
	// AES-256-GCM under ENCRYPTION_KEY -- mfa.EncryptSecret, the same
	// primitive (and the same key) user_mfa.totp_secret_encrypted
	// already uses for TOTP secrets at rest (D-I/D-P), reused rather
	// than reinvented. Only the worker opens it, at send time.
	ShortCodeEncrypted []byte `json:"short_code_encrypted"`
}

func (invitationEmailJobArgs) Kind() string { return "invitation_email" }

// RawSender is the minimal shape internal/mail.AsyncMailer and
// internal/platform/mail.LogMailer both already satisfy, mirroring
// internal/http/handlers.RawSender's identical structural interface --
// declared separately here so this package needs no dependency on the
// HTTP layer.
type RawSender interface {
	SendRaw(ctx context.Context, to, subject, body string) error
}

// RiverInvitationQueue adapts a *river.Client[pgx.Tx] to the
// invitations.Queue port (design D interfaces table, phase A). Key is
// ENCRYPTION_KEY: the seal applied at this exact boundary, where an
// in-memory port value becomes a durable row.
type RiverInvitationQueue struct {
	Client *river.Client[pgx.Tx]
	Key    [32]byte
}

// EnqueueInvitationEmail inserts the invitation-email job on tx (design
// D-6: "river.InsertTx for the invitation email job inside the creation
// transaction") -- if tx rolls back, the job is never worked (River's
// own snapshot-visibility guarantee across transactions). The short
// code is sealed first: the job row outlives the request, so it must
// never hold a usable invitation credential.
func (q RiverInvitationQueue) EnqueueInvitationEmail(ctx context.Context, tx pgx.Tx, args invitations.EmailArgs) error {
	sealed, err := mfa.EncryptSecret(q.Key, []byte(args.ShortCode))
	if err != nil {
		return fmt.Errorf("queue: seal invitation short code: %w", err)
	}
	jobArgs := invitationEmailJobArgs{
		InvitationID:       args.InvitationID,
		Email:              args.Email,
		ShortCodeEncrypted: sealed,
	}
	if _, err := q.Client.InsertTx(ctx, tx, jobArgs, nil); err != nil {
		return fmt.Errorf("queue: enqueue invitation email: %w", err)
	}
	return nil
}

// InvitationEmailWorker consumes the invitation_email job kind, rendering
// and dispatching the actual email through Sender. A nil Sender makes
// Work a no-op rather than an error: a deployment that deliberately runs
// with no mail sender configured (M1 does not depend on production SMTP
// -- the paper short-code path works without a mail server) must not
// fail the job and retry forever.
type InvitationEmailWorker struct {
	river.WorkerDefaults[invitationEmailJobArgs]
	Sender RawSender
	// Key is ENCRYPTION_KEY, the counterpart of the seal
	// RiverInvitationQueue applies: the plaintext short code exists
	// only between this Open and the rendered message.
	Key [32]byte
}

func (w *InvitationEmailWorker) Work(ctx context.Context, job *river.Job[invitationEmailJobArgs]) error {
	if w.Sender == nil {
		return nil
	}
	shortCode, err := mfa.DecryptSecret(w.Key, job.Args.ShortCodeEncrypted)
	if err != nil {
		return fmt.Errorf("invitation email worker: open short code: %w", err)
	}
	subject, body, err := mail.RenderInvitation(mail.InvitationData{ShortCode: string(shortCode)})
	if err != nil {
		return fmt.Errorf("invitation email worker: render: %w", err)
	}
	if err := w.Sender.SendRaw(ctx, job.Args.Email, subject, body); err != nil {
		return fmt.Errorf("invitation email worker: send: %w", err)
	}
	return nil
}

// invitationsExpireArgs is the daily sweep job's (empty) payload (design
// D-6; PRD §7.4 job table: "invitations.expire | diario | Marca
// invitaciones caducadas"; task 6.18/6.19).
type invitationsExpireArgs struct{}

func (invitationsExpireArgs) Kind() string { return "invitations_expire" }

// invitationsExpireInterval is the job table's own "diario" (daily)
// cadence.
const invitationsExpireInterval = 24 * time.Hour

// invitationsExpireWorker runs db.Queries.SweepExpiredInvitations against
// pool directly -- this job has no per-tenant scope by design (it is a
// maintenance pass over every community, exactly like
// cmd/lintscope's documented audit_log/invitations.sql exceptions), so
// it needs no handlers.Deps or HTTP request context at all.
type invitationsExpireWorker struct {
	river.WorkerDefaults[invitationsExpireArgs]
	pool *pgxpool.Pool
}

func (w *invitationsExpireWorker) Work(ctx context.Context, _ *river.Job[invitationsExpireArgs]) error {
	q := db.New(w.pool)
	if _, err := q.SweepExpiredInvitations(ctx); err != nil {
		return fmt.Errorf("invitations expire worker: sweep: %w", err)
	}
	return nil
}
