package queue

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
)

// captureSender records the single message Work dispatches, so the test
// asserts on what the invitee would actually receive.
type captureSender struct{ to, body string }

func (c *captureSender) SendRaw(_ context.Context, to, _, body string) error {
	c.to, c.body = to, body
	return nil
}

// Sealing the short code in the job row (review lineage
// review-e72754dc7521b57a) is only correct if the worker still opens it
// at send time: the email MUST carry the plaintext code the invitee was
// issued, never the ciphertext that replaced it in river_job.
func TestInvitationEmailWorker_OpensTheSealedShortCodeBeforeSending(t *testing.T) {
	key := [32]byte{9, 8, 7, 6, 5, 4, 3, 2, 1}
	const shortCode = "K7M2P9QZ"

	sealed, err := mfa.EncryptSecret(key, []byte(shortCode))
	if err != nil {
		t.Fatalf("seal short code: %v", err)
	}

	sender := &captureSender{}
	worker := &InvitationEmailWorker{Sender: sender, Key: key}
	job := &river.Job[invitationEmailJobArgs]{Args: invitationEmailJobArgs{
		InvitationID:       uuid.New(),
		Email:              "sealed-invitee@example.com",
		ShortCodeEncrypted: sealed,
	}}

	if err := worker.Work(t.Context(), job); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if sender.to != "sealed-invitee@example.com" {
		t.Fatalf("expected the message addressed to the invitee, got %q", sender.to)
	}
	if !strings.Contains(sender.body, shortCode) {
		t.Fatalf("expected the invitation email to carry the opened short code %q, got body: %s", shortCode, sender.body)
	}
	if strings.Contains(sender.body, string(sealed)) {
		t.Fatalf("expected the invitation email to carry the opened short code, not the stored ciphertext")
	}
}

// A payload that cannot be opened (wrong key, tampered ciphertext) must
// fail the job, never dispatch an email carrying whatever bytes were
// there.
func TestInvitationEmailWorker_RefusesAnUnopenablePayload(t *testing.T) {
	sealed, err := mfa.EncryptSecret([32]byte{1}, []byte("K7M2P9QZ"))
	if err != nil {
		t.Fatalf("seal short code: %v", err)
	}

	sender := &captureSender{}
	worker := &InvitationEmailWorker{Sender: sender, Key: [32]byte{2}}
	job := &river.Job[invitationEmailJobArgs]{Args: invitationEmailJobArgs{
		InvitationID:       uuid.New(),
		Email:              "sealed-invitee@example.com",
		ShortCodeEncrypted: sealed,
	}}

	if err := worker.Work(t.Context(), job); err == nil {
		t.Fatalf("expected Work to fail on a payload it cannot open")
	}
	if sender.body != "" {
		t.Fatalf("expected no email dispatched for an unopenable payload, got body: %s", sender.body)
	}
}
