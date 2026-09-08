// Package cache implements the M0 phase-A Cache seam (design D-D's
// interface table): an in-process TTL set of revoked session families,
// kept current by a Postgres LISTEN/NOTIFY subscription on the
// session_revoked channel, with a fail-closed fallback to an indexed
// `sessions` lookup exactly when that subscription cannot be trusted
// (auth-session-tokens: Immediate Session Revocation Effect, both
// scenarios). Phase B swaps this implementation for Valkey with no
// change to any caller of the Cache interface below.
package cache

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// RevokedChannel is the LISTEN/NOTIFY channel session.Rotator's reuse
// path (and any explicit session-revocation handler) publishes to.
const RevokedChannel = "session_revoked"

// Cache is the D-D revocation-set seam consulted by the Bearer-auth
// middleware after JWT verification.
type Cache interface {
	IsRevoked(ctx context.Context, familyID uuid.UUID) (bool, error)
}

// FamilyLiveChecker is the fallback boundary: whether a family still has
// at least one live (non-revoked) session row. internal/db satisfies
// this via ListLiveSessionsByFamilyID; this package depends on the
// interface only, so its own tests never need a real database.
type FamilyLiveChecker interface {
	HasLiveSession(ctx context.Context, familyID uuid.UUID) (bool, error)
}

// Clock abstracts time.Now() so tests never depend on the wall clock.
type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// RevocationCache is the M0 phase-A Cache implementation.
type RevocationCache struct {
	mu      sync.Mutex
	revoked map[string]time.Time // familyID -> expiry (TTL = access-token lifetime)
	ttl     time.Duration
	clock   Clock

	startedAt time.Time

	listenMu        sync.Mutex
	listenDownSince time.Time // zero means currently healthy (or never started)

	checker FamilyLiveChecker
}

// New builds a RevocationCache. ttl MUST equal the access-token
// lifetime: an entry never needs to be trusted for longer than a token
// signed before the revocation could possibly still be valid.
func New(checker FamilyLiveChecker, ttl time.Duration, clock Clock) *RevocationCache {
	if clock == nil {
		clock = systemClock{}
	}
	return &RevocationCache{
		revoked:   make(map[string]time.Time),
		ttl:       ttl,
		clock:     clock,
		startedAt: clock.Now(),
		checker:   checker,
	}
}

// MarkRevoked records familyID as revoked for ttl. Called directly by
// the same process that just revoked it (the fast, common path), and by
// the LISTEN loop when another replica's NOTIFY arrives.
func (c *RevocationCache) MarkRevoked(familyID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.revoked[familyID.String()] = c.clock.Now().Add(c.ttl)
}

func (c *RevocationCache) prune(now time.Time) {
	for k, exp := range c.revoked {
		if now.After(exp) {
			delete(c.revoked, k)
		}
	}
}

// MarkListenDown starts (or continues) the LISTEN-outage window; call
// this from the LISTEN loop whenever the subscription connection drops.
func (c *RevocationCache) MarkListenDown() {
	c.listenMu.Lock()
	defer c.listenMu.Unlock()
	if c.listenDownSince.IsZero() {
		c.listenDownSince = c.clock.Now()
	}
}

// MarkListenHealthy clears the outage window; call this once the LISTEN
// loop has (re)established its subscription.
func (c *RevocationCache) MarkListenHealthy() {
	c.listenMu.Lock()
	defer c.listenMu.Unlock()
	c.listenDownSince = time.Time{}
}

func (c *RevocationCache) listenDownFor(now time.Time) time.Duration {
	c.listenMu.Lock()
	defer c.listenMu.Unlock()
	if c.listenDownSince.IsZero() {
		return 0
	}
	return now.Sub(c.listenDownSince)
}

// IsRevoked reports whether familyID has been revoked. It fails closed
// to the indexed sessions lookup exactly when: the LISTEN connection has
// been down longer than ttl, OR the process itself booted less than one
// ttl ago -- in both cases the in-process map cannot yet be trusted to
// hold every revocation that could apply to a still-valid access token
// (auth-session-tokens: "Degraded fallback after a LISTEN outage or
// fresh restart").
func (c *RevocationCache) IsRevoked(ctx context.Context, familyID uuid.UUID) (bool, error) {
	now := c.clock.Now()

	freshRestart := now.Sub(c.startedAt) < c.ttl
	listenStale := c.listenDownFor(now) > c.ttl

	if freshRestart || listenStale {
		live, err := c.checker.HasLiveSession(ctx, familyID)
		if err != nil {
			return false, err
		}
		return !live, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.prune(now)
	_, ok := c.revoked[familyID.String()]
	return ok, nil
}
