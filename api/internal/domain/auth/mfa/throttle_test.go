package mfa_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
)

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

func alwaysFail(context.Context) (bool, error)   { return false, nil }
func alwaysAccept(context.Context) (bool, error) { return true, nil }

// auth-mfa-totp: TOTP Attempt Throttling -- "Fifth TOTP failure blocks
// the sixth attempt".
func TestThrottledVerify_BlocksSixthFailure(t *testing.T) {
	counter := newFakeCounter()
	userID := uuid.New()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		accepted, throttled, err := mfa.ThrottledVerify(ctx, counter, userID, alwaysFail)
		if err != nil {
			t.Fatalf("ThrottledVerify #%d: unexpected error: %v", i+1, err)
		}
		if accepted || throttled {
			t.Fatalf("attempt #%d: accepted=%v throttled=%v, want both false", i+1, accepted, throttled)
		}
	}

	_, throttled, err := mfa.ThrottledVerify(ctx, counter, userID, alwaysFail)
	if err != nil {
		t.Fatalf("6th attempt: unexpected error: %v", err)
	}
	if !throttled {
		t.Fatalf("expected the 6th attempt to be throttled")
	}
}

// auth-mfa-totp: TOTP Attempt Throttling -- "Recovery-code failures
// count toward the same budget": exercised here as "any failing verify
// closure counts", which is exactly how ConsumeRecoveryCode's caller is
// expected to wire in.
func TestThrottledVerify_SharesBudgetAcrossFailureSources(t *testing.T) {
	counter := newFakeCounter()
	userID := uuid.New()
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		if _, _, err := mfa.ThrottledVerify(ctx, counter, userID, alwaysFail); err != nil {
			t.Fatalf("totp failure #%d: %v", i+1, err)
		}
	}
	// A 5th failure via a DIFFERENT verify source (standing in for a
	// recovery-code failure) still counts toward the same key.
	if _, _, err := mfa.ThrottledVerify(ctx, counter, userID, alwaysFail); err != nil {
		t.Fatalf("5th failure: %v", err)
	}

	_, throttled, err := mfa.ThrottledVerify(ctx, counter, userID, alwaysAccept)
	if err != nil {
		t.Fatalf("6th attempt: %v", err)
	}
	if !throttled {
		t.Fatalf("expected the 6th attempt (even a would-be success) to be throttled")
	}
}

// auth-mfa-totp: TOTP Attempt Throttling -- "Success resets the
// per-user counter".
func TestThrottledVerify_SuccessResetsCounter(t *testing.T) {
	counter := newFakeCounter()
	userID := uuid.New()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, _, err := mfa.ThrottledVerify(ctx, counter, userID, alwaysFail); err != nil {
			t.Fatalf("failure #%d: %v", i+1, err)
		}
	}
	accepted, throttled, err := mfa.ThrottledVerify(ctx, counter, userID, alwaysAccept)
	if err != nil {
		t.Fatalf("success attempt: %v", err)
	}
	if !accepted || throttled {
		t.Fatalf("expected the success to be accepted and not throttled, got accepted=%v throttled=%v", accepted, throttled)
	}

	// A subsequent failure is counted as the first of a new window.
	if _, throttled, err := mfa.ThrottledVerify(ctx, counter, userID, alwaysFail); err != nil || throttled {
		t.Fatalf("post-reset failure: throttled=%v err=%v, want throttled=false", throttled, err)
	}
}

// auth-mfa-totp: TOTP Attempt Throttling -- "Success does not reset the
// shared IP counter": ThrottledVerify only ever touches the per-user
// key; it must never be given the IP-keyed counter to reset.
func TestThrottledVerify_NeverTouchesAnyOtherKey(t *testing.T) {
	counter := newFakeCounter()
	userID := uuid.New()
	ctx := context.Background()

	counter.fails["auth:fail:ip:203.0.113.1"] = 4
	if _, _, err := mfa.ThrottledVerify(ctx, counter, userID, alwaysAccept); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counter.fails["auth:fail:ip:203.0.113.1"] != 4 {
		t.Fatalf("expected the unrelated IP key to be untouched, got %d", counter.fails["auth:fail:ip:203.0.113.1"])
	}
}

func TestThrottledVerify_PropagatesVerifyError(t *testing.T) {
	counter := newFakeCounter()
	userID := uuid.New()
	ctx := context.Background()
	boom := errors.New("boom")

	_, _, err := mfa.ThrottledVerify(ctx, counter, userID, func(context.Context) (bool, error) {
		return false, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected the verify error to propagate, got %v", err)
	}
}
