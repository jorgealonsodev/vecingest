package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
	"github.com/jorgealonsodev/vecingest/internal/mail"
)

// mfaEnrollEmailJobArgs is the durable river_job shape of
// mfa.EnrollEmail. The code crosses it sealed, for the same reason the
// invitation short code does (review lineage review-e72754dc7521b57a):
// the row outlives the request, and a plaintext code in it would let
// anyone with SELECT on river_job confirm someone else's enrollment.
type mfaEnrollEmailJobArgs struct {
	UserID        uuid.UUID `json:"user_id"`
	ChallengeID   uuid.UUID `json:"challenge_id"`
	Email         string    `json:"email"`
	CodeEncrypted []byte    `json:"code_encrypted"`
}

func (mfaEnrollEmailJobArgs) Kind() string { return "mfa_enroll_email" }

// RiverMFAEnrollQueue adapts a *river.Client[pgx.Tx] to
// mfa.EnrollEmailQueue. Key is ENCRYPTION_KEY.
type RiverMFAEnrollQueue struct {
	Client *river.Client[pgx.Tx]
	Key    [32]byte
}

// EnqueueMFAEnrollEmail seals the code and inserts the job on tx, the
// transaction that also wrote the pending secret and the challenge: if
// it rolls back, no email goes out for a challenge that does not exist.
func (q RiverMFAEnrollQueue) EnqueueMFAEnrollEmail(ctx context.Context, tx pgx.Tx, args mfa.EnrollEmail) error {
	sealed, err := mfa.EncryptSecret(q.Key, []byte(args.Code))
	if err != nil {
		return fmt.Errorf("queue: seal mfa enrollment code: %w", err)
	}
	jobArgs := mfaEnrollEmailJobArgs{UserID: args.UserID, ChallengeID: args.ChallengeID, Email: args.Email, CodeEncrypted: sealed}
	if _, err := q.Client.InsertTx(ctx, tx, jobArgs, nil); err != nil {
		return fmt.Errorf("queue: enqueue mfa enrollment email: %w", err)
	}
	return nil
}

// MFAEnrollEmailWorker delivers the enrollment code. Like
// InvitationEmailWorker, it returns SendRaw's error verbatim so the job
// completes only on delivery (see RawSender's contract), and a nil
// Sender no-ops instead of retrying forever.
//
// A nil Sender here has a sharper consequence than for invitations,
// stated so nobody mistakes it for harmless: with no mail delivered,
// nobody on that deployment can complete a TOTP enrollment. That is the
// intended failure direction -- closed, not open.
//
// A job can run long after it was enqueued: River retries a failed send
// with backoff. Before sending, the worker re-reads the challenge and
// completes WITHOUT sending when it is no longer open (superseded by a
// re-enrollment, expired, verified, or gone), so the user is never
// mailed a code that cannot confirm anything. A failed lookup fails the
// job instead: it is not proof the code is dead.
type MFAEnrollEmailWorker struct {
	river.WorkerDefaults[mfaEnrollEmailJobArgs]
	Sender     RawSender
	Key        [32]byte
	Challenges challengeReader
}

// challengeReader is the otp_challenges read the worker needs;
// *db.Queries satisfies it.
type challengeReader interface {
	GetOTPChallenge(ctx context.Context, id uuid.UUID) (db.OtpChallenge, error)
}

func (w *MFAEnrollEmailWorker) Work(ctx context.Context, job *river.Job[mfaEnrollEmailJobArgs]) error {
	if w.Sender == nil {
		return nil
	}
	challenge, err := w.Challenges.GetOTPChallenge(ctx, job.Args.ChallengeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("mfa enroll email worker: read challenge: %w", err)
	}
	if challenge.VerifiedAt.Valid || !challenge.ExpiresAt.After(time.Now()) {
		return nil
	}

	code, err := mfa.DecryptSecret(w.Key, job.Args.CodeEncrypted)
	if err != nil {
		return fmt.Errorf("mfa enroll email worker: open code: %w", err)
	}
	subject, body, err := mail.RenderMFAEnrollCode(mail.MFAEnrollCodeData{
		Code: string(code), ExpiresInMinutes: int(mfa.EnrollEmailTTL / time.Minute),
	})
	if err != nil {
		return fmt.Errorf("mfa enroll email worker: render: %w", err)
	}
	if err := w.Sender.SendRaw(ctx, job.Args.Email, subject, body); err != nil {
		return fmt.Errorf("mfa enroll email worker: send: %w", err)
	}
	return nil
}
