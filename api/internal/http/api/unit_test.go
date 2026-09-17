package api_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/db"
)

// unit-management: Unit Creation Scoped To Community (both scenarios).
func TestUnit_CreationScopedToCommunity(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "unit-create-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Unit Create Community")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)

	ownerID := createUser(t, handlesDB, "unit-create-owner@example.com", false)
	seedUnitOwner(t, handlesDB, communityID, ownerID)
	ownerToken := mintAccessToken(t, deps, handlesDB, ownerID, false)

	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/units", map[string]any{
		"block": "A", "floor": "1", "door": "A", "type": "flat",
	}, map[string]string{"Authorization": "Bearer " + ownerToken})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for an owner creating a unit, got %d body=%v", resp.StatusCode, body)
	}

	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/units", map[string]any{
		"block": "A", "floor": "1", "door": "A", "type": "flat",
	}, map[string]string{"Authorization": "Bearer " + adminToken})
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected the admin's unit creation to succeed, got %d body=%v", resp.StatusCode, body)
	}
	if body["community_id"] != communityID.String() {
		t.Fatalf("expected community_id %s from the route, got %v", communityID, body["community_id"])
	}
}

// unit-management: Unit Uniqueness Per Community.
func TestUnit_UniquenessPerCommunity(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "unit-unique-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Unit Unique Community")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/units", map[string]any{
		"block": "A", "floor": "1", "door": "A", "type": "flat",
	}, auth)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected the first unit's creation to succeed, got %d body=%v", resp.StatusCode, body)
	}

	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/units", map[string]any{
		"block": "A", "floor": "1", "door": "A", "type": "flat",
	}, auth)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for a duplicate block/floor/door, got %d body=%v", resp.StatusCode, body)
	}
}

// unit-management: Participation Coefficient Sum Is A Warning, Not A
// Block (both scenarios).
func TestUnit_ParticipationCoefficientSumIsAWarning(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "unit-coeff-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Unit Coeff Community")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/units", map[string]any{
		"block": "A", "floor": "1", "door": "A", "type": "flat", "participation_coefficient": "97",
	}, auth)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected creation to succeed, got %d body=%v", resp.StatusCode, body)
	}
	warnings, _ := body["warnings"].([]any)
	if len(warnings) == 0 {
		t.Fatalf("expected a coefficient-sum warning at 97, got %v", body)
	}

	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/units", map[string]any{
		"block": "A", "floor": "2", "door": "A", "type": "flat", "participation_coefficient": "3",
	}, auth)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected creation to succeed, got %d body=%v", resp.StatusCode, body)
	}
	if warnings, ok := body["warnings"].([]any); ok && len(warnings) != 0 {
		t.Fatalf("expected no coefficient-sum warning at exactly 100, got %v", body)
	}
}

// unit-management: Unit Member Roles And Deferred board_role (both
// scenarios).
func TestUnit_MemberRolesAndDeferredBoardRole(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "unit-members-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Unit Members Community")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	owner1 := createUser(t, handlesDB, "co-owner-1@example.com", false)
	owner2 := createUser(t, handlesDB, "co-owner-2@example.com", false)

	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/units", map[string]any{
		"block": "B", "floor": "1", "door": "A", "type": "flat",
		"members": []map[string]any{
			{"email": "co-owner-1@example.com", "role": "owner"},
			{"email": "co-owner-2@example.com", "role": "owner"},
		},
	}, auth)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected creation with two co-owners to succeed, got %d body=%v", resp.StatusCode, body)
	}
	members, _ := body["members"].([]any)
	if len(members) != 2 {
		t.Fatalf("expected two independent unit_members rows, got %v", body)
	}

	q := db.New(handlesDB.Write)
	rows, err := q.ListUnitMembersByCommunityID(t.Context(), communityID)
	if err != nil {
		t.Fatalf("list unit members: %v", err)
	}
	found := 0
	for _, r := range rows {
		if r.UserID == owner1 || r.UserID == owner2 {
			found++
			if r.Role != "owner" {
				t.Fatalf("expected role owner, got %q", r.Role)
			}
		}
	}
	if found != 2 {
		t.Fatalf("expected both co-owners persisted independently, found %d", found)
	}

	// no M1 request DTO accepts board_role for write -- huma's own
	// schema validation rejects the unknown property outright (422),
	// an even stronger guarantee than merely ignoring it silently.
	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/units", map[string]any{
		"block": "B", "floor": "2", "door": "A", "type": "flat",
		"members": []map[string]any{
			{"email": "co-owner-1@example.com", "role": "owner", "board_role": "president"},
		},
	}, auth)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 rejecting the unknown board_role property, got %d body=%v", resp.StatusCode, body)
	}
	for _, m := range members {
		mm, _ := m.(map[string]any)
		if _, present := mm["board_role"]; present {
			t.Fatalf("expected no board_role field in any unit member response, got %v", mm)
		}
	}
}

// unit-management: Consent And Notification Fields Captured Per Member
// (both scenarios).
func TestUnit_ConsentAndNotificationFieldsCaptured(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "unit-consent-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Unit Consent Community")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	createUser(t, handlesDB, "no-consent-owner@example.com", false)
	createUser(t, handlesDB, "consenting-owner@example.com", false)

	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/units", map[string]any{
		"block": "C", "floor": "1", "door": "A", "type": "flat",
		"members": []map[string]any{{"email": "no-consent-owner@example.com", "role": "owner"}},
	}, auth)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected creation to succeed, got %d body=%v", resp.StatusCode, body)
	}
	members, _ := body["members"].([]any)
	member, _ := members[0].(map[string]any)
	if v, present := member["electronic_notifications_consent_at"]; present && v != nil {
		t.Fatalf("expected no consent timestamp without explicit consent, got %v", v)
	}

	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/units", map[string]any{
		"block": "C", "floor": "2", "door": "A", "type": "flat",
		"members": []map[string]any{{
			"email": "consenting-owner@example.com", "role": "owner",
			"electronic_notifications_consent": true, "consent_text_version": "v1",
		}},
	}, auth)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected creation to succeed, got %d body=%v", resp.StatusCode, body)
	}
	members, _ = body["members"].([]any)
	member, _ = members[0].(map[string]any)
	if v, present := member["electronic_notifications_consent_at"]; !present || v == nil {
		t.Fatalf("expected a consent timestamp when consent is accepted, got %v", member)
	}
	if member["consent_text_version"] != "v1" {
		t.Fatalf("expected consent_text_version v1 persisted, got %v", member["consent_text_version"])
	}
}

// unit-management: Unit Member Management Scoped To Community (negative
// half: member of another community cannot list members).
func TestUnitMembers_ManagementScopedToCommunity(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "unit-scope-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Unit Scope Community")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/units", map[string]any{
		"block": "D", "floor": "1", "door": "A", "type": "flat",
	}, auth)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected creation to succeed, got %d body=%v", resp.StatusCode, body)
	}
	unitID, _ := body["id"].(string)

	otherOfficeID, _ := seedOfficeWithAdmin(t, handlesDB, "unit-scope-foreign-admin@example.com")
	otherCommunityID := seedCommunity(t, handlesDB, otherOfficeID, "Foreign Unit Scope Community")
	tenantID := createUser(t, handlesDB, "foreign-tenant@example.com", false)
	seedUnitOwner(t, handlesDB, otherCommunityID, tenantID)
	tenantToken := mintAccessToken(t, deps, handlesDB, tenantID, false)

	resp, _ = doJSON(t, client, http.MethodGet, srv.URL+"/v1/units/"+unitID+"/members", nil, map[string]string{
		"Authorization": "Bearer " + tenantToken,
	})
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 403 or 404 for a caller with no membership tied to the unit's community, got %d", resp.StatusCode)
	}

	// same-community owner CAN list (any role tied to the community).
	ownerID := createUser(t, handlesDB, "same-community-owner@example.com", false)
	seedUnitOwner(t, handlesDB, communityID, ownerID)
	ownerToken := mintAccessToken(t, deps, handlesDB, ownerID, false)

	resp, listBody := doJSON(t, client, http.MethodGet, srv.URL+"/v1/units/"+unitID+"/members", nil, map[string]string{
		"Authorization": "Bearer " + ownerToken,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for a caller tied to the SAME community, got %d body=%v", resp.StatusCode, listBody)
	}
}

// unit-management: Unit Member Management Scoped To Community --
// PATCH persists (positive half). Without this, a PATCH that echoes the
// request without persisting still passes a negative-only suite (the
// exact Phase 3 gap this phase's instructions call out).
func TestUnitMembers_UpdatePersists(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "unit-patch-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Unit Patch Community")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	createUser(t, handlesDB, "patch-owner@example.com", false)
	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/units", map[string]any{
		"block": "E", "floor": "1", "door": "A", "type": "flat",
		"members": []map[string]any{{"email": "patch-owner@example.com", "role": "owner"}},
	}, auth)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected creation to succeed, got %d body=%v", resp.StatusCode, body)
	}
	unitID, _ := body["id"].(string)
	members, _ := body["members"].([]any)
	member, _ := members[0].(map[string]any)
	memberID, _ := member["id"].(string)

	resp, body = doJSON(t, client, http.MethodPatch, srv.URL+"/v1/units/"+unitID+"/members/"+memberID, map[string]any{
		"notification_address": "Calle Falsa 123",
	}, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 patching the member, got %d body=%v", resp.StatusCode, body)
	}

	// Persisted, not merely echoed back in the PATCH response: re-read
	// via a SEPARATE GET.
	resp, listBody := doJSON(t, client, http.MethodGet, srv.URL+"/v1/units/"+unitID+"/members", nil, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 re-reading the members, got %d body=%v", resp.StatusCode, listBody)
	}
	reread, _ := listBody["members"].([]any)
	found := false
	for _, m := range reread {
		mm, _ := m.(map[string]any)
		if mm["id"] == memberID {
			found = true
			if mm["notification_address"] != "Calle Falsa 123" {
				t.Fatalf("expected the new notification_address to survive a re-read, got %v", mm["notification_address"])
			}
		}
	}
	if !found {
		t.Fatalf("expected to find the patched member on re-read, got %v", listBody)
	}
}

// unit-management: Unit Member Management Scoped To Community -- DELETE
// soft-deletes and the member no longer appears on a re-read.
func TestUnitMembers_DeletePersists(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "unit-delete-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Unit Delete Community")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	createUser(t, handlesDB, "delete-owner@example.com", false)
	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/units", map[string]any{
		"block": "F", "floor": "1", "door": "A", "type": "flat",
		"members": []map[string]any{{"email": "delete-owner@example.com", "role": "owner"}},
	}, auth)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected creation to succeed, got %d body=%v", resp.StatusCode, body)
	}
	unitID, _ := body["id"].(string)
	members, _ := body["members"].([]any)
	member, _ := members[0].(map[string]any)
	memberID, _ := member["id"].(string)

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/v1/units/"+unitID+"/members/"+memberID, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("delete member: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 204 or 200 deleting the member, got %d", resp.StatusCode)
	}

	resp, listBody := doJSON(t, client, http.MethodGet, srv.URL+"/v1/units/"+unitID+"/members", nil, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 re-reading the members, got %d body=%v", resp.StatusCode, listBody)
	}
	reread, _ := listBody["members"].([]any)
	for _, m := range reread {
		mm, _ := m.(map[string]any)
		if mm["id"] == memberID {
			t.Fatalf("expected the deleted member to be absent from a re-read, got %v", listBody)
		}
	}

	q := db.New(handlesDB.Write)
	deletedMemberID := uuid.MustParse(memberID)
	var deletedAt any
	if err := handlesDB.Write.QueryRow(t.Context(), "SELECT deleted_at FROM unit_members WHERE id = $1", deletedMemberID).Scan(&deletedAt); err != nil {
		t.Fatalf("query deleted_at: %v", err)
	}
	if deletedAt == nil {
		t.Fatalf("expected deleted_at to be set (soft delete), got nil")
	}
	_ = q
}
