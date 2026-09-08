package audit

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
)

// chainLockKey is the pinned pg_advisory_xact_lock key that serializes
// every Append (D-O). It is global rather than per-scope because
// community_id is nullable at M0 and null for every authentication
// event, so a per-scope chain would leave M0's entire audit trail
// unchained.
const chainLockKey = 5432003

// genesisHash is prev_hash for the very first row ever inserted.
var genesisHash = make([]byte, 32)

// Append is the audit_log hash chain's single writer (D-O). It MUST run
// inside the same transaction as the domain change it is recording: the
// caller opens tx, calls Append, and commits together. Append:
//  1. takes pg_advisory_xact_lock(chainLockKey), serializing concurrent
//     Appends against the same Postgres backend group;
//  2. reads the current chain head from the partitioned parent (a month
//     boundary carries no chain semantics: only the newest partition is
//     ever touched by that query's ORDER BY ... LIMIT 1);
//  3. assigns a UUIDv7 id and a Clock-sourced created_at, clamped
//     forward to the head's created_at so chain order and
//     (created_at, id) sort order can never disagree;
//  4. computes hash = SHA-256(prev ‖ F(fields)…) and inserts the row.
func Append(ctx context.Context, tx pgx.Tx, clock Clock, e Entry) (db.AuditLog, error) {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(chainLockKey)); err != nil {
		return db.AuditLog{}, fmt.Errorf("audit: acquire chain lock: %w", err)
	}

	q := db.New(tx)
	prevHash := genesisHash
	var headCreatedAt time.Time

	head, err := q.GetAuditLogHead(ctx)
	switch {
	case err == nil:
		prevHash = head.Hash
		headCreatedAt = head.CreatedAt
	case errors.Is(err, pgx.ErrNoRows):
		// Genesis row: prevHash and headCreatedAt keep their zero values.
	default:
		return db.AuditLog{}, fmt.Errorf("audit: read chain head: %w", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return db.AuditLog{}, fmt.Errorf("audit: generate id: %w", err)
	}

	// Truncate to microsecond precision BEFORE hashing: PostgreSQL's
	// timestamptz only stores microseconds, so hashing Go's
	// nanosecond-precision clock value would make VerifyChain's
	// recomputation -- which reads the truncated value back -- disagree
	// with every hash on every row, not just a tampered one.
	createdAt := clock.Now().UTC().Truncate(time.Microsecond)
	if createdAt.Before(headCreatedAt) {
		createdAt = headCreatedAt
	}

	hash := computeHash(prevHash, id, createdAt, e)

	row, err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{
		ID:          id,
		UserID:      pgUUIDFromPtr(e.UserID),
		CommunityID: pgUUIDFromPtr(e.CommunityID),
		Action:      e.Action,
		Entity:      e.Entity,
		EntityID:    pgUUIDFromPtr(e.EntityID),
		Before:      e.Before,
		After:       e.After,
		Ip:          e.IP,
		RequestID:   pgUUIDFromPtr(e.RequestID),
		PrevHash:    prevHash,
		Hash:        hash,
		CreatedAt:   createdAt,
	})
	if err != nil {
		return db.AuditLog{}, fmt.Errorf("audit: insert row: %w", err)
	}
	return row, nil
}

func pgUUIDFromPtr(u *uuid.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *u, Valid: true}
}

func uuidPtrFromPg(u pgtype.UUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	id := uuid.UUID(u.Bytes)
	return &id
}

// entryFromRow reconstructs the Entry a stored db.AuditLog row was
// hashed from, so VerifyChain can recompute and compare.
func entryFromRow(r db.AuditLog) Entry {
	var ip *netip.Addr
	if r.Ip != nil {
		ip = r.Ip
	}
	return Entry{
		UserID:      uuidPtrFromPg(r.UserID),
		CommunityID: uuidPtrFromPg(r.CommunityID),
		Action:      r.Action,
		Entity:      r.Entity,
		EntityID:    uuidPtrFromPg(r.EntityID),
		Before:      r.Before,
		After:       r.After,
		IP:          ip,
		RequestID:   uuidPtrFromPg(r.RequestID),
	}
}
