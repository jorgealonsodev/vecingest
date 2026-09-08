package audit

import (
	"time"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/db"
)

// ComputeHashForTest exposes the unexported computeHash to the external
// audit_test package, per this project's go-testing convention of
// testing behavior through the smallest public boundary while still
// keeping the hash algorithm itself unexported production API.
func ComputeHashForTest(prev []byte, id uuid.UUID, createdAt time.Time, e Entry) []byte {
	return computeHash(prev, id, createdAt, e)
}

// VerifyRowsForTest exposes the unexported verifyRows so audit_test can
// exercise chain-consistency checking against fabricated rows without a
// database.
func VerifyRowsForTest(rows []db.AuditLog) (bool, *BrokenLink, error) {
	return verifyRows(rows)
}
