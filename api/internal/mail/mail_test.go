package mail_test

import (
	"context"
	"fmt"
	"io"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
	"time"

	smtpmock "github.com/mocktools/go-smtp-mock/v2"

	vecmail "github.com/jorgealonsodev/vecingest/internal/mail"
)

// newFakeSMTPServer starts a real, local, loopback-only fake SMTP
// server (never a real network call, never a container) so
// AsyncMailer's tests exercise the actual go-mail wire protocol instead
// of a hand-rolled double. It starts a real listener, so it carries the
// mandatory testing.Short() skip guard even though it needs no Docker.
func newFakeSMTPServer(t *testing.T, responseDelaySeconds int) *smtpmock.Server {
	t.Helper()
	if testing.Short() {
		t.Skip("integration: starts a local SMTP server")
	}
	server := smtpmock.New(smtpmock.ConfigurationAttr{
		HostAddress:           "127.0.0.1",
		PortNumber:            0, // OS-assigned free port
		LogToStdout:           false,
		LogServerActivity:     false,
		ResponseDelayMailfrom: responseDelaySeconds,
	})
	if err := server.Start(); err != nil {
		t.Fatalf("failed to start fake SMTP server: %v", err)
	}
	t.Cleanup(func() { _ = server.Stop() })
	return server
}

func waitForMessages(t *testing.T, server *smtpmock.Server, want int) []smtpmock.Message {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if msgs := server.Messages(); len(msgs) >= want {
			return msgs
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d message(s), got %d", want, len(server.Messages()))
	return nil
}

// decodeDeliveredBody parses the raw DATA content the fake SMTP server
// captured and returns the subject header plus the quoted-printable
// decoded body, so assertions read actual rendered text instead of a
// QP-wrapped line-broken transport encoding.
func decodeDeliveredBody(t *testing.T, raw string) (subject, body string) {
	t.Helper()
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("failed to parse delivered message: %v", err)
	}
	decoded, err := io.ReadAll(quotedprintable.NewReader(msg.Body))
	if err != nil {
		t.Fatalf("failed to decode quoted-printable body: %v", err)
	}
	return msg.Header.Get("Subject"), string(decoded)
}

// request-protection has no direct scenario for this (auth-credentials
// does): the design-mandated real SMTP sender must actually deliver the
// rendered password-reset template through a real SMTP dialogue.
func TestAsyncMailer_SendRaw_DeliversRenderedContent(t *testing.T) {
	server := newFakeSMTPServer(t, 0)

	m, err := vecmail.New(vecmail.Config{
		SMTPURL: fmt.Sprintf("smtp://127.0.0.1:%d", server.PortNumber()),
		From:    "Vecingest <no-reply@example.com>",
	})
	if err != nil {
		t.Fatalf("mail.New: %v", err)
	}
	defer func() { _ = m.Close(context.Background()) }()

	subject, body, err := vecmail.RenderPasswordReset(vecmail.PasswordResetData{RawToken: "reset-token-xyz"})
	if err != nil {
		t.Fatalf("RenderPasswordReset: %v", err)
	}

	if err := m.SendRaw(context.Background(), "user@example.com", subject, body); err != nil {
		t.Fatalf("SendRaw: %v", err)
	}

	msgs := waitForMessages(t, server, 1)
	gotSubject, gotBody := decodeDeliveredBody(t, msgs[0].MsgRequest())
	if gotSubject != vecmail.PasswordResetSubject {
		t.Fatalf("expected subject %q, got %q", vecmail.PasswordResetSubject, gotSubject)
	}
	if !strings.Contains(gotBody, "reset-token-xyz") {
		t.Fatalf("expected the delivered message to contain the rendered token, got: %s", gotBody)
	}
	if !strings.Contains(strings.ToLower(msgs[0].MsgRequest()), "text/html") {
		t.Fatalf("expected the delivered message to be HTML, got: %s", msgs[0].MsgRequest())
	}
}

// The whole point of the bounded async pool (design.md D-N: "sending
// inline would add SMTP latency to the response only for existing
// accounts, which is a timing oracle"): SendRaw must return before the
// slow SMTP round-trip completes, not after.
func TestAsyncMailer_SendRaw_DoesNotBlockOnSlowSMTPServer(t *testing.T) {
	const serverDelaySeconds = 2
	server := newFakeSMTPServer(t, serverDelaySeconds)

	m, err := vecmail.New(vecmail.Config{
		SMTPURL: fmt.Sprintf("smtp://127.0.0.1:%d", server.PortNumber()),
		From:    "Vecingest <no-reply@example.com>",
	})
	if err != nil {
		t.Fatalf("mail.New: %v", err)
	}
	defer func() { _ = m.Close(context.Background()) }()

	start := time.Now()
	if err := m.SendRaw(context.Background(), "user@example.com", "subject", "body"); err != nil {
		t.Fatalf("SendRaw: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed >= serverDelaySeconds*time.Second {
		t.Fatalf("SendRaw blocked for %v, expected it to return immediately (server delay is %ds)", elapsed, serverDelaySeconds)
	}

	// The send did eventually happen in the background, proving this
	// isn't a silent no-op -- only that it's asynchronous.
	waitForMessages(t, server, 1)
}

// Close must drain the bounded pool (design.md D-N: "drained on
// graceful shutdown") rather than dropping a still-queued send when the
// process exits.
func TestAsyncMailer_Close_DrainsPendingSends(t *testing.T) {
	server := newFakeSMTPServer(t, 0)

	m, err := vecmail.New(vecmail.Config{
		SMTPURL: fmt.Sprintf("smtp://127.0.0.1:%d", server.PortNumber()),
		From:    "Vecingest <no-reply@example.com>",
	})
	if err != nil {
		t.Fatalf("mail.New: %v", err)
	}

	if err := m.SendRaw(context.Background(), "user@example.com", "subject", "body"); err != nil {
		t.Fatalf("SendRaw: %v", err)
	}

	closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.Close(closeCtx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if len(server.Messages()) != 1 {
		t.Fatalf("expected Close to have drained the one pending send, got %d messages", len(server.Messages()))
	}
}
