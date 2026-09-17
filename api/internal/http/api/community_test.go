package api_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/db"
)

// seedCommunity inserts a community directly under officeID (bypassing
// the HTTP creation flow this file also tests), for tests whose focus is
// a DIFFERENT endpoint that merely needs an existing community as a
// precondition.
func seedCommunity(t *testing.T, handlesDB db.Handles, officeID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	q := db.New(handlesDB.Write)
	community, err := q.InsertCommunity(t.Context(), db.InsertCommunityParams{
		ID:       uuid.New(),
		OfficeID: officeID,
		Name:     name,
	})
	if err != nil {
		t.Fatalf("insert community: %v", err)
	}
	return community.ID
}

// seedUnitOwner inserts a unit and an "owner" unit_members row for
// userID in communityID, for tests exercising owner/tenant (unit-scoped)
// community access.
func seedUnitOwner(t *testing.T, handlesDB db.Handles, communityID, userID uuid.UUID) {
	t.Helper()
	q := db.New(handlesDB.Write)
	unit, err := q.InsertUnit(t.Context(), db.InsertUnitParams{
		ID:          uuid.New(),
		CommunityID: communityID,
		Type:        "flat",
	})
	if err != nil {
		t.Fatalf("insert unit: %v", err)
	}
	if _, err := q.InsertUnitMember(t.Context(), db.InsertUnitMemberParams{
		ID:          uuid.New(),
		UnitID:      unit.ID,
		CommunityID: communityID,
		UserID:      userID,
		Role:        "owner",
		Tenure:      "full_owner",
	}); err != nil {
		t.Fatalf("insert unit member: %v", err)
	}
}

// community-management: Community Creation Restricted To Admin, Scoped
// To Office (both scenarios).
func TestCommunity_CreationRestrictedToAdminScopedToOffice(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "community-admin@example.com")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)

	staffID := createUser(t, handlesDB, "community-admin-staff@example.com", false)
	q := db.New(handlesDB.Write)
	if _, err := q.InsertOfficeMember(t.Context(), db.InsertOfficeMemberParams{
		ID: uuid.New(), OfficeID: officeID, UserID: staffID, Role: "admin_staff",
	}); err != nil {
		t.Fatalf("insert office member: %v", err)
	}
	staffToken := mintAccessToken(t, deps, handlesDB, staffID, false)

	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities", map[string]any{
		"office_id": officeID.String(), "name": "Denied Community",
	}, map[string]string{"Authorization": "Bearer " + staffToken})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for admin_staff, got %d body=%v", resp.StatusCode, body)
	}

	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities", map[string]any{
		"office_id": officeID.String(), "name": "Allowed Community",
	}, map[string]string{"Authorization": "Bearer " + adminToken})
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected the admin's community creation to succeed, got %d body=%v", resp.StatusCode, body)
	}
	if body["office_id"] != officeID.String() {
		t.Fatalf("expected office_id %s from the resolved membership, got %v", officeID, body["office_id"])
	}
}

// community-management: Community Read And List Scoped By Membership
// (both scenarios).
func TestCommunity_ReadAndListScopedByMembership(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, _ := seedOfficeWithAdmin(t, handlesDB, "owner-community-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Owner's Community")
	otherOfficeID, _ := seedOfficeWithAdmin(t, handlesDB, "foreign-community-admin@example.com")
	foreignCommunityID := seedCommunity(t, handlesDB, otherOfficeID, "Foreign Community")

	ownerID := createUser(t, handlesDB, "unit-owner@example.com", false)
	seedUnitOwner(t, handlesDB, communityID, ownerID)
	ownerToken := mintAccessToken(t, deps, handlesDB, ownerID, false)

	resp, body := doJSON(t, client, http.MethodGet, srv.URL+"/v1/communities", nil, map[string]string{
		"Authorization": "Bearer " + ownerToken,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%v", resp.StatusCode, body)
	}
	communities, _ := body["communities"].([]any)
	if len(communities) != 1 {
		t.Fatalf("expected exactly one community (the owner's own), got %v", body)
	}
	first, _ := communities[0].(map[string]any)
	if first["id"] != communityID.String() {
		t.Fatalf("expected community %s, got %v", communityID, first["id"])
	}

	resp, _ = doJSON(t, client, http.MethodGet, srv.URL+"/v1/communities/"+foreignCommunityID.String(), nil, map[string]string{
		"Authorization": "Bearer " + ownerToken,
	})
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 403 or 404 for a foreign community, got %d", resp.StatusCode)
	}
}

// community-management: Community Update Restricted To Office Roles.
func TestCommunity_UpdateRestrictedToOfficeRoles(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, _ := seedOfficeWithAdmin(t, handlesDB, "update-community-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Community To Update")

	ownerID := createUser(t, handlesDB, "update-community-owner@example.com", false)
	seedUnitOwner(t, handlesDB, communityID, ownerID)
	ownerToken := mintAccessToken(t, deps, handlesDB, ownerID, false)

	resp, body := doJSON(t, client, http.MethodPatch, srv.URL+"/v1/communities/"+communityID.String(), map[string]any{
		"name": "Hijacked Name",
	}, map[string]string{"Authorization": "Bearer " + ownerToken})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for an owner updating community fields, got %d body=%v", resp.StatusCode, body)
	}
}

// community-management: Community Update Restricted To Office Roles
// (Scenario: Admin of the owning office updates community fields).
//
// This covers the POSITIVE half of the requirement. Without it, a
// completely broken UpdateCommunity -- one that rejects or silently
// discards every write -- still passes the whole community suite, since
// the only other PATCH in this file asserts a 403. That was verified by
// planting exactly that break and watching all five tests stay green.
func TestCommunity_UpdateByOfficeAdminPersists(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "patching-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Original Name")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	resp, body := doJSON(t, client, http.MethodPatch, srv.URL+"/v1/communities/"+communityID.String(), map[string]any{
		"name": "Renamed By Admin",
	}, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for an office admin updating their own community, got %d body=%v", resp.StatusCode, body)
	}

	// Persisted, not merely echoed back in the PATCH response.
	resp, body = doJSON(t, client, http.MethodGet, srv.URL+"/v1/communities/"+communityID.String(), nil, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 re-reading the community, got %d body=%v", resp.StatusCode, body)
	}
	if body["name"] != "Renamed By Admin" {
		t.Fatalf("expected the new name to survive a re-read, got %v", body["name"])
	}
}

// community-management: Legal And Descriptive Fields Persisted Per §7.3
// (both scenarios: created without a parent, linked to a parent).
func TestCommunity_LegalAndDescriptiveFieldsPersisted(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "legal-fields-admin@example.com")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)

	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities", map[string]any{
		"office_id": officeID.String(), "name": "No Parent Community",
	}, map[string]string{"Authorization": "Bearer " + adminToken})
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected creation to succeed, got %d body=%v", resp.StatusCode, body)
	}
	if v, ok := body["parent_community_id"]; ok && v != nil {
		t.Fatalf("expected no parent_community_id, got %v", v)
	}

	parentID, _ := body["id"].(string)
	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities", map[string]any{
		"office_id": officeID.String(), "name": "Child Community", "parent_community_id": parentID,
	}, map[string]string{"Authorization": "Bearer " + adminToken})
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected creation to succeed, got %d body=%v", resp.StatusCode, body)
	}
	childID, _ := body["id"].(string)
	if body["parent_community_id"] != parentID {
		t.Fatalf("expected the link to be persisted at creation time, got %v", body["parent_community_id"])
	}

	resp, body = doJSON(t, client, http.MethodGet, srv.URL+"/v1/communities/"+childID, nil, map[string]string{
		"Authorization": "Bearer " + adminToken,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%v", resp.StatusCode, body)
	}
	if body["parent_community_id"] != parentID {
		t.Fatalf("expected the link readable on the child's detail response, got %v", body["parent_community_id"])
	}
}

// community-management: Community Detail Excludes Cross-Milestone
// Aggregates.
func TestCommunity_DetailExcludesCrossMilestoneAggregates(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "aggregate-admin@example.com")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	communityID := seedCommunity(t, handlesDB, officeID, "Aggregate-Free Community")

	resp, body := doJSON(t, client, http.MethodGet, srv.URL+"/v1/communities/"+communityID.String(), nil, map[string]string{
		"Authorization": "Bearer " + adminToken,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%v", resp.StatusCode, body)
	}
	for _, forbidden := range []string{"reserve_fund_compliance", "quorum", "balance"} {
		if _, present := body[forbidden]; present {
			t.Fatalf("expected no %q field in the community detail response, got %v", forbidden, body)
		}
	}
	if _, ok := body["unit_count"]; !ok {
		t.Fatalf("expected a unit_count field, got %v", body)
	}
	if _, ok := body["office_member_count"]; !ok {
		t.Fatalf("expected an office_member_count field, got %v", body)
	}
}
