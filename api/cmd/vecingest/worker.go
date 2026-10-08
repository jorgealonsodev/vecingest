package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/jorgealonsodev/vecingest/internal/config"
	"github.com/jorgealonsodev/vecingest/internal/config/secrets"
	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/mail"
	"github.com/jorgealonsodev/vecingest/internal/platform/queue"
)

// workerShutdownGrace bounds client.Stop below the compose
// stop_grace_period: 30s (design D-S / PRD §8.1) with margin for the
// pool close that follows it, so a hung shutdown still lets the process
// exit before Docker sends SIGKILL.
const workerShutdownGrace = 20 * time.Second

// runWorker implements `vecingest worker` (platform-bootstrap: CLI
// Subcommands; design D-S "a real deliverable with zero jobs"). It
// starts nothing but WorkerDB and a River client with leader election
// (no HTTP listener, no mail pool, no rate limiter -- design D-S), then
// blocks until ctx is cancelled (main.go installs the real SIGTERM
// handler), and shuts down gracefully within workerShutdownGrace.
func runWorker(ctx context.Context, _ []string, stdout io.Writer, lookup config.LookupEnv) error {
	cfg, holder, err := config.Load(ctx, lookup, config.CommandWorker)
	if err != nil {
		return err
	}

	workerDB, err := db.NewWorkerHandle(ctx, holder.DatabaseURLWorker())
	if err != nil {
		return fmt.Errorf("worker: connect to database: %w", err)
	}
	defer workerDB.Close()

	// The REAL SMTP sender, never a log sink. This process is the only
	// consumer of the invitation_email job kind (serve builds a
	// producer-only River client it never Start()s), so whatever is
	// wired here decides whether M1's central feature works at all.
	// It used to be platmail.LogMailer: every job decrypted the short
	// code, rendered the message, wrote it to a log and returned nil,
	// so the row was recorded COMPLETED, never retried, never
	// dead-lettered -- an invitation that was never delivered was
	// indistinguishable from one that was (review lineage
	// review-c4efc3f92d076299). SMTP_URL and MAIL_FROM are now declared
	// requirements of CommandWorker for the same reason
	// TURNSTILE_SECRET became one: a missing credential must fail at
	// boot, not silently at every send.
	//
	// ENCRYPTION_KEY is a worker requirement too (review lineage
	// review-e72754dc7521b57a): the job payload carries its short code
	// SEALED, so the worker that renders the email is the only place it
	// is opened again.
	var encryptionKey [32]byte
	copy(encryptionKey[:], holder.EncryptionKey())

	sender, err := buildWorkerMailer(cfg, holder)
	if err != nil {
		return fmt.Errorf("worker: build mailer: %w", err)
	}

	client, err := queue.NewClient(workerDB.Pool(), nil, sender, encryptionKey)
	if err != nil {
		return fmt.Errorf("worker: build River client: %w", err)
	}

	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("worker: start: %w", err)
	}
	_, _ = fmt.Fprintln(stdout, "worker: started (leader election active, zero registered producers)")

	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), workerShutdownGrace)
	defer cancel()
	if err := client.Stop(stopCtx); err != nil {
		return fmt.Errorf("worker: graceful shutdown: %w", err)
	}
	// Drain the mailer's bounded pool AFTER the River client stopped
	// working jobs (design D-N: "drained on graceful shutdown"), on the
	// same bounded budget serve uses. Since buildWorkerMailer returns
	// mail.Sync, no invitation_email job leaves anything IN that pool --
	// its send completed, or the job failed and River will retry it. This
	// drain is now belt-and-braces for anything else that might share the
	// mailer, not the thing standing between an accepted job and a
	// delivered message.
	if err := sender.Close(stopCtx); err != nil {
		slog.WarnContext(ctx, "worker: mailer did not drain within the shutdown grace period", "error", err)
	}
	_, _ = fmt.Fprintln(stdout, "worker: stopped")
	return nil
}

// workerMailer is the worker's outbound-mail seam. It is an INTERFACE,
// not the concrete *mail.AsyncMailer, so that a log sink stays
// representable here and TestWorker_DispatchesThroughARealSMTPSender-
// NotALogSink is a live assertion rather than a tautology the compiler
// already guarantees.
type workerMailer interface {
	queue.RawSender
	Close(ctx context.Context) error
}

// buildWorkerMailer builds the worker's real SMTP sender from the same
// SMTP_URL/MAIL_FROM pair serve already uses (internal/mail.New does no
// network I/O: a malformed SMTP_URL fails here, at boot, and an
// unreachable server fails individual sends). runWorker has exactly one
// sender expression and it is this call, so a regression to a log sink
// has to be written here, where the test looks.
func buildWorkerMailer(cfg config.Config, holder *secrets.Holder) (workerMailer, error) {
	m, err := mail.New(mail.Config{SMTPURL: holder.SMTPURL(), From: cfg.MailFrom})
	if err != nil {
		return nil, err
	}
	// mail.Sync, never the bare *AsyncMailer. A worker's return value is
	// what marks its job row completed, and AsyncMailer.SendRaw returns nil
	// the moment the bounded pool accepts the message -- before a byte is
	// dialled -- so River recorded invitation_email COMPLETED before
	// delivery was attempted and every failure after the hand-off fell
	// outside its retry and dead-letter machinery
	// (R4-invitation-email-job-completed-before-delivery-is-attempted,
	// review lineage review-f855997b550a986d). serve keeps the async
	// contract, because there SMTP latency on the response is an
	// account-existence timing oracle (D-N); here nothing is waiting on the
	// response, and the job IS the retry.
	return mail.Sync{AsyncMailer: m}, nil
}
