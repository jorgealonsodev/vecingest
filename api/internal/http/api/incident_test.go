package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/db"
)

func TestIncident_CreateAuthorizationValidationAndSafeProjection(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)
	q := db.New(handlesDB.Write)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "incident-create-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Incident Community")
	foreignOfficeID, _ := seedOfficeWithAdmin(t, handlesDB, "incident-foreign-admin@example.com")
	foreignCommunityID := seedCommunity(t, handlesDB, foreignOfficeID, "Foreign Incident Community")
	adminStaffID := createUser(t, handlesDB, "incident-create-staff@example.com", false)
	if _, err := q.InsertOfficeMember(t.Context(), db.InsertOfficeMemberParams{
		ID: uuid.New(), OfficeID: officeID, UserID: adminStaffID, Role: "admin_staff",
	}); err != nil {
		t.Fatalf("insert admin_staff: %v", err)
	}
	seedActiveMFA(t, handlesDB, adminStaffID)
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	staffToken := mintAccessToken(t, deps, handlesDB, adminStaffID, false)

	unitID := seedUnit(t, handlesDB, communityID)
	otherUnitID := seedUnit(t, handlesDB, communityID)
	expiredUnitID := seedUnit(t, handlesDB, communityID)
	futureUnitID := seedUnit(t, handlesDB, communityID)
	deletedUnitID := seedUnit(t, handlesDB, communityID)
	foreignUnitID := seedUnit(t, handlesDB, foreignCommunityID)
	if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE units SET deleted_at = now() WHERE id = $1", deletedUnitID); err != nil {
		t.Fatalf("soft-delete unit fixture: %v", err)
	}

	ownerID := createUser(t, handlesDB, "incident-owner@example.com", false)
	tenantID := createUser(t, handlesDB, "incident-tenant@example.com", false)
	otherTenantID := createUser(t, handlesDB, "incident-other-tenant@example.com", false)
	expiredTenantID := createUser(t, handlesDB, "incident-expired-tenant@example.com", false)
	futureTenantID := createUser(t, handlesDB, "incident-future-tenant@example.com", false)
	deletedUnitTenantID := createUser(t, handlesDB, "incident-deleted-unit-tenant@example.com", false)
	mixedRoleUserID := createUser(t, handlesDB, "incident-mixed-role@example.com", false)
	activeMixedRoleUserID := createUser(t, handlesDB, "incident-active-mixed-role@example.com", false)
	seedIncidentUnitMember(t, handlesDB, unitID, communityID, ownerID, "owner")
	seedIncidentUnitMember(t, handlesDB, unitID, communityID, tenantID, "tenant")
	seedIncidentUnitMember(t, handlesDB, otherUnitID, communityID, otherTenantID, "tenant")
	expiredMemberID := seedIncidentUnitMember(t, handlesDB, expiredUnitID, communityID, expiredTenantID, "tenant")
	futureMemberID := seedIncidentUnitMember(t, handlesDB, futureUnitID, communityID, futureTenantID, "tenant")
	seedIncidentUnitMember(t, handlesDB, deletedUnitID, communityID, deletedUnitTenantID, "tenant")
	mixedExpiredOwnerID := seedIncidentUnitMember(t, handlesDB, expiredUnitID, communityID, mixedRoleUserID, "owner")
	mixedFutureOwnerID := seedIncidentUnitMember(t, handlesDB, futureUnitID, communityID, mixedRoleUserID, "owner")
	seedIncidentUnitMember(t, handlesDB, deletedUnitID, communityID, mixedRoleUserID, "owner")
	seedIncidentUnitMember(t, handlesDB, otherUnitID, communityID, mixedRoleUserID, "tenant")
	seedIncidentUnitMember(t, handlesDB, unitID, communityID, activeMixedRoleUserID, "owner")
	seedIncidentUnitMember(t, handlesDB, otherUnitID, communityID, activeMixedRoleUserID, "tenant")
	if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE unit_members SET valid_to = $1 WHERE id = $2", time.Now().Add(-time.Hour), expiredMemberID); err != nil {
		t.Fatalf("expire member fixture: %v", err)
	}
	if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE unit_members SET valid_from = $1 WHERE id = $2", time.Now().Add(time.Hour), futureMemberID); err != nil {
		t.Fatalf("future-date member fixture: %v", err)
	}
	if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE unit_members SET valid_to = $1 WHERE id = $2", time.Now().Add(-time.Hour), mixedExpiredOwnerID); err != nil {
		t.Fatalf("expire mixed-role owner fixture: %v", err)
	}
	if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE unit_members SET valid_from = $1 WHERE id = $2", time.Now().Add(time.Hour), mixedFutureOwnerID); err != nil {
		t.Fatalf("future-date mixed-role owner fixture: %v", err)
	}
	ownerToken := mintAccessToken(t, deps, handlesDB, ownerID, false)
	tenantToken := mintAccessToken(t, deps, handlesDB, tenantID, false)
	otherTenantToken := mintAccessToken(t, deps, handlesDB, otherTenantID, false)
	expiredTenantToken := mintAccessToken(t, deps, handlesDB, expiredTenantID, false)
	futureTenantToken := mintAccessToken(t, deps, handlesDB, futureTenantID, false)
	deletedUnitTenantToken := mintAccessToken(t, deps, handlesDB, deletedUnitTenantID, false)
	mixedRoleToken := mintAccessToken(t, deps, handlesDB, mixedRoleUserID, false)
	activeMixedRoleToken := mintAccessToken(t, deps, handlesDB, activeMixedRoleUserID, false)

	send := func(community uuid.UUID, bearer string, body map[string]any, extra map[string]string) (*http.Response, map[string]any) {
		headers := map[string]string{}
		if bearer != "" {
			headers["Authorization"] = "Bearer " + bearer
		}
		for key, value := range extra {
			headers[key] = value
		}
		return doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+community.String()+"/incidents", body, headers)
	}
	valid := func(scope string) map[string]any { return incidentCreateBody(scope, "") }
	expect := func(label string, status int, response *http.Response, body map[string]any) {
		t.Helper()
		if response.StatusCode != status {
			t.Fatalf("%s: expected %d, got %d body=%v", label, status, response.StatusCode, body)
		}
	}
	created := func(label, bearer string, body map[string]any, headers map[string]string) map[string]any {
		t.Helper()
		response, output := send(communityID, bearer, body, headers)
		expect(label, http.StatusCreated, response, output)
		return output
	}

	// Missing tenants_can_create_incidents uses the documented true default.
	// The caller id is derived from the authenticated scope despite spoofed headers.
	adminOutput := created("admin unit create", adminToken, incidentCreateBody("unit", unitID.String()), map[string]string{
		"X-User-ID": uuid.NewString(), "X-Caller-ID": uuid.NewString(),
	})
	if adminOutput["created_by"] != adminID.String() || adminOutput["status"] != "open" || adminOutput["priority"] != "normal" {
		t.Fatalf("creator/status/priority must be server-owned: %v", adminOutput)
	}
	for _, forbidden := range []string{"deleted_at", "company_id", "company_name", "annual_budget", "reserve_fund", "rejection_reason", "notes", "photos", "attachments"} {
		if _, exists := adminOutput[forbidden]; exists {
			t.Fatalf("safe incident projection exposed %q: %v", forbidden, adminOutput)
		}
	}
	createdID, err := uuid.Parse(adminOutput["id"].(string))
	if err != nil {
		t.Fatalf("response omitted a valid incident id: %v", adminOutput)
	}
	stored, err := db.New(handlesDB.Read).GetVisibleIncidentByID(t.Context(), db.GetVisibleIncidentByIDParams{
		ID: createdID, CommunityID: communityID, UserID: adminID,
	})
	if err != nil || stored.CreatedBy != adminID || stored.Status != "open" || stored.Priority != "normal" {
		t.Fatalf("expected persisted authenticated creator and initial state, got row=%+v err=%v", stored, err)
	}

	created("tenant common default", tenantToken, valid("common"), nil)
	for _, inactive := range []struct {
		name  string
		token string
	}{
		{"sole expired member", expiredTenantToken},
		{"sole future member", futureTenantToken},
		{"sole deleted-unit member", deletedUnitTenantToken},
	} {
		response, body := send(communityID, inactive.token, valid("common"), nil)
		if response.StatusCode != http.StatusForbidden {
			t.Errorf("%s must not create common incident without active community-unit membership: got %d body=%v", inactive.name, response.StatusCode, body)
		}
	}
	created("tenant active exact unit", tenantToken, incidentCreateBody("unit", unitID.String()), nil)
	created("owner active exact unit", ownerToken, incidentCreateBody("unit", unitID.String()), nil)
	created("admin_staff target in community", staffToken, incidentCreateBody("unit", otherUnitID.String()), nil)
	noiseOutput := created("noise contract category", tenantToken, mapWith(valid("common"), "category", "noise"), nil)
	if noiseOutput["category"] != "noise" {
		t.Fatalf("public category vocabulary should remain noise, got %v", noiseOutput)
	}

	for _, denied := range []struct {
		name  string
		token string
		unit  uuid.UUID
	}{
		{"different exact unit", otherTenantToken, unitID},
		{"expired exact membership", expiredTenantToken, expiredUnitID},
		{"future exact membership", futureTenantToken, futureUnitID},
		{"owner different exact unit", ownerToken, otherUnitID},
	} {
		response, body := send(communityID, denied.token, incidentCreateBody("unit", denied.unit.String()), nil)
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("%s: expected 403, got %d body=%v", denied.name, response.StatusCode, body)
		}
	}
	for _, foreign := range []uuid.UUID{foreignUnitID, deletedUnitID} {
		response, body := send(communityID, adminToken, incidentCreateBody("unit", foreign.String()), nil)
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("foreign/deleted target %s: expected opaque 404, got %d body=%v", foreign, response.StatusCode, body)
		}
		response, body = send(communityID, staffToken, incidentCreateBody("unit", foreign.String()), nil)
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("admin_staff foreign/deleted target %s: expected opaque 404, got %d body=%v", foreign, response.StatusCode, body)
		}
	}
	response, body := send(foreignCommunityID, adminToken, valid("common"), nil)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign community: expected 404, got %d body=%v", response.StatusCode, body)
	}
	response, body = send(communityID, "", valid("common"), nil)
	expect("unauthenticated create", http.StatusUnauthorized, response, body)

	// Unit membership refusal and tenant-setting refusal are own-scope 403s.
	response, body = send(communityID, otherTenantToken, incidentCreateBody("unit", unitID.String()), nil)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("non-member unit target: expected 403, got %d body=%v", response.StatusCode, body)
	}
	setIncidentCommunitySettings(t, handlesDB, communityID, `{"tenants_can_create_incidents":false}`)
	response, body = send(communityID, mixedRoleToken, valid("common"), nil)
	if response.StatusCode != http.StatusForbidden {
		t.Errorf("active tenant membership must not be promoted by a stale owner role when tenant creation is disabled: got %d body=%v", response.StatusCode, body)
	}
	response, body = send(communityID, activeMixedRoleToken, valid("common"), nil)
	if response.StatusCode != http.StatusCreated {
		t.Errorf("active owner membership must take priority over the tenant setting for common creation: got %d body=%v", response.StatusCode, body)
	}
	response, body = send(communityID, activeMixedRoleToken, incidentCreateBody("unit", unitID.String()), nil)
	if response.StatusCode != http.StatusCreated {
		t.Errorf("active owner membership must take priority over the tenant setting for its exact unit: got %d body=%v", response.StatusCode, body)
	}
	response, body = send(communityID, tenantToken, valid("common"), nil)
	expect("tenant disabled common create", http.StatusForbidden, response, body)
	created("owner unaffected by tenant setting", ownerToken, valid("common"), nil)
	created("admin unaffected by tenant setting", adminToken, incidentCreateBody("unit", unitID.String()), nil)
	setIncidentCommunitySettings(t, handlesDB, communityID, `{"tenants_can_create_incidents":true}`)
	created("tenant explicit true setting", tenantToken, valid("common"), nil)
	setIncidentCommunitySettings(t, handlesDB, communityID, `{"tenants_can_create_incidents":"false"}`)
	response, body = send(communityID, tenantToken, valid("common"), nil)
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("invalid setting type must fail closed as an internal configuration error, got %d body=%v", response.StatusCode, body)
	}
	setIncidentCommunitySettings(t, handlesDB, communityID, `{}`)

	invalidRequests := []struct {
		name string
		body map[string]any
	}{
		{"common forbids unit_id", incidentCreateBody("common", unitID.String())},
		{"common rejects null unit_id", mapWith(valid("common"), "unit_id", nil)},
		{"unit requires unit_id", incidentCreateBody("unit", "")},
		{"unit rejects null unit_id", mapWith(incidentCreateBody("unit", ""), "unit_id", nil)},
		{"malformed unit uuid", incidentCreateBody("unit", "not-a-uuid")},
		{"malformed scope", mapWith(valid("common"), "scope", "building")},
		{"malformed category", mapWith(valid("common"), "category", "other;drop table")},
		{"empty title", mapWith(valid("common"), "title", "")},
		{"blank title", mapWith(valid("common"), "title", "   ")},
		{"long title", mapWith(valid("common"), "title", strings.Repeat("x", 201))},
		{"empty description", mapWith(valid("common"), "description", "")},
		{"long description", mapWith(valid("common"), "description", strings.Repeat("x", 5001))},
		{"long location", mapWith(valid("common"), "location_text", strings.Repeat("x", 256))},
	}
	for _, test := range invalidRequests {
		response, body := send(communityID, tenantToken, test.body, nil)
		if response.StatusCode != http.StatusBadRequest && response.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%s: expected 400/422, got %d body=%v", test.name, response.StatusCode, body)
		}
	}
	forgedFields := []string{"created_by", "status", "priority", "photos"}
	for _, field := range forgedFields {
		response, body := send(communityID, adminToken, mapWith(valid("common"), field, "forged"), nil)
		if response.StatusCode != http.StatusBadRequest && response.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("unknown/forged %s field: expected 400/422, got %d body=%v", field, response.StatusCode, body)
		}
	}
	response, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/not-a-uuid/incidents", valid("common"), map[string]string{"Authorization": "Bearer " + adminToken})
	if response.StatusCode != http.StatusBadRequest && response.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("malformed path UUID: expected 400/422, got %d body=%v", response.StatusCode, body)
	}
}

func incidentCreateBody(scope, unitID string) map[string]any {
	body := map[string]any{
		"title": "Lift stopped", "description": "The lift is not moving.",
		"category": "elevator", "scope": scope,
	}
	if unitID != "" {
		body["unit_id"] = unitID
	}
	return body
}

func mapWith(source map[string]any, key string, value any) map[string]any {
	out := make(map[string]any, len(source)+1)
	for name, item := range source {
		out[name] = item
	}
	out[key] = value
	return out
}

func seedIncidentUnitMember(t *testing.T, handlesDB db.Handles, unitID, communityID, userID uuid.UUID, role string) uuid.UUID {
	t.Helper()
	member, err := db.New(handlesDB.Write).InsertUnitMember(t.Context(), db.InsertUnitMemberParams{
		ID: uuid.New(), UnitID: unitID, CommunityID: communityID, UserID: userID,
		Role: role, Tenure: "full_owner",
	})
	if err != nil {
		t.Fatalf("insert %s fixture membership: %v", role, err)
	}
	return member.ID
}

func setIncidentCommunitySettings(t *testing.T, handlesDB db.Handles, communityID uuid.UUID, settings string) {
	t.Helper()
	if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE communities SET settings = $1::jsonb WHERE id = $2", settings, communityID); err != nil {
		t.Fatalf("set incident settings: %v", err)
	}
}
