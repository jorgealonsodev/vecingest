package session

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/audit"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
)

// sessionRevokedChannel is the Postgres LISTEN/NOTIFY channel that
// propagates revocation to every replica within the D-D fail-closed
// bound (auth-session-tokens: Immediate Session Revocation Effect).
const sessionRevokedChannel = "session_revoked"

// Clock abstracts time.Now() so rotation tests never depend on the wall
// clock.
type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

var (
	// ErrRefreshInvalid is returned for a refresh token that does not
	// match any session row, or whose row has already expired.
	ErrRefreshInvalid = errors.New("session: refresh token invalid or expired")
	// ErrRefreshReused is returned when a refresh token that was
	// already rotated away is presented again -- the reuse-detection
	// signal that revokes the whole family
	// (auth-session-tokens: Refresh Rotation with Family Invalidation).
	ErrRefreshReused = errors.New("session: refresh token reuse detected, family revoked")
)

// RotateInput carries what Rotate needs beyond the presented token: the
// new session row's device metadata and the caller's own
// already-resolved isSuperadmin fact (session never queries the users
// table itself, keeping its DB surface limited to sessions/audit_log).
type RotateInput struct {
	RawRefreshToken string
	IsSuperadmin    bool
	DeviceName      string
	Platform        string
	IP              *netip.Addr
}

// RotateResult is what a successful rotation produces.
type RotateResult struct {
	UserID          uuid.UUID
	FamilyID        uuid.UUID
	SessionID       uuid.UUID
	RawRefreshToken string
	ExpiresAt       time.Time
}

// Rotator implements single-use refresh rotation with family
// invalidation (D-D).
type Rotator struct {
	Clock Clock
}

func (r Rotator) clock() Clock {
	if r.Clock != nil {
		return r.Clock
	}
	return systemClock{}
}

// Rotate looks up in.RawRefreshToken by its SHA-256 hash and either:
//   - rotates it (issues a new refresh token sharing the same family_id,
//     revokes the old row), or
//   - detects reuse (the matched row is already revoked) and revokes the
//     ENTIRE family, writing an audit_log entry and NOTIFYing
//     session_revoked so every replica drops the family's access tokens
//     immediately (D-D's fail-closed revocation bound).
//
// Both outcomes run in a single transaction on wdb.
func (r Rotator) Rotate(ctx context.Context, wdb db.WriteDB, in RotateInput) (RotateResult, error) {
	hash := token.HashToken(in.RawRefreshToken)

	tx, err := wdb.Begin(ctx)
	if err != nil {
		return RotateResult{}, fmt.Errorf("session: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := db.New(tx)
	row, err := q.GetSessionByRefreshTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RotateResult{}, ErrRefreshInvalid
		}
		return RotateResult{}, fmt.Errorf("session: lookup refresh token: %w", err)
	}

	now := r.clock().Now()

	if row.RevokedAt.Valid {
		if err := r.revokeFamilyForReuse(ctx, tx, q, row); err != nil {
			return RotateResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return RotateResult{}, fmt.Errorf("session: commit reuse revocation: %w", err)
		}
		return RotateResult{}, ErrRefreshReused
	}

	if !row.ExpiresAt.After(now) {
		return RotateResult{}, ErrRefreshInvalid
	}

	newRaw, newHash, err := token.GenerateRefresh()
	if err != nil {
		return RotateResult{}, fmt.Errorf("session: generate refresh: %w", err)
	}
	newSessionID := uuid.New()
	newExpiresAt := now.Add(token.RefreshLifetime(in.IsSuperadmin))

	var deviceName pgtype.Text
	if in.DeviceName != "" {
		deviceName = pgtype.Text{String: in.DeviceName, Valid: true}
	}

	newRow, err := q.InsertSession(ctx, db.InsertSessionParams{
		ID:               newSessionID,
		UserID:           row.UserID,
		RefreshTokenHash: newHash,
		FamilyID:         row.FamilyID,
		DeviceName:       deviceName,
		Platform:         in.Platform,
		Ip:               in.IP,
		ExpiresAt:        newExpiresAt,
	})
	if err != nil {
		return RotateResult{}, fmt.Errorf("session: insert rotated session: %w", err)
	}
	if err := q.RevokeSession(ctx, row.ID); err != nil {
		return RotateResult{}, fmt.Errorf("session: revoke old session: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return RotateResult{}, fmt.Errorf("session: commit rotation: %w", err)
	}

	return RotateResult{
		UserID:          newRow.UserID,
		FamilyID:        newRow.FamilyID,
		SessionID:       newRow.ID,
		RawRefreshToken: newRaw,
		ExpiresAt:       newRow.ExpiresAt,
	}, nil
}

// revokeFamilyForReuse handles the reuse branch: revoke every live
// session in the family, record it in the audit chain, and NOTIFY so
// every replica's revocation cache drops the family's access tokens
// immediately, not merely at their next refresh.
func (r Rotator) revokeFamilyForReuse(ctx context.Context, tx pgx.Tx, q *db.Queries, row db.Session) error {
	if err := q.RevokeSessionFamily(ctx, row.FamilyID); err != nil {
		return fmt.Errorf("session: revoke family on reuse: %w", err)
	}

	familyID := row.FamilyID
	userID := row.UserID
	// r.clock() already satisfies audit.Clock structurally (both
	// declare the identical Now() time.Time method at their own point
	// of use); no adapter needed.
	if _, err := audit.Append(ctx, tx, r.clock(), audit.Entry{
		UserID: &userID,
		Action: "auth.refresh_reused",
		Entity: "session",
	}); err != nil {
		return fmt.Errorf("session: audit reuse: %w", err)
	}

	if _, err := tx.Exec(ctx, "SELECT pg_notify($1, $2)", sessionRevokedChannel, familyID.String()); err != nil {
		return fmt.Errorf("session: notify session_revoked: %w", err)
	}
	return nil
}
