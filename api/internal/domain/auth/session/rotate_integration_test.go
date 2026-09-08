package session_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/session"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
	"github.com/jorgealonsodev/vecingest/internal/testhelpers"
)

type fixedRotateClock struct{ now time.Time }

func (c fixedRotateClock) Now() time.Time { return c.now }

// seedUserAndSession inserts a minimal user row and an initial session
// row directly, returning the user id, family id and the raw refresh
// token whose hash the seeded row carries.
func seedUserAndSession(t *testing.T, ctx context.Context, handles db.Handles, expiresAt time.Time) (userID, familyID uuid.UUID, rawRefresh string) {
	t.Helper()
	q := db.New(handles.Write)

	userID = uuid.New()
	if _, err := handles.Write.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, 'x', 'Test User')`,
		userID, userID.String()+"@example.com"); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	familyID = uuid.New()
	rawRefresh, hash, err := token.GenerateRefresh()
	if err != nil {
		t.Fatalf("GenerateRefresh: %v", err)
	}
	if _, err := q.InsertSession(ctx, db.InsertSessionParams{
		ID:               uuid.New(),
		UserID:           userID,
		RefreshTokenHash: hash,
		FamilyID:         familyID,
		DeviceName:       pgtype.Text{},
		Platform:         "web",
		ExpiresAt:        expiresAt,
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	return userID, familyID, rawRefresh
}

// auth-session-tokens: Refresh Rotation with Family Invalidation --
// "Normal rotation".
func TestRotate_NormalRotationSharesFamilyAndInvalidatesOld(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, _ := testhelpers.AppRWHandles(t)
	ctx := context.Background()
	now := time.Now().UTC()

	userID, familyID, rawRefresh := seedUserAndSession(t, ctx, handles, now.Add(30*24*time.Hour))

	rotator := session.Rotator{Clock: fixedRotateClock{now}}
	result, err := rotator.Rotate(ctx, handles.Write, session.RotateInput{
		RawRefreshToken: rawRefresh,
		IsSuperadmin:    false,
		Platform:        "web",
		DeviceName:      "Jorge's iPhone",
	})
	if err != nil {
		t.Fatalf("Rotate: unexpected error: %v", err)
	}
	if result.FamilyID != familyID {
		t.Fatalf("FamilyID = %s, want %s (rotation must preserve the family)", result.FamilyID, familyID)
	}
	if result.UserID != userID {
		t.Fatalf("UserID = %s, want %s", result.UserID, userID)
	}
	if result.RawRefreshToken == rawRefresh {
		t.Fatalf("expected a NEW refresh token, got the same raw value")
	}

	q := db.New(handles.Read)
	oldHash := token.HashToken(rawRefresh)
	oldRow, err := q.GetSessionByRefreshTokenHash(ctx, oldHash)
	if err != nil {
		t.Fatalf("GetSessionByRefreshTokenHash(old): %v", err)
	}
	if !oldRow.RevokedAt.Valid {
		t.Fatalf("expected the old session row to be revoked after rotation")
	}

	newHash := token.HashToken(result.RawRefreshToken)
	newRow, err := q.GetSessionByRefreshTokenHash(ctx, newHash)
	if err != nil {
		t.Fatalf("GetSessionByRefreshTokenHash(new): %v", err)
	}
	if newRow.RevokedAt.Valid {
		t.Fatalf("expected the new session row to be live, got revoked")
	}
	if newRow.FamilyID != familyID {
		t.Fatalf("new row FamilyID = %s, want %s", newRow.FamilyID, familyID)
	}
}

// auth-session-tokens: Refresh Rotation with Family Invalidation --
// "Reuse revokes the family".
func TestRotate_ReuseOfInvalidatedTokenRevokesWholeFamily(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, _ := testhelpers.AppRWHandles(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_, familyID, rawRefresh := seedUserAndSession(t, ctx, handles, now.Add(30*24*time.Hour))

	rotator := session.Rotator{Clock: fixedRotateClock{now}}
	// First rotation succeeds and invalidates rawRefresh.
	if _, err := rotator.Rotate(ctx, handles.Write, session.RotateInput{
		RawRefreshToken: rawRefresh, Platform: "web",
	}); err != nil {
		t.Fatalf("first Rotate: unexpected error: %v", err)
	}

	// Presenting the now-invalidated token again must revoke the whole
	// family and reject the request.
	_, err := rotator.Rotate(ctx, handles.Write, session.RotateInput{
		RawRefreshToken: rawRefresh, Platform: "web",
	})
	if !errors.Is(err, session.ErrRefreshReused) {
		t.Fatalf("second Rotate: error = %v, want ErrRefreshReused", err)
	}

	q := db.New(handles.Read)
	live, err := q.ListLiveSessionsByFamilyID(ctx, familyID)
	if err != nil {
		t.Fatalf("ListLiveSessionsByFamilyID: %v", err)
	}
	if len(live) != 0 {
		t.Fatalf("expected every session in the family to be revoked after reuse, found %d still live", len(live))
	}
}

func TestRotate_UnknownTokenIsRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, _ := testhelpers.AppRWHandles(t)
	ctx := context.Background()

	rotator := session.Rotator{Clock: fixedRotateClock{time.Now().UTC()}}
	_, err := rotator.Rotate(ctx, handles.Write, session.RotateInput{
		RawRefreshToken: "not-a-real-token",
		Platform:        "web",
	})
	if err == nil {
		t.Fatalf("expected an error for an unknown refresh token, got nil")
	}
}

// A refresh token whose session row has already expired must be
// rejected without rotating.
func TestRotate_ExpiredTokenIsRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, _ := testhelpers.AppRWHandles(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_, _, rawRefresh := seedUserAndSession(t, ctx, handles, now.Add(-time.Minute))

	rotator := session.Rotator{Clock: fixedRotateClock{now}}
	_, err := rotator.Rotate(ctx, handles.Write, session.RotateInput{
		RawRefreshToken: rawRefresh, Platform: "web",
	})
	if !errors.Is(err, session.ErrRefreshInvalid) {
		t.Fatalf("Rotate(expired): error = %v, want ErrRefreshInvalid", err)
	}
}

// A zero-value Rotator falls back to the real system clock.
func TestRotator_DefaultClockUsesSystemTime(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, _ := testhelpers.AppRWHandles(t)
	ctx := context.Background()

	_, _, rawRefresh := seedUserAndSession(t, ctx, handles, time.Now().UTC().Add(30*24*time.Hour))

	var rotator session.Rotator // zero value: Clock is nil
	result, err := rotator.Rotate(ctx, handles.Write, session.RotateInput{
		RawRefreshToken: rawRefresh, Platform: "web",
	})
	if err != nil {
		t.Fatalf("Rotate with default clock: unexpected error: %v", err)
	}
	if result.RawRefreshToken == "" {
		t.Fatalf("expected a rotated token")
	}
}

// D-D: superadmin refresh lifetime is 8h, not 30d.
func TestRotate_SuperadminGetsShortRefreshLifetime(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, _ := testhelpers.AppRWHandles(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_, _, rawRefresh := seedUserAndSession(t, ctx, handles, now.Add(8*time.Hour))

	rotator := session.Rotator{Clock: fixedRotateClock{now}}
	result, err := rotator.Rotate(ctx, handles.Write, session.RotateInput{
		RawRefreshToken: rawRefresh,
		IsSuperadmin:    true,
		Platform:        "web",
	})
	if err != nil {
		t.Fatalf("Rotate: unexpected error: %v", err)
	}

	q := db.New(handles.Read)
	row, err := q.GetSessionByRefreshTokenHash(ctx, token.HashToken(result.RawRefreshToken))
	if err != nil {
		t.Fatalf("GetSessionByRefreshTokenHash: %v", err)
	}
	gotLifetime := row.ExpiresAt.Sub(now)
	if gotLifetime > 8*time.Hour+time.Minute || gotLifetime < 8*time.Hour-time.Minute {
		t.Fatalf("expected an ~8h refresh lifetime for superadmin, got %v", gotLifetime)
	}
}
