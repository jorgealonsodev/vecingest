package db_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/testhelpers"
)

// authz-membership / m1-communities design D-5: sqlc generates queries
// for every M1 tenant table, each taking its tenant column as a bound
// parameter (task 1.6/1.7). This test exercises the full membership
// chain a scoped route resolves through: an office admin, a community
// under that office, a unit in that community, and a tenant member of
// that unit -- proving every generated query compiles against
// decimal.Decimal/uuid.UUID/time.Time (never float64) and actually reads
// back what it wrote.
func TestM1Queries_FullMembershipChain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}
	handles, _ := testhelpers.AppRWHandles(t)
	ctx := t.Context()
	q := db.New(handles.Write)

	adminUser, err := q.InsertUser(ctx, db.InsertUserParams{
		ID:           uuid.New(),
		Email:        "admin@example.com",
		PasswordHash: "hash",
		Name:         "Admin User",
		Locale:       "es",
	})
	if err != nil {
		t.Fatalf("InsertUser (admin): %v", err)
	}
	tenantUser, err := q.InsertUser(ctx, db.InsertUserParams{
		ID:           uuid.New(),
		Email:        "tenant@example.com",
		PasswordHash: "hash",
		Name:         "Tenant User",
		Locale:       "es",
	})
	if err != nil {
		t.Fatalf("InsertUser (tenant): %v", err)
	}

	office, err := q.InsertOffice(ctx, db.InsertOfficeParams{
		ID:   uuid.New(),
		Name: "Acme Admin",
		Cif:  "B12345678",
	})
	if err != nil {
		t.Fatalf("InsertOffice: %v", err)
	}
	if office.ID == uuid.Nil {
		t.Fatalf("expected a non-nil office id")
	}

	gotOffice, err := q.GetOfficeByID(ctx, office.ID)
	if err != nil {
		t.Fatalf("GetOfficeByID: %v", err)
	}
	if gotOffice.Name != "Acme Admin" {
		t.Fatalf("expected office name %q, got %q", "Acme Admin", gotOffice.Name)
	}

	if _, err := q.InsertOfficeMember(ctx, db.InsertOfficeMemberParams{
		ID:       uuid.New(),
		OfficeID: office.ID,
		UserID:   adminUser.ID,
		Role:     "admin",
	}); err != nil {
		t.Fatalf("InsertOfficeMember: %v", err)
	}

	officeRole, err := q.GetOfficeMemberByOfficeAndUser(ctx, db.GetOfficeMemberByOfficeAndUserParams{
		OfficeID: office.ID,
		UserID:   adminUser.ID,
	})
	if err != nil {
		t.Fatalf("GetOfficeMemberByOfficeAndUser: %v", err)
	}
	if officeRole.Role != "admin" {
		t.Fatalf("expected role %q, got %q", "admin", officeRole.Role)
	}

	community, err := q.InsertCommunity(ctx, db.InsertCommunityParams{
		ID:       uuid.New(),
		OfficeID: office.ID,
		Name:     "Comunidad Mayor 3",
	})
	if err != nil {
		t.Fatalf("InsertCommunity: %v", err)
	}

	// The admin reaches this community via office_members, never a
	// caller-supplied office id (design D-4).
	viaOfficeRole, err := q.ResolveCommunityRoleViaOffice(ctx, db.ResolveCommunityRoleViaOfficeParams{
		CommunityID: community.ID,
		UserID:      adminUser.ID,
	})
	if err != nil {
		t.Fatalf("ResolveCommunityRoleViaOffice: %v", err)
	}
	if viaOfficeRole != "admin" {
		t.Fatalf("expected resolved role %q, got %q", "admin", viaOfficeRole)
	}

	unit, err := q.InsertUnit(ctx, db.InsertUnitParams{
		ID:          uuid.New(),
		CommunityID: community.ID,
		Type:        "flat",
	})
	if err != nil {
		t.Fatalf("InsertUnit: %v", err)
	}

	if _, err := q.InsertUnitMember(ctx, db.InsertUnitMemberParams{
		ID:          uuid.New(),
		UnitID:      unit.ID,
		CommunityID: community.ID,
		UserID:      tenantUser.ID,
		Role:        "tenant",
		Tenure:      "full_owner",
	}); err != nil {
		t.Fatalf("InsertUnitMember: %v", err)
	}

	viaUnitRole, err := q.ResolveCommunityRoleViaUnit(ctx, db.ResolveCommunityRoleViaUnitParams{
		CommunityID: community.ID,
		UserID:      tenantUser.ID,
	})
	if err != nil {
		t.Fatalf("ResolveCommunityRoleViaUnit: %v", err)
	}
	if viaUnitRole != "tenant" {
		t.Fatalf("expected resolved role %q, got %q", "tenant", viaUnitRole)
	}

	// The admin has no unit_members row, so the unit-resolution path
	// must report no rows -- a foreign caller resolves to nothing
	// (authz-membership: "Foreign resource id resolves to no membership").
	if _, err := q.ResolveCommunityRoleViaUnit(ctx, db.ResolveCommunityRoleViaUnitParams{
		CommunityID: community.ID,
		UserID:      adminUser.ID,
	}); err == nil {
		t.Fatalf("expected no rows resolving the admin via unit_members")
	}

	invitation, err := q.InsertInvitation(ctx, db.InsertInvitationParams{
		ID:            uuid.New(),
		CommunityID:   community.ID,
		Role:          "owner",
		TokenHash:     []byte("token-hash-32-bytes-of-fake-sha256!!"),
		ShortCodeHash: []byte("short-code-hash-32-bytes-fake-sha!!"),
		ExpiresAt:     time.Now().Add(14 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("InsertInvitation: %v", err)
	}

	invitationCommunityID, err := q.GetInvitationCommunityID(ctx, invitation.ID)
	if err != nil {
		t.Fatalf("GetInvitationCommunityID: %v", err)
	}
	if invitationCommunityID != community.ID {
		t.Fatalf("expected invitation community id %s, got %s", community.ID, invitationCommunityID)
	}
}
