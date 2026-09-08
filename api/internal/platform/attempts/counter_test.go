package attempts_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jorgealonsodev/vecingest/internal/platform/attempts"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// D-N: AttemptCounter.Fail increments a sliding-window count for key.
func TestCounter_FailIncrementsWithinWindow(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	c := attempts.NewCounter(clock.Now)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		n, err := c.Fail(ctx, "k", 15*time.Minute)
		if err != nil {
			t.Fatalf("Fail: unexpected error: %v", err)
		}
		if n != i {
			t.Errorf("Fail #%d: count = %d, want %d", i, n, i)
		}
	}

	n, err := c.Count(ctx, "k", 15*time.Minute)
	if err != nil {
		t.Fatalf("Count: unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("Count = %d, want 5", n)
	}
}

// Entries older than the window must not count.
func TestCounter_WindowExpiryPrunesOldFailures(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	c := attempts.NewCounter(clock.Now)
	ctx := context.Background()

	if _, err := c.Fail(ctx, "k", 15*time.Minute); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	clock.Advance(16 * time.Minute)
	if _, err := c.Fail(ctx, "k", 15*time.Minute); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	n, err := c.Count(ctx, "k", 15*time.Minute)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 1 {
		t.Errorf("Count = %d, want 1 (the expired failure must be pruned)", n)
	}
}

// Reset clears the key entirely.
func TestCounter_ResetClearsKey(t *testing.T) {
	c := attempts.NewCounter(time.Now)
	ctx := context.Background()

	if _, err := c.Fail(ctx, "k", 15*time.Minute); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if err := c.Reset(ctx, "k"); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	n, err := c.Count(ctx, "k", 15*time.Minute)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 0 {
		t.Errorf("Count after Reset = %d, want 0", n)
	}
}

// Different keys are independent -- this is what makes the email/IP
// dual-keying in D-N work at all.
func TestCounter_KeysAreIndependent(t *testing.T) {
	c := attempts.NewCounter(time.Now)
	ctx := context.Background()

	if _, err := c.Fail(ctx, "email:a@example.com", 15*time.Minute); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	n, err := c.Count(ctx, "ip:1.2.3.4", 15*time.Minute)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 0 {
		t.Errorf("Count for an untouched key = %d, want 0", n)
	}
}

func TestCounter_ConcurrentFailsAreRaceFree(t *testing.T) {
	c := attempts.NewCounter(time.Now)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Fail(ctx, "concurrent", time.Minute); err != nil {
				t.Errorf("Fail: %v", err)
			}
		}()
	}
	wg.Wait()
	n, err := c.Count(ctx, "concurrent", time.Minute)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 50 {
		t.Errorf("Count = %d, want 50", n)
	}
}
