package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/jorgealonsodev/vecingest/internal/config"
	"github.com/jorgealonsodev/vecingest/internal/db"
	platmail "github.com/jorgealonsodev/vecingest/internal/platform/mail"
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
	_, holder, err := config.Load(ctx, lookup, config.CommandWorker)
	if err != nil {
		return err
	}

	workerDB, err := db.NewWorkerHandle(ctx, holder.DatabaseURLWorker())
	if err != nil {
		return fmt.Errorf("worker: connect to database: %w", err)
	}
	defer workerDB.Close()

	// LogMailer, not a real SMTP sender: CommandWorker's config
	// requirement set (internal/config) deliberately declares no
	// SMTP_URL/MAIL_FROM -- the worker subcommand has never needed mail
	// credentials, and M1 does not depend on production SMTP (the paper
	// short-code path works without a mail server). A deployment that
	// wants the worker to actually dispatch invitation email wires a
	// real sender here once that config requirement is added; until
	// then this keeps the worker's zero-new-required-config contract
	// intact while still exercising the real render+log path.
	// ENCRYPTION_KEY is now a worker requirement (internal/config,
	// CommandWorker): the invitation-email job payload carries its
	// short code SEALED, so the worker that renders the email is the
	// only place it is opened again (review lineage
	// review-e72754dc7521b57a).
	var encryptionKey [32]byte
	copy(encryptionKey[:], holder.EncryptionKey())

	client, err := queue.NewClient(workerDB.Pool(), nil, platmail.LogMailer{}, encryptionKey)
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
	_, _ = fmt.Fprintln(stdout, "worker: stopped")
	return nil
}
