package audit_test

import (
	"encoding/hex"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/domain/audit"
)

var update = flag.Bool("update", false, "update golden files (make test-golden-update)")

func goldenPath(name string) string {
	return filepath.Join("testdata", name+".golden")
}

// checkGolden compares got (hex-encoded) against the golden file, or
// writes it when -update is passed. This is the ONLY sanctioned way to
// change a golden hash (Makefile's test-golden-update target).
func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := goldenPath(name)
	gotHex := hex.EncodeToString(got)

	if *update {
		if err := os.WriteFile(path, []byte(gotHex+"\n"), 0o600); err != nil {
			t.Fatalf("failed to write golden file %s: %v", path, err)
		}
		return
	}

	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read golden file %s (run `make test-golden-update` first): %v", path, err)
	}
	want := string(wantBytes)
	// trim the trailing newline written above
	for len(want) > 0 && (want[len(want)-1] == '\n' || want[len(want)-1] == '\r') {
		want = want[:len(want)-1]
	}
	if gotHex != want {
		t.Errorf("hash mismatch for %s:\n got:  %s\n want: %s", name, gotHex, want)
	}
}

func fixedUUID(seed byte) uuid.UUID {
	var u uuid.UUID
	for i := range u {
		u[i] = seed
	}
	return u
}

// db-access-control: audit_log Hash Chain -- "Chain links consecutive
// rows": hash is stable for a fixed input and derived from prev ‖
// content.
func TestComputeHash_StableForFixedInput(t *testing.T) {
	id := fixedUUID(0x01)
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	userID := fixedUUID(0x02)
	entry := audit.Entry{
		UserID: &userID,
		Action: "auth.login",
		Entity: "session",
	}
	genesis := make([]byte, 32)

	got1 := audit.ComputeHashForTest(genesis, id, createdAt, entry)
	got2 := audit.ComputeHashForTest(genesis, id, createdAt, entry)
	if hex.EncodeToString(got1) != hex.EncodeToString(got2) {
		t.Fatalf("ComputeHash is not stable for identical input")
	}
	checkGolden(t, "stable_fixed_input", got1)
}

// A one-field change (different action) must change the hash.
func TestComputeHash_OneFieldChangeChangesHash(t *testing.T) {
	id := fixedUUID(0x01)
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	genesis := make([]byte, 32)

	base := audit.Entry{Action: "auth.login", Entity: "session"}
	changed := audit.Entry{Action: "auth.logout", Entity: "session"}

	h1 := audit.ComputeHashForTest(genesis, id, createdAt, base)
	h2 := audit.ComputeHashForTest(genesis, id, createdAt, changed)
	if hex.EncodeToString(h1) == hex.EncodeToString(h2) {
		t.Fatalf("expected different hashes for different action, got identical")
	}
}

// db-access-control: audit_log Hash Chain -- "Tampering breaks the
// chain", field-boundary variant: moving a character from action to
// entity must change the hash even though the concatenation is
// byte-identical without length prefixes.
func TestComputeHash_FieldBoundaryShiftChangesHash(t *testing.T) {
	id := fixedUUID(0x01)
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	genesis := make([]byte, 32)

	a := audit.Entry{Action: "auth.logi", Entity: "nsession"}
	b := audit.Entry{Action: "auth.login", Entity: "session"}

	ha := audit.ComputeHashForTest(genesis, id, createdAt, a)
	hb := audit.ComputeHashForTest(genesis, id, createdAt, b)
	if hex.EncodeToString(ha) == hex.EncodeToString(hb) {
		t.Fatalf("field-boundary shift produced identical hashes -- length prefixes are not working")
	}
	checkGolden(t, "field_boundary_a", ha)
	checkGolden(t, "field_boundary_b", hb)
}

// NULL and "" (empty string / zero-length JSON) MUST hash differently.
func TestComputeHash_NullAndEmptyStringDifferently(t *testing.T) {
	id := fixedUUID(0x01)
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	genesis := make([]byte, 32)

	withNilBefore := audit.Entry{Action: "x", Entity: "y", Before: nil}
	withEmptyBefore := audit.Entry{Action: "x", Entity: "y", Before: []byte("{}")}

	h1 := audit.ComputeHashForTest(genesis, id, createdAt, withNilBefore)
	h2 := audit.ComputeHashForTest(genesis, id, createdAt, withEmptyBefore)
	if hex.EncodeToString(h1) == hex.EncodeToString(h2) {
		t.Fatalf("NULL before and {} before must hash differently, got identical")
	}
}

// Canonical JSON: key order in the input must not affect the hash.
func TestComputeHash_JSONKeyOrderIsCanonicalized(t *testing.T) {
	id := fixedUUID(0x01)
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	genesis := make([]byte, 32)

	e1 := audit.Entry{Action: "x", Entity: "y", After: []byte(`{"a":1,"b":2}`)}
	e2 := audit.Entry{Action: "x", Entity: "y", After: []byte(`{"b":2,"a":1}`)}

	h1 := audit.ComputeHashForTest(genesis, id, createdAt, e1)
	h2 := audit.ComputeHashForTest(genesis, id, createdAt, e2)
	if hex.EncodeToString(h1) != hex.EncodeToString(h2) {
		t.Fatalf("expected canonical JSON to make key order irrelevant, got different hashes")
	}
}

// Different prev hashes chain differently -- the whole point of "prev".
func TestComputeHash_DependsOnPrevHash(t *testing.T) {
	id := fixedUUID(0x01)
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entry := audit.Entry{Action: "x", Entity: "y"}

	genesis := make([]byte, 32)
	other := make([]byte, 32)
	other[0] = 0xAB

	h1 := audit.ComputeHashForTest(genesis, id, createdAt, entry)
	h2 := audit.ComputeHashForTest(other, id, createdAt, entry)
	if hex.EncodeToString(h1) == hex.EncodeToString(h2) {
		t.Fatalf("expected different prev hashes to produce different chained hashes")
	}
}
