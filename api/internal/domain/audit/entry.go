// Package audit implements the audit_log hash chain (D-O; db-access-control:
// audit_log Hash Chain). The chain is computed here, in Go, never inside
// a database trigger: PRD §6.1 names an operator with database access as
// the threat, so a chain whose construction lived entirely inside the
// database would be verifiable only by the same component the threat
// model distrusts. Append is the audit_log table's only writer -- the
// Semgrep rule .semgrep/single-writer-audit-log.yml fails the build for
// any other call site of db.Queries.InsertAuditLog.
package audit

import (
	"net/netip"
	"time"

	"github.com/google/uuid"
)

// Entry is the caller-supplied content of one audit_log row. ID and
// CreatedAt are NOT part of it: Append assigns both itself (a UUIDv7 and
// the injected Clock's time, clamped to the chain head) while holding
// the chain's advisory lock, which is what keeps chain order and
// (created_at, id) sort order from ever being able to disagree.
type Entry struct {
	UserID      *uuid.UUID
	CommunityID *uuid.UUID
	Action      string
	Entity      string
	EntityID    *uuid.UUID
	// Before and After are raw JSON (or nil for SQL NULL). ComputeHash
	// canonicalizes them (sorted keys, no insignificant whitespace)
	// before hashing, so callers never need to pre-sort their own JSON.
	Before []byte
	After  []byte
	IP     *netip.Addr
	// RequestID is the resolved X-Request-Id (D-H step 1), when the
	// caller has one.
	RequestID *uuid.UUID
}

// Clock abstracts time.Now() so Append's chain-order tests never depend
// on the wall clock.
type Clock interface{ Now() time.Time }
