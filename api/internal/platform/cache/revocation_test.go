package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/platform/cache"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

type fakeFamilyChecker struct {
	live map[string]bool
	err  error
	n    int
}

func (f *fakeFamilyChecker) HasLiveSession(_ context.Context, familyID uuid.UUID) (bool, error) {
	f.n++
	if f.err != nil {
		return false, f.err
	}
	return f.live[familyID.String()], nil
}

// auth-session-tokens: Immediate Session Revocation Effect — "Access
// token rejected right after revocation" scenario.
func TestRevocationCache_MarkRevokedThenIsRevoked(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100000, 0)}
	ttl := 15 * time.Minute
	checker := &fakeFamilyChecker{live: map[string]bool{}}
	c := cache.New(checker, ttl, clock)
	// Advance past the "fresh restart" fallback window so IsRevoked
	// trusts the in-process map.
	clock.now = clock.now.Add(ttl + time.Second)

	familyID := uuid.New()
	if revoked, err := c.IsRevoked(context.Background(), familyID); err != nil || revoked {
		t.Fatalf("expected not revoked before MarkRevoked, got revoked=%v err=%v", revoked, err)
	}

	c.MarkRevoked(familyID)

	revoked, err := c.IsRevoked(context.Background(), familyID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !revoked {
		t.Fatalf("expected the session to be immediately revoked in-process")
	}
	if checker.n != 0 {
		t.Fatalf("expected a healthy, warmed-up cache to never hit the fallback checker, got %d calls", checker.n)
	}
}

func TestRevocationCache_EntryExpiresAfterTTL(t *testing.T) {
	clock := &fakeClock{now: time.Unix(200000, 0)}
	ttl := 15 * time.Minute
	checker := &fakeFamilyChecker{live: map[string]bool{}}
	c := cache.New(checker, ttl, clock)
	clock.now = clock.now.Add(ttl + time.Second)

	familyID := uuid.New()
	c.MarkRevoked(familyID)

	clock.now = clock.now.Add(ttl + time.Minute)
	revoked, err := c.IsRevoked(context.Background(), familyID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if revoked {
		t.Fatalf("expected the revoked entry to have expired after TTL (an access token can't outlive it anyway)")
	}
}

// auth-session-tokens: Immediate Session Revocation Effect — "Degraded
// fallback after a LISTEN outage or fresh restart" scenario, the
// fresh-restart leg: a process younger than one TTL always falls back
// to the indexed lookup, regardless of the in-process map's state.
func TestRevocationCache_FreshRestartAlwaysFallsBack(t *testing.T) {
	clock := &fakeClock{now: time.Unix(300000, 0)}
	ttl := 15 * time.Minute
	familyID := uuid.New()
	checker := &fakeFamilyChecker{live: map[string]bool{familyID.String(): false}}
	c := cache.New(checker, ttl, clock)

	// Process just started (clock.now == startedAt): must fall back even
	// though nothing was ever marked revoked in-process.
	revoked, err := c.IsRevoked(context.Background(), familyID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !revoked {
		t.Fatalf("expected the fallback lookup (no live session) to report revoked")
	}
	if checker.n != 1 {
		t.Fatalf("expected the fresh-restart window to hit the fallback checker, got %d calls", checker.n)
	}
}

// The LISTEN-outage leg: once MarkListenDown has been called and stays
// down longer than TTL, IsRevoked falls back even for a long-running
// process.
func TestRevocationCache_ListenOutageLongerThanTTLFallsBack(t *testing.T) {
	clock := &fakeClock{now: time.Unix(400000, 0)}
	ttl := 15 * time.Minute
	c := cache.New(&fakeFamilyChecker{live: map[string]bool{}}, ttl, clock)
	clock.now = clock.now.Add(ttl + time.Second) // past the fresh-restart window

	c.MarkListenDown()
	clock.now = clock.now.Add(ttl + time.Second) // outage now longer than TTL

	familyID := uuid.New()
	checker2 := &fakeFamilyChecker{live: map[string]bool{familyID.String(): true}}
	c2 := cache.New(checker2, ttl, clock)
	c2.MarkListenDown()
	clock.now = clock.now.Add(ttl + time.Second)

	revoked, err := c2.IsRevoked(context.Background(), familyID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if revoked {
		t.Fatalf("expected the fallback (live session found) to report not revoked")
	}
	if checker2.n != 1 {
		t.Fatalf("expected the outage window to hit the fallback checker, got %d calls", checker2.n)
	}
	_ = c
}

func TestRevocationCache_ListenHealthyClearsOutage(t *testing.T) {
	clock := &fakeClock{now: time.Unix(500000, 0)}
	ttl := 15 * time.Minute
	familyID := uuid.New()
	checker := &fakeFamilyChecker{live: map[string]bool{}}
	c := cache.New(checker, ttl, clock)
	clock.now = clock.now.Add(ttl + time.Second)

	c.MarkListenDown()
	c.MarkListenHealthy()

	if _, err := c.IsRevoked(context.Background(), familyID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checker.n != 0 {
		t.Fatalf("expected a healthy LISTEN to skip the fallback entirely, got %d calls", checker.n)
	}
}
