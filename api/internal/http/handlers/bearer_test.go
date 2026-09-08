package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
	"github.com/jorgealonsodev/vecingest/internal/platform/cache"
)

type fakeFamilyChecker struct{ live bool }

func (f fakeFamilyChecker) HasLiveSession(context.Context, uuid.UUID) (bool, error) {
	return f.live, nil
}

func newTestDeps() *Deps {
	return &Deps{
		AccessIssuer: token.Issuer{Secret: []byte("test-secret-32-bytes-long-enough")},
	}
}

func TestAuthenticate_MissingHeaderRejected(t *testing.T) {
	d := newTestDeps()
	if _, err := d.authenticate(context.Background(), ""); err == nil {
		t.Fatalf("expected an error for a missing Authorization header")
	}
}

func TestAuthenticate_WrongSchemeRejected(t *testing.T) {
	d := newTestDeps()
	if _, err := d.authenticate(context.Background(), "Basic abc123"); err == nil {
		t.Fatalf("expected an error for a non-Bearer scheme")
	}
}

func TestAuthenticate_ValidBearerAccepted(t *testing.T) {
	d := newTestDeps()
	familyID := uuid.New()
	tok, err := d.AccessIssuer.IssueAccess(uuid.New(), familyID, false)
	if err != nil {
		t.Fatalf("unexpected error issuing token: %v", err)
	}

	claims, err := d.authenticate(context.Background(), "Bearer "+tok)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claims.SID != familyID.String() {
		t.Fatalf("expected sid %s, got %s", familyID, claims.SID)
	}
}

// auth-session-tokens: Immediate Session Revocation Effect.
func TestAuthenticate_RevokedSessionRejected(t *testing.T) {
	d := newTestDeps()
	familyID := uuid.New()
	tok, err := d.AccessIssuer.IssueAccess(uuid.New(), familyID, false)
	if err != nil {
		t.Fatalf("unexpected error issuing token: %v", err)
	}

	rc := cache.New(fakeFamilyChecker{live: false}, 15*time.Minute, nil)
	// Warm past the fresh-restart fallback window so the in-process map
	// is authoritative, then mark the family revoked directly.
	rc.MarkRevoked(familyID)
	d.RevocationCache = rc
	// Force past the fresh-restart window deterministically by using a
	// fallback checker that already reports "not live" -- either path
	// (in-process map or fallback) must reject.

	if _, err := d.authenticate(context.Background(), "Bearer "+tok); err == nil {
		t.Fatalf("expected the revoked session's access token to be rejected")
	}
}
