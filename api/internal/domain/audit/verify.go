package audit

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/jorgealonsodev/vecingest/internal/db"
)

// BrokenLink describes the first row VerifyChain found inconsistent.
type BrokenLink struct {
	Row db.AuditLog
	// Reason is "hash-mismatch" when the row's own recomputed hash does
	// not match its stored hash (tampered content), or
	// "prev-hash-mismatch" when its prev_hash does not equal the
	// preceding row's hash within the verified range (a missing or
	// reordered row).
	Reason string
}

// VerifyChain recomputes and checks every row's hash in [from, to]
// against the partitioned parent (db-access-control: audit_log Hash
// Chain). It is a domain function, not a CLI: the `vecingest
// verify-chain` subcommand is an M7 gate item scoped to votes/meetings,
// and daily anchoring is an M1+ periodic job (D-O) -- both are thin
// wrappers over this function once their own scope exists.
//
// VerifyChain only asserts internal consistency of the returned range:
// each row's stored hash must equal its own recomputed hash, and (for
// every row after the first in the range) its prev_hash must equal the
// immediately preceding row's hash. It does not reach outside [from, to]
// to validate the very first row's prev_hash against a true external
// predecessor -- a caller wanting whole-chain verification should widen
// the range to genesis.
func VerifyChain(ctx context.Context, rdb db.ReadDB, from, to time.Time) (ok bool, broken *BrokenLink, err error) {
	q := db.New(rdb)
	rows, err := q.ListAuditLogRange(ctx, db.ListAuditLogRangeParams{CreatedAt: from, CreatedAt_2: to})
	if err != nil {
		return false, nil, fmt.Errorf("audit: list range: %w", err)
	}
	return verifyRows(rows)
}

func verifyRows(rows []db.AuditLog) (ok bool, broken *BrokenLink, err error) {
	var prevHash []byte
	for i, r := range rows {
		if i > 0 && !bytes.Equal(r.PrevHash, prevHash) {
			return false, &BrokenLink{Row: r, Reason: "prev-hash-mismatch"}, nil
		}
		want := computeHash(r.PrevHash, r.ID, r.CreatedAt, entryFromRow(r))
		if !bytes.Equal(want, r.Hash) {
			return false, &BrokenLink{Row: r, Reason: "hash-mismatch"}, nil
		}
		prevHash = r.Hash
	}
	return true, nil, nil
}
