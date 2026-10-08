package queue

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
)

// stubChallenges serves one otp_challenges row by ID; a nil row is a
// challenge that does not exist.
type stubChallenges struct {
	row *db.OtpChallenge
	err error
}

func (s stubChallenges) GetOTPChallenge(_ context.Context, id uuid.UUID) (db.OtpChallenge, error) {
	if s.err != nil {
		return db.OtpChallenge{}, s.err
	}
	if s.row == nil || s.row.ID != id {
		return db.OtpChallenge{}, pgx.ErrNoRows
	}
	return *s.row, nil
}

func openChallenge(id uuid.UUID) *db.OtpChallenge {
	return &db.OtpChallenge{ID: id, Purpose: mfa.EnrollEmailPurpose, ExpiresAt: time.Now().Add(mfa.EnrollEmailTTL)}
}

func sealedEnrollJob(t *testing.T, key [32]byte, challengeID uuid.UUID, code string) *river.Job[mfaEnrollEmailJobArgs] {
	t.Helper()
	sealed, err := mfa.EncryptSecret(key, []byte(code))
	if err != nil {
		t.Fatalf("seal code: %v", err)
	}
	return &river.Job[mfaEnrollEmailJobArgs]{Args: mfaEnrollEmailJobArgs{
		UserID: uuid.New(), ChallengeID: challengeID, Email: "enrollee@example.com", CodeEncrypted: sealed,
	}}
}

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

	challengeID := uuid.New()
	sender := &captureSender{}
	worker := &MFAEnrollEmailWorker{Sender: sender, Key: key, Challenges: stubChallenges{row: openChallenge(challengeID)}}
	job := &river.Job[mfaEnrollEmailJobArgs]{Args: mfaEnrollEmailJobArgs{
		UserID: uuid.New(), ChallengeID: challengeID, Email: "enrollee@example.com", CodeEncrypted: sealed,
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
	if want := fmt.Sprintf("expires in %d minutes", int(mfa.EnrollEmailTTL.Minutes())); !strings.Contains(sender.body, want) {
		t.Fatalf("expected the email to state the issued TTL (%q), got body: %s", want, sender.body)
	}
}

// A retried job must not mail a code that can no longer confirm
// anything: the challenge was superseded by a re-enrollment, expired,
// already verified, or is gone. The job completes without sending, so
// the user never receives a stale code that looks current.
func TestMFAEnrollEmailWorker_SkipsAChallengeThatIsNoLongerOpen(t *testing.T) {
	key := [32]byte{4, 2}
	id := uuid.New()
	expired := openChallenge(id)
	expired.ExpiresAt = time.Now().Add(-time.Second)
	verified := openChallenge(id)
	verified.VerifiedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}

	for name, challenges := range map[string]stubChallenges{
		"expired or superseded": {row: expired},
		"verified":              {row: verified},
		"missing":               {},
	} {
		t.Run(name, func(t *testing.T) {
			sender := &captureSender{}
			worker := &MFAEnrollEmailWorker{Sender: sender, Key: key, Challenges: challenges}
			if err := worker.Work(t.Context(), sealedEnrollJob(t, key, id, "093418")); err != nil {
				t.Fatalf("expected the job to complete without sending, got %v", err)
			}
			if sender.to != "" {
				t.Fatalf("expected nothing to be sent, got a message to %q", sender.to)
			}
		})
	}
}

// A failed lookup is not proof the challenge is closed: the job fails
// and River retries it, rather than silently dropping a valid code.
func TestMFAEnrollEmailWorker_LookupFailureFailsTheJob(t *testing.T) {
	key := [32]byte{4, 2}
	sender := &captureSender{}
	worker := &MFAEnrollEmailWorker{Sender: sender, Key: key, Challenges: stubChallenges{err: fmt.Errorf("connection refused")}}
	if err := worker.Work(t.Context(), sealedEnrollJob(t, key, uuid.New(), "093418")); err == nil {
		t.Fatalf("expected Work to fail when the challenge cannot be read")
	}
	if sender.to != "" {
		t.Fatalf("expected nothing to be sent, got a message to %q", sender.to)
	}
}

func TestMFAEnrollEmailWorker_UnopenablePayloadFailsTheJob(t *testing.T) {
	sealed, err := mfa.EncryptSecret([32]byte{1}, []byte("093418"))
	if err != nil {
		t.Fatalf("seal code: %v", err)
	}
	sender := &captureSender{}
	challengeID := uuid.New()
	worker := &MFAEnrollEmailWorker{Sender: sender, Key: [32]byte{2}, Challenges: stubChallenges{row: openChallenge(challengeID)}}
	job := &river.Job[mfaEnrollEmailJobArgs]{Args: mfaEnrollEmailJobArgs{
		UserID: uuid.New(), ChallengeID: challengeID, Email: "enrollee@example.com", CodeEncrypted: sealed,
	}}

	if err := worker.Work(t.Context(), job); err == nil {
		t.Fatalf("expected Work to fail on a payload it cannot open")
	}
	if sender.to != "" {
		t.Fatalf("expected nothing to be sent, got a message to %q", sender.to)
	}
}
