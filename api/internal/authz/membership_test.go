package authz

import (
	"testing"

	"github.com/google/uuid"
)

// authz-membership: Zero-value membership rejected (task 1.10). A
// Membership{} zero value must fail every accessor -- design D-1: "an
// unexported field ... panics on the zero value -- unreachable by
// construction".
func TestMembership_ZeroValueAccessorsPanic(t *testing.T) {
	tests := []struct {
		name string
		call func(Membership)
	}{
		{"UserID", func(m Membership) { m.UserID() }},
		{"Role", func(m Membership) { m.Role() }},
		{"CommunityID", func(m Membership) { m.CommunityID() }},
		{"OfficeID", func(m Membership) { m.OfficeID() }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("expected %s() on a zero-value Membership to panic", tc.name)
				}
			}()
			tc.call(Membership{})
		})
	}
}

// Triangulation: a Membership resolved with valid=true, the community
// kind, and a real role must return exactly those values from every
// accessor with no panic -- proving the panic above is a REAL validity
// gate, not an accessor that always panics.
func TestMembership_ValidCommunityMembershipAccessorsReturnResolvedValues(t *testing.T) {
	communityID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	userID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	m := Membership{
		userID: userID,
		scope:  scope{kind: KindCommunity, id: communityID},
		role:   RoleOwner,
		valid:  true,
	}

	if got := m.UserID(); got != userID {
		t.Fatalf("expected UserID() %s, got %s", userID, got)
	}
	if got := m.Role(); got != RoleOwner {
		t.Fatalf("expected Role() %s, got %s", RoleOwner, got)
	}
	if got := m.CommunityID(); got != communityID {
		t.Fatalf("expected CommunityID() %s, got %s", communityID, got)
	}

	defer func() {
		if recover() == nil {
			t.Fatalf("expected OfficeID() on a community-scoped Membership to panic (wrong scope kind)")
		}
	}()
	m.OfficeID()
}
