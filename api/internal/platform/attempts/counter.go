// Package attempts implements the phase-A in-process sliding-window
// AttemptCounter (D-N): the fifth phase-A/phase-B seam, declared
// alongside Limiter/Cache/Queue/Mailer because Limiter counts requests
// while progressive lockout counts failures. Phase B swaps this for
// Valkey with no change to any domain-service caller.
package attempts

import (
	"context"
	"sync"
	"time"
)

// Counter is an in-process, per-key sliding-window failure counter. The
// zero value is not usable; construct with NewCounter.
type Counter struct {
	mu   sync.Mutex
	now  func() time.Time
	logs map[string][]time.Time
}

// NewCounter builds a Counter. now defaults to time.Now when nil, but
// tests should always inject a controllable clock so window-expiry
// assertions never depend on real sleeps.
func NewCounter(now func() time.Time) *Counter {
	if now == nil {
		now = time.Now
	}
	return &Counter{now: now, logs: make(map[string][]time.Time)}
}

// Fail records one failure for key and returns the resulting count of
// failures still within window (i.e. the sliding-window count AFTER
// this failure).
func (c *Counter) Fail(_ context.Context, key string, window time.Duration) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	pruned := prune(c.logs[key], now, window)
	pruned = append(pruned, now)
	c.logs[key] = pruned
	return len(pruned), nil
}

// Count returns how many failures are recorded for key within window,
// without recording a new one.
func (c *Counter) Count(_ context.Context, key string, window time.Duration) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	pruned := prune(c.logs[key], now, window)
	c.logs[key] = pruned
	return len(pruned), nil
}

// Reset clears every recorded failure for key.
func (c *Counter) Reset(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.logs, key)
	return nil
}

// prune drops every timestamp older than now-window, preserving order.
func prune(in []time.Time, now time.Time, window time.Duration) []time.Time {
	cutoff := now.Add(-window)
	out := in[:0:0]
	for _, ts := range in {
		if ts.After(cutoff) {
			out = append(out, ts)
		}
	}
	return out
}
