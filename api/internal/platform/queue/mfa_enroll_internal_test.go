package queue

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
)

// The enrollment code is sealed in river_job, so the worker must open it
// before sending: the email carries the code the user has to type, at
// the address the job names.
func TestMFAEnrollEmailWorker_OpensTheSealedCodeBeforeSending(t *testing.T) {
	key := [32]byte{4, 2}
	const code = "093418"

	sealed, err := mfa.EncryptSecret(key, []byte(code))
	if err != nil {
		t.Fatalf("seal code: %v", err)
	}

	sender := &captureSender{}
	worker := &MFAEnrollEmailWorker{Sender: sender, Key: key}
	job := &river.Job[mfaEnrollEmailJobArgs]{Args: mfaEnrollEmailJobArgs{
		UserID: uuid.New(), Email: "enrollee@example.com", CodeEncrypted: sealed,
	}}

	if err := worker.Work(t.Context(), job); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if sender.to != "enrollee@example.com" {
		t.Fatalf("expected the message addressed to the job's address, got %q", sender.to)
	}
	if !strings.Contains(sender.body, code) {
		t.Fatalf("expected the email to carry the opened code %q, got body: %s", code, sender.body)
	}
}

func TestMFAEnrollEmailWorker_UnopenablePayloadFailsTheJob(t *testing.T) {
	sealed, err := mfa.EncryptSecret([32]byte{1}, []byte("093418"))
	if err != nil {
		t.Fatalf("seal code: %v", err)
	}
	sender := &captureSender{}
	worker := &MFAEnrollEmailWorker{Sender: sender, Key: [32]byte{2}}
	job := &river.Job[mfaEnrollEmailJobArgs]{Args: mfaEnrollEmailJobArgs{
		UserID: uuid.New(), Email: "enrollee@example.com", CodeEncrypted: sealed,
	}}

	if err := worker.Work(t.Context(), job); err == nil {
		t.Fatalf("expected Work to fail on a payload it cannot open")
	}
	if sender.to != "" {
		t.Fatalf("expected nothing to be sent, got a message to %q", sender.to)
	}
}
