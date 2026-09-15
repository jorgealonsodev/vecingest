package authz

import (
	"strings"
	"testing"
)

// authz-membership / design D-3: Allowlist hygiene (task 1.22). Every
// PublicOperations entry has a non-empty Reason, and recovery from a
// production false positive is an allowlist entry plus an issue, never
// disabling the assertion -- so no entry may sit under a tenant-scoped
// prefix, where it would silently defeat A3.
func TestPublicOperations_EveryEntryHasAReason(t *testing.T) {
	if len(PublicOperations) == 0 {
		t.Fatalf("expected PublicOperations to be non-empty")
	}
	for _, e := range PublicOperations {
		if strings.TrimSpace(e.Reason) == "" {
			t.Fatalf("allowlist entry %s %s has no Reason", e.Method, e.Path)
		}
	}
}

func TestPublicOperations_NoEntryUnderAScopedPrefix(t *testing.T) {
	forbidden := []string{"/v1/communities/", "/v1/units/", "/v1/offices/"}
	for _, e := range PublicOperations {
		for _, prefix := range forbidden {
			if strings.HasPrefix(e.Path, prefix) {
				t.Fatalf("allowlist entry %s %s sits under the forbidden scoped prefix %q", e.Method, e.Path, prefix)
			}
		}
	}
}
