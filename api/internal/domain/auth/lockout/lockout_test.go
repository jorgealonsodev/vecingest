package lockout_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/lockout"
)

// fakeCounter is an in-memory AttemptCounter double, independent of the
// real internal/platform/attempts implementation, so lockout's own
// tests never depend on that package's behavior.
type fakeCounter struct {
	mu    sync.Mutex
	fails map[string]int
}

func newFakeCounter() *fakeCounter { return &fakeCounter{fails: map[string]int{}} }

func (f *fakeCounter) Fail(_ context.Context, key string, _ time.Duration) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fails[key]++
	return f.fails[key], nil
}

func (f *fakeCounter) Count(_ context.Context, key string, _ time.Duration) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fails[key], nil
}

func (f *fakeCounter) Reset(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.fails, key)
	return nil
}

type sentMail struct {
	to      string
	subject string
}

type fakeMailer struct {
	mu   sync.Mutex
	sent []sentMail
}

func (m *fakeMailer) Send(_ context.Context, msg lockout.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, sentMail{to: msg.To, subject: msg.Subject})
	return nil
}

func (m *fakeMailer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

func syncDispatch(fn func()) { fn() }

func newService(counter lockout.AttemptCounter, mailer lockout.Mailer) lockout.Service {
	return lockout.Service{
		Counter:  counter,
		Mailer:   mailer,
		Dispatch: syncDispatch,
	}
}

// auth-credentials: Progressive Lockout -- "Lockout by email".
func TestRecordFailure_LocksByEmailAfterFiveFailures(t *testing.T) {
	counter := newFakeCounter()
	mailer := &fakeMailer{}
	svc := newService(counter, mailer)
	ctx := context.Background()

	var result lockout.FailureResult
	var err error
	for i := 0; i < 6; i++ {
		result, err = svc.RecordFailure(ctx, "victim@example.com", "203.0.113.1", true)
		if err != nil {
			t.Fatalf("RecordFailure #%d: unexpected error: %v", i+1, err)
		}
	}
	if !result.Locked || !result.LockedByEmail {
		t.Fatalf("expected the 6th failure to be locked by email, got %+v", result)
	}
}

// auth-credentials: Progressive Lockout -- "Lockout by IP", across
// DIFFERENT emails.
func TestRecordFailure_LocksByIPAcrossDifferentEmails(t *testing.T) {
	counter := newFakeCounter()
	mailer := &fakeMailer{}
	svc := newService(counter, mailer)
	ctx := context.Background()

	emails := []string{"a@example.com", "b@example.com", "c@example.com", "d@example.com", "e@example.com", "f@example.com"}
	var result lockout.FailureResult
	var err error
	for _, email := range emails {
		result, err = svc.RecordFailure(ctx, email, "203.0.113.9", true)
		if err != nil {
			t.Fatalf("RecordFailure(%s): unexpected error: %v", email, err)
		}
	}
	if !result.Locked || !result.LockedByIP {
		t.Fatalf("expected the 6th distinct-email attempt from the same IP to lock by IP, got %+v", result)
	}
	if result.LockedByEmail {
		t.Fatalf("the 6th email (f@example.com) has only 1 failure of its own; it must not be email-locked")
	}
}

// The email counter MUST advance for unknown addresses too, or lockout
// becomes a user-enumeration oracle.
func TestRecordFailure_UnknownEmailAdvancesCounterIdentically(t *testing.T) {
	counterKnown := newFakeCounter()
	counterUnknown := newFakeCounter()
	svcKnown := newService(counterKnown, &fakeMailer{})
	svcUnknown := newService(counterUnknown, &fakeMailer{})
	ctx := context.Background()

	var knownResult, unknownResult lockout.FailureResult
	for i := 0; i < 6; i++ {
		var err error
		knownResult, err = svcKnown.RecordFailure(ctx, "known@example.com", "198.51.100.1", true)
		if err != nil {
			t.Fatalf("RecordFailure(known): %v", err)
		}
		unknownResult, err = svcUnknown.RecordFailure(ctx, "unknown@example.com", "198.51.100.2", false)
		if err != nil {
			t.Fatalf("RecordFailure(unknown): %v", err)
		}
	}
	if knownResult.LockedByEmail != unknownResult.LockedByEmail {
		t.Fatalf("known and unknown emails must lock identically: known=%v unknown=%v", knownResult, unknownResult)
	}
	if !unknownResult.LockedByEmail {
		t.Fatalf("expected the unknown email to be locked exactly like a known one after 6 failures")
	}
}

// A successful login resets the email counter only, never the IP
// counter.
func TestRecordSuccess_ResetsEmailCounterOnlyNotIP(t *testing.T) {
	counter := newFakeCounter()
	svc := newService(counter, &fakeMailer{})
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		if _, err := svc.RecordFailure(ctx, "victim@example.com", "203.0.113.5", true); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}
	if err := svc.RecordSuccess(ctx, "victim@example.com"); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}

	locked, err := svc.IsLocked(ctx, "victim@example.com", "203.0.113.5")
	if err != nil {
		t.Fatalf("IsLocked: %v", err)
	}
	if locked {
		t.Fatalf("expected the email counter to be reset after success")
	}

	// The IP counter must be unaffected: 5 more failures from other
	// emails at the SAME ip should still reach the threshold.
	for i := 0; i < 5; i++ {
		if _, err := svc.RecordFailure(ctx, "other@example.com", "203.0.113.5", true); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}
	locked, err = svc.IsLocked(ctx, "someone-else@example.com", "203.0.113.5")
	if err != nil {
		t.Fatalf("IsLocked: %v", err)
	}
	if !locked {
		t.Fatalf("expected the IP counter to still reflect the 4 pre-reset + 5 post-reset failures and be locked")
	}
}

// The alert email is sent only for a real account, at most once per
// lock window (dedup key), and is dispatched off the caller's own call
// path (async) -- exercised here via a synchronous test Dispatch so the
// assertion is deterministic.
func TestRecordFailure_SendsAlertOnceForRealAccountOnly(t *testing.T) {
	counter := newFakeCounter()
	mailer := &fakeMailer{}
	svc := newService(counter, mailer)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := svc.RecordFailure(ctx, "victim@example.com", "203.0.113.7", true); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}
	if mailer.count() != 1 {
		t.Fatalf("expected exactly one alert email after crossing the threshold, got %d", mailer.count())
	}

	// Further failed attempts while still locked must not send another.
	if _, err := svc.RecordFailure(ctx, "victim@example.com", "203.0.113.7", true); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if mailer.count() != 1 {
		t.Fatalf("expected the alert to be deduped within the same lock window, got %d sends", mailer.count())
	}
}

func TestRecordFailure_NoAlertForUnknownAccount(t *testing.T) {
	counter := newFakeCounter()
	mailer := &fakeMailer{}
	svc := newService(counter, mailer)
	ctx := context.Background()

	for i := 0; i < 6; i++ {
		if _, err := svc.RecordFailure(ctx, "nobody@example.com", "203.0.113.8", false); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}
	if mailer.count() != 0 {
		t.Fatalf("expected no alert for an unregistered address, got %d", mailer.count())
	}
}

// D-N: "Progressive" is the escalating window: 15 -> 30 -> 60 minutes,
// capped at 60, across consecutive lock cycles on the same key.
func TestRecordFailure_EscalatesBlockDurationAcrossCycles(t *testing.T) {
	counter := newFakeCounter()
	svc := newService(counter, &fakeMailer{})
	ctx := context.Background()

	lockAndReset := func(email string) time.Duration {
		var result lockout.FailureResult
		for i := 0; i < 5; i++ {
			var err error
			result, err = svc.RecordFailure(ctx, email, "203.0.113.20", true)
			if err != nil {
				t.Fatalf("RecordFailure: %v", err)
			}
		}
		if !result.LockedByEmail {
			t.Fatalf("expected email to be locked after 5 failures")
		}
		if err := svc.RecordSuccess(ctx, email); err != nil {
			t.Fatalf("RecordSuccess: %v", err)
		}
		return result.BlockDuration
	}

	d1 := lockAndReset("cycles@example.com")
	d2 := lockAndReset("cycles@example.com")
	d3 := lockAndReset("cycles@example.com")
	d4 := lockAndReset("cycles@example.com")

	if d1 != 15*time.Minute {
		t.Errorf("cycle 1 duration = %v, want 15m", d1)
	}
	if d2 != 30*time.Minute {
		t.Errorf("cycle 2 duration = %v, want 30m", d2)
	}
	if d3 != 60*time.Minute {
		t.Errorf("cycle 3 duration = %v, want 60m", d3)
	}
	if d4 != 60*time.Minute {
		t.Errorf("cycle 4 duration = %v, want 60m (capped)", d4)
	}
}
