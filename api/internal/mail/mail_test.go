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

// R4-invitation-email-job-completed-before-delivery-is-attempted (review
// lineage review-f855997b550a986d). SendRaw hands off to the bounded pool and
// returns nil before SMTP is attempted, which is exactly right for the request
// path it was built for and exactly wrong for a River worker: the job row is
// marked COMPLETED on that nil, so every failure after the hand-off falls
// outside River's retry and dead-letter machinery and the invitee silently
// never receives the code.
//
// mail.Sync is the same mailer with the opposite contract. This asserts the
// half that carries the guarantee: a send the SMTP server refuses must come
// back as an error to the caller, not into a log line.
func TestSync_SendRaw_ReportsDeliveryFailureToTheCaller(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: dials an SMTP port")
	}
	// Port 1 is reserved and nothing listens on it, so the dial fails
	// deterministically without depending on a stopped fixture server.
	m, err := vecmail.New(vecmail.Config{
		SMTPURL:     "smtp://127.0.0.1:1",
		From:        "Vecingest <no-reply@example.com>",
		SendTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("mail.New: %v", err)
	}
	defer func() { _ = m.Close(context.Background()) }()

	if err := m.SendRaw(context.Background(), "user@example.com", "subject", "body"); err != nil {
		t.Fatalf("test premise: the async SendRaw accepts the message without dialling, got %v", err)
	}
	if err := (vecmail.Sync{AsyncMailer: m}).SendRaw(context.Background(), "user@example.com", "subject", "body"); err == nil {
		t.Fatalf("expected the synchronous sender to report an undeliverable message as an error, so the job that called it is retried instead of recorded completed")
	}
}

// The other half: a successful synchronous send must have ALREADY reached the
// server when it returns, with no polling. waitForMessages exists precisely
// because the async path cannot promise that; this path must not need it.
func TestSync_SendRaw_DeliversBeforeReturning(t *testing.T) {
	server := newFakeSMTPServer(t, 0)

	m, err := vecmail.New(vecmail.Config{
		SMTPURL: fmt.Sprintf("smtp://127.0.0.1:%d", server.PortNumber()),
		From:    "Vecingest <no-reply@example.com>",
	})
	if err != nil {
		t.Fatalf("mail.New: %v", err)
	}
	defer func() { _ = m.Close(context.Background()) }()

	if err := (vecmail.Sync{AsyncMailer: m}).SendRaw(context.Background(), "invitee@example.com", "subject", "the-short-code"); err != nil {
		t.Fatalf("Sync.SendRaw: %v", err)
	}
	msgs := server.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected the message to have been delivered before SendRaw returned, got %d message(s)", len(msgs))
	}
	_, body := decodeDeliveredBody(t, msgs[0].MsgRequest())
	if !strings.Contains(body, "the-short-code") {
		t.Fatalf("expected the delivered body to carry the rendered content, got: %s", body)
	}
}
