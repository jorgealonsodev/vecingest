package audit_test

import (
	"testing"
	"time"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/audit"
)

// buildChain fabricates a valid in-memory chain of n rows with the real
// hash algorithm, for pure chain-verification tests with no database.
func buildChain(t *testing.T, n int) []db.AuditLog {
	t.Helper()
	genesis := make([]byte, 32)
	prev := genesis
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := make([]db.AuditLog, 0, n)
	for i := 0; i < n; i++ {
		id := fixedUUID(byte(i + 1))
		createdAt := base.Add(time.Duration(i) * time.Minute)
		entry := audit.Entry{Action: "auth.login", Entity: "session"}
		hash := audit.ComputeHashForTest(prev, id, createdAt, entry)
		rows = append(rows, db.AuditLog{
			ID:        id,
			Action:    entry.Action,
			Entity:    entry.Entity,
			CreatedAt: createdAt,
			PrevHash:  prev,
			Hash:      hash,
		})
		prev = hash
	}
	return rows
}

// db-access-control: audit_log Hash Chain -- "Chain links consecutive
// rows".
func TestVerifyRows_AcceptsAGoodChain(t *testing.T) {
	rows := buildChain(t, 5)
	ok, broken, err := audit.VerifyRowsForTest(rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || broken != nil {
		t.Fatalf("expected a good chain to verify, got ok=%v broken=%+v", ok, broken)
	}
}

// db-access-control: audit_log Hash Chain -- "Tampering breaks the
// chain": pinpoints the first broken link, not just "somewhere".
func TestVerifyRows_PinpointsFirstBrokenLink(t *testing.T) {
	rows := buildChain(t, 5)
	// Tamper with row index 2's action after the fact, as a hostile
	// owner-connection UPDATE would (bypassing app_rw's revoke).
	rows[2].Action = "auth.tampered"

	ok, broken, err := audit.VerifyRowsForTest(rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok || broken == nil {
		t.Fatalf("expected tampering to be detected, got ok=%v broken=%v", ok, broken)
	}
	if broken.Row.ID != rows[2].ID {
		t.Fatalf("VerifyChain flagged row %s, want the tampered row %s", broken.Row.ID, rows[2].ID)
	}
	if broken.Reason != "hash-mismatch" {
		t.Fatalf("Reason = %q, want hash-mismatch", broken.Reason)
	}

	// The row immediately after the tampered one must not falsely
	// report first -- verification must stop at the true first break.
	if broken.Row.ID == rows[3].ID {
		t.Fatalf("VerifyChain flagged the wrong row")
	}
}

func TestVerifyRows_EmptyChainIsValid(t *testing.T) {
	ok, broken, err := audit.VerifyRowsForTest(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || broken != nil {
		t.Fatalf("expected an empty range to verify trivially, got ok=%v broken=%v", ok, broken)
	}
}
