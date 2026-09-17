package main

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/config"
	"github.com/jorgealonsodev/vecingest/internal/mail"
	platmail "github.com/jorgealonsodev/vecingest/internal/platform/mail"
)

// workerEnv is a complete, valid `worker` environment. Tests below
// delete exactly one key from it, so a failure names the variable under
// test rather than whatever else happened to be missing.
func workerEnv() map[string]string {
	return map[string]string{ //nolint:gosec // G101: a fake fixture DSN, never a real credential -- config.Load only parses it
		"DATABASE_URL":   "postgres://app_rw:pw@127.0.0.1:5432/vecingest",
		"APP_ENV":        "production",
		"ENCRYPTION_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"SMTP_URL":       "smtp://mail.example.com:587",
		"MAIL_FROM":      "no-reply@example.com",
	}
}

func lookupFrom(env map[string]string) config.LookupEnv {
	return func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	}
}

// R4-invitation-email-is-never-dispatched-by-any-process (review lineage
// review-c4efc3f92d076299). The worker subcommand is the ONLY consumer
// of the invitation_email job kind, and it used to construct that
// consumer with platmail.LogMailer: every job decrypted the short code,
// rendered the message, wrote it to a log sink and returned nil, so the
// row was recorded completed and nothing was ever delivered. The
// invitation flow -- M1's central feature -- did not work end to end,
// and it failed silently.
//
// This is a wiring assertion on purpose: the defect was never in
// InvitationEmailWorker.Work (which correctly returns the sender's
// error), it was in which sender the only process that runs workers
// hands it. runWorker builds its sender through exactly this function.
func TestWorker_DispatchesThroughARealSMTPSenderNotALogSink(t *testing.T) {
	cfg, holder, err := config.Load(context.Background(), lookupFrom(workerEnv()), config.CommandWorker)
	if err != nil {
		t.Fatalf("load worker config: %v", err)
	}

	sender, err := buildWorkerMailer(cfg, holder)
	if err != nil {
		t.Fatalf("build worker mailer: %v", err)
	}
	t.Cleanup(func() { _ = sender.Close(context.Background()) })

	if _, isLogSink := any(sender).(platmail.LogMailer); isLogSink {
		t.Fatalf("the worker dispatches invitation email through a log sink: every job would complete without delivering anything")
	}
	// Not *mail.AsyncMailer, which this assertion used to require:
	// AsyncMailer.SendRaw returns nil as soon as the bounded pool accepts
	// the message, so River marked the job COMPLETED before SMTP was
	// attempted and every later failure fell outside its retry machinery
	// (R4-invitation-email-job-completed-before-delivery-is-attempted,
	// review lineage review-f855997b550a986d). The worker needs the
	// synchronous contract; mail.Sync is the same SMTP mailer carrying it.
	if _, isSync := any(sender).(mail.Sync); !isSync {
		t.Fatalf("expected the worker's invitation-email sender to complete delivery before returning (mail.Sync), got %T: a job that completes on hand-off loses the retry the queue exists to provide", sender)
	}
}

// The other half of the same fix: a worker with no SMTP credentials
// cannot silently degrade to delivering nothing, because it refuses to
// boot. SMTP_URL and MAIL_FROM are now declared requirements of
// CommandWorker, exactly as they already were of CommandServe.
func TestWorkerConfig_RequiresSMTPCredentials(t *testing.T) {
	for _, name := range []string{"SMTP_URL", "MAIL_FROM"} {
		t.Run(name, func(t *testing.T) {
			env := workerEnv()
			delete(env, name)

			_, _, err := config.Load(context.Background(), lookupFrom(env), config.CommandWorker)
			var verr *config.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("expected `worker` to refuse to boot without %s, got err=%v", name, err)
			}
			found := false
			for _, missing := range verr.Missing {
				if missing == name {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected %s to be reported missing for the worker command, got missing=%v", name, verr.Missing)
			}
		})
	}
}
