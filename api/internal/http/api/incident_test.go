package api_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

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

func TestIncident_ListAndDetailRoutes(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)
	q := db.New(handlesDB.Write)
	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "incident-list-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Incident List Community")
	unitID := seedUnit(t, handlesDB, communityID)
	memberID := createUser(t, handlesDB, "incident-list-member@example.com", false)
	seedIncidentUnitMember(t, handlesDB, unitID, communityID, memberID, "owner")
	token := mintAccessToken(t, deps, handlesDB, memberID, false)
	incident, err := q.InsertIncident(t.Context(), db.InsertIncidentParams{
		ID: uuid.New(), CommunityID: communityID, CreatedBy: adminID,
		Title: "Visible common incident", Description: "A common issue.",
		Category: "elevator", Scope: "common",
	})
	if err != nil {
		t.Fatalf("insert incident fixture: %v", err)
	}
	headers := map[string]string{"Authorization": "Bearer " + token}

	response, list := doJSON(t, client, http.MethodGet,
		srv.URL+"/v1/communities/"+communityID.String()+"/incidents", nil, headers)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("list incidents: expected 200, got %d body=%v", response.StatusCode, list)
	}
	items, ok := list["items"].([]any)
	if !ok || len(items) != 1 || items[0].(map[string]any)["id"] != incident.ID.String() {
		t.Fatalf("list should return the visible incident in items: %v", list)
	}

	response, detail := doJSON(t, client, http.MethodGet,
		srv.URL+"/v1/incidents/"+incident.ID.String(), nil, headers)
	if response.StatusCode != http.StatusOK || detail["id"] != incident.ID.String() {
		t.Fatalf("detail should return the visible incident, got %d body=%v", response.StatusCode, detail)
	}
}

func TestIncident_ListAndDetailVisibilityFiltersAndValidation(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)
	q := db.New(handlesDB.Write)
	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "incident-read-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Incident Read Community")
	foreignOfficeID, _ := seedOfficeWithAdmin(t, handlesDB, "incident-read-foreign-admin@example.com")
	foreignCommunityID := seedCommunity(t, handlesDB, foreignOfficeID, "Foreign Incident Read Community")

	unitA := seedUnit(t, handlesDB, communityID)
	unitB := seedUnit(t, handlesDB, communityID)
	foreignUnit := seedUnit(t, handlesDB, foreignCommunityID)
	deletedUnit := seedUnit(t, handlesDB, communityID)
	if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE units SET deleted_at = now() WHERE id = $1", deletedUnit); err != nil {
		t.Fatalf("soft-delete unit fixture: %v", err)
	}
	memberID := createUser(t, handlesDB, "incident-read-member@example.com", false)
	peerID := createUser(t, handlesDB, "incident-read-peer@example.com", false)
	unrelatedID := createUser(t, handlesDB, "incident-read-unrelated@example.com", false)
	presidentID := createUser(t, handlesDB, "incident-read-president@example.com", false)
	expiredCreatorID := createUser(t, handlesDB, "incident-read-expired-creator@example.com", false)
	seedIncidentUnitMember(t, handlesDB, unitA, communityID, memberID, "owner")
	seedIncidentUnitMember(t, handlesDB, unitA, communityID, peerID, "tenant")
	seedIncidentUnitMember(t, handlesDB, unitB, communityID, unrelatedID, "tenant")
	presidentRowID := seedIncidentUnitMember(t, handlesDB, unitA, communityID, presidentID, "owner")
	expiredRowID := seedIncidentUnitMember(t, handlesDB, unitA, communityID, expiredCreatorID, "owner")
	if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE unit_members SET board_role = 'president' WHERE id = $1", presidentRowID); err != nil {
		t.Fatalf("set board president fixture: %v", err)
	}
	if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE unit_members SET valid_to = $1 WHERE id = $2", time.Now().Add(-time.Hour), expiredRowID); err != nil {
		t.Fatalf("expire creator membership fixture: %v", err)
	}

	staffID := createUser(t, handlesDB, "incident-read-staff@example.com", false)
	if _, err := q.InsertOfficeMember(t.Context(), db.InsertOfficeMemberParams{
		ID: uuid.New(), OfficeID: officeID, UserID: staffID, Role: "admin_staff",
	}); err != nil {
		t.Fatalf("insert admin_staff: %v", err)
	}
	seedActiveMFA(t, handlesDB, staffID)
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	staffToken := mintAccessToken(t, deps, handlesDB, staffID, false)
	memberToken := mintAccessToken(t, deps, handlesDB, memberID, false)
	peerToken := mintAccessToken(t, deps, handlesDB, peerID, false)
	unrelatedToken := mintAccessToken(t, deps, handlesDB, unrelatedID, false)
	presidentToken := mintAccessToken(t, deps, handlesDB, presidentID, false)
	expiredCreatorToken := mintAccessToken(t, deps, handlesDB, expiredCreatorID, false)
	createRow := func(community, unit, creator uuid.UUID, category, status, scope, title string) db.Incident {
		t.Helper()
		return insertIncidentFixture(t, handlesDB, community, unit, creator, category, status, scope, title)
	}
	common := createRow(communityID, uuid.Nil, adminID, "elevator", "open", "common", "Common visible")
	privateA := createRow(communityID, unitA, adminID, "plumbing", "assigned", "unit", "Unit A private")
	privateB := createRow(communityID, unitB, adminID, "electricity", "open", "unit", "Unit B private")
	expiredCreator := createRow(communityID, unitA, expiredCreatorID, "works", "in_progress", "unit", "Creator grant")
	noise := createRow(communityID, uuid.Nil, adminID, "noise_and_coexistence", "closed", "common", "Noise round trip")
	deleted := createRow(communityID, uuid.Nil, adminID, "other", "rejected", "common", "Deleted incident")
	foreign := createRow(foreignCommunityID, foreignUnit, adminID, "other", "open", "unit", "Foreign incident")
	if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE incidents SET deleted_at = now() WHERE id = $1", deleted.ID); err != nil {
		t.Fatalf("soft-delete incident fixture: %v", err)
	}

	list := func(community uuid.UUID, token string, values url.Values) (*http.Response, map[string]any) {
		t.Helper()
		path := srv.URL + "/v1/communities/" + community.String() + "/incidents"
		if len(values) > 0 {
			path += "?" + values.Encode()
		}
		return doJSON(t, client, http.MethodGet, path, nil, map[string]string{"Authorization": "Bearer " + token})
	}
	detail := func(id uuid.UUID, token string) (*http.Response, map[string]any) {
		t.Helper()
		return doJSON(t, client, http.MethodGet, srv.URL+"/v1/incidents/"+id.String(), nil, map[string]string{"Authorization": "Bearer " + token})
	}
	status, response := list(communityID, memberToken, nil)
	if status.StatusCode != http.StatusOK {
		t.Fatalf("member list: got %d body=%v", status.StatusCode, response)
	}
	items := incidentResponseItems(t, response)
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		seen[item["id"].(string)] = true
		for _, forbidden := range []string{"deleted_at", "company_id", "company_name", "annual_budget", "reserve_fund", "rejection_reason", "max_budget", "notes", "comments", "attachments", "events"} {
			if _, exists := item[forbidden]; exists {
				t.Fatalf("list projection exposed %q: %v", forbidden, item)
			}
		}
	}
	for _, id := range []uuid.UUID{common.ID, privateA.ID, expiredCreator.ID, noise.ID} {
		if !seen[id.String()] {
			t.Errorf("member list omitted visible incident %s: %v", id, seen)
		}
	}
	for _, id := range []uuid.UUID{privateB.ID, deleted.ID, foreign.ID} {
		if seen[id.String()] {
			t.Errorf("member list leaked invisible incident %s", id)
		}
	}

	for _, test := range []struct {
		name  string
		token string
		id    uuid.UUID
		want  int
	}{
		{"same-unit peer", peerToken, privateA.ID, http.StatusOK},
		{"unrelated unit member", unrelatedToken, privateA.ID, http.StatusNotFound},
		{"president alone", presidentToken, privateB.ID, http.StatusNotFound},
		{"owner exact unit", unrelatedToken, privateB.ID, http.StatusOK},
		{"admin sees private", adminToken, privateB.ID, http.StatusOK},
		{"admin_staff sees private", staffToken, privateB.ID, http.StatusOK},
		{"creator after membership expiry", expiredCreatorToken, expiredCreator.ID, http.StatusOK},
		{"cross-community", memberToken, foreign.ID, http.StatusNotFound},
		{"deleted incident", adminToken, deleted.ID, http.StatusNotFound},
		{"absent incident", adminToken, uuid.New(), http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			resp, body := detail(test.id, test.token)
			if resp.StatusCode != test.want {
				t.Fatalf("detail status: want %d, got %d body=%v", test.want, resp.StatusCode, body)
			}
			if resp.StatusCode == http.StatusOK {
				for _, forbidden := range []string{"deleted_at", "company_id", "company_name", "annual_budget", "reserve_fund", "rejection_reason", "max_budget", "notes", "comments", "attachments", "events"} {
					if _, exists := body[forbidden]; exists {
						t.Errorf("detail projection exposed %q: %v", forbidden, body)
					}
				}
			}
		})
	}

	for _, test := range []struct {
		name  string
		token string
		want  map[string]bool
	}{
		{"exact unit peer", peerToken, map[string]bool{privateA.ID.String(): true, expiredCreator.ID.String(): true}},
		{"unrelated unit", unrelatedToken, map[string]bool{}},
	} {
		resp, body := list(communityID, test.token, url.Values{"unit_id": {unitA.String()}})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s unit filter: got %d body=%v", test.name, resp.StatusCode, body)
		}
		ids := make(map[string]bool)
		for _, item := range incidentResponseItems(t, body) {
			ids[item["id"].(string)] = true
		}
		if len(ids) != len(test.want) {
			t.Errorf("%s unit filter: want %v, got %v", test.name, test.want, ids)
		}
		for id := range test.want {
			if !ids[id] {
				t.Errorf("%s unit filter omitted %s", test.name, id)
			}
		}
	}
	noiseResp, noiseBody := list(communityID, memberToken, url.Values{"category": {"noise"}})
	if noiseResp.StatusCode != http.StatusOK {
		t.Fatalf("noise filter: got %d body=%v", noiseResp.StatusCode, noiseBody)
	}
	noiseItems := incidentResponseItems(t, noiseBody)
	if len(noiseItems) != 1 || noiseItems[0]["id"] != noise.ID.String() || noiseItems[0]["category"] != "noise" {
		t.Fatalf("public noise filter/projection did not round-trip stored category: %v", noiseBody)
	}
	for _, status := range []string{"open", "assigned", "in_progress", "resolved", "closed", "rejected"} {
		resp, body := list(communityID, memberToken, url.Values{"status": {status}})
		if resp.StatusCode != http.StatusOK {
			t.Errorf("documented status filter %q was rejected: %d body=%v", status, resp.StatusCode, body)
		}
	}
	closedResp, closedBody := list(communityID, memberToken, url.Values{"status": {"closed"}})
	if closedResp.StatusCode != http.StatusOK || len(incidentResponseItems(t, closedBody)) != 1 {
		t.Fatalf("closed status filter: got %d body=%v", closedResp.StatusCode, closedBody)
	}
	queryResp, queryBody := doJSON(t, client, http.MethodGet,
		srv.URL+"/v1/incidents/"+common.ID.String()+"?unexpected=x", nil,
		map[string]string{"Authorization": "Bearer " + memberToken})
	if queryResp.StatusCode != http.StatusBadRequest && queryResp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("unknown detail query parameter: expected 400/422, got %d body=%v", queryResp.StatusCode, queryBody)
	}

	if resp, body := list(foreignCommunityID, memberToken, nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("foreign community list should be opaque 404, got %d body=%v", resp.StatusCode, body)
	}
	for _, endpoint := range []string{
		"/v1/communities/" + communityID.String() + "/incidents",
		"/v1/incidents/" + common.ID.String(),
	} {
		resp, body := doJSON(t, client, http.MethodGet, srv.URL+endpoint, nil, nil)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("unauthenticated GET %s: want 401, got %d body=%v", endpoint, resp.StatusCode, body)
		}
	}
	invalidFilters := []url.Values{
		{"status": {"future_status"}}, {"category": {"unknown"}}, {"unit_id": {"not-a-uuid"}},
		{"unit_id": {foreignUnit.String()}}, {"unit_id": {deletedUnit.String()}},
		{"limit": {"0"}}, {"limit": {"101"}}, {"limit": {"-1"}}, {"limit": {"many"}},
		{"cursor": {"not-a-cursor"}}, {"cursor": {strings.Repeat("a", 1025)}},
		{"unexpected": {"x"}}, {"category": {""}},
	}
	for _, values := range invalidFilters {
		resp, body := list(communityID, memberToken, values)
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("invalid list filter %v: expected 400/422, got %d body=%v", values, resp.StatusCode, body)
		}
	}
}

func TestIncident_KeysetPaginationAndVisibilityRecheck(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)
	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "incident-page-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Incident Page Community")
	otherCommunityID := seedCommunity(t, handlesDB, officeID, "Other Cursor Community")
	pagerID := createUser(t, handlesDB, "incident-page-member@example.com", false)
	otherMemberID := createUser(t, handlesDB, "incident-page-other-member@example.com", false)
	unitA, unitB := seedUnit(t, handlesDB, communityID), seedUnit(t, handlesDB, communityID)
	seedIncidentUnitMember(t, handlesDB, unitA, communityID, pagerID, "owner")
	seedIncidentUnitMember(t, handlesDB, unitB, communityID, pagerID, "owner")
	seedIncidentUnitMember(t, handlesDB, unitB, communityID, otherMemberID, "tenant")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	pagerToken := mintAccessToken(t, deps, handlesDB, pagerID, false)
	otherMemberToken := mintAccessToken(t, deps, handlesDB, otherMemberID, false)

	fixedTime := time.Now().UTC().Truncate(time.Microsecond)
	all := make([]db.Incident, 105)
	for i := range all {
		all[i] = insertIncidentFixture(t, handlesDB, communityID, uuid.Nil, adminID, "elevator", "open", "common", "Tied timestamp page item")
		if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE incidents SET created_at = $1 WHERE id = $2", fixedTime, all[i].ID); err != nil {
			t.Fatalf("set tied cursor timestamp: %v", err)
		}
	}
	expected := make([]string, len(all))
	for i := range all {
		expected[i] = all[i].ID.String()
	}
	sort.Sort(sort.Reverse(sort.StringSlice(expected)))
	requestPage := func(community uuid.UUID, token string, query url.Values) (*http.Response, map[string]any) {
		t.Helper()
		path := srv.URL + "/v1/communities/" + community.String() + "/incidents"
		if len(query) != 0 {
			path += "?" + query.Encode()
		}
		return doJSON(t, client, http.MethodGet, path, nil, map[string]string{"Authorization": "Bearer " + token})
	}
	firstResp, firstBody := requestPage(communityID, pagerToken, nil)
	if firstResp.StatusCode != http.StatusOK {
		t.Fatalf("default first page: got %d body=%v", firstResp.StatusCode, firstBody)
	}
	first := incidentResponseItems(t, firstBody)
	if len(first) != 20 || firstBody["next_cursor"] == "" {
		t.Fatalf("default page size/cursor: got %d items next=%v", len(first), firstBody["next_cursor"])
	}
	cursor := firstBody["next_cursor"].(string)
	observed := make([]string, 0, len(all))
	for _, item := range first {
		observed = append(observed, item["id"].(string))
	}
	for cursor != "" {
		resp, body := requestPage(communityID, pagerToken, url.Values{"cursor": {cursor}})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("default next page: got %d body=%v", resp.StatusCode, body)
		}
		items := incidentResponseItems(t, body)
		if len(items) > 20 {
			t.Fatalf("default page exceeded 20: %d", len(items))
		}
		for _, item := range items {
			observed = append(observed, item["id"].(string))
		}
		cursor = body["next_cursor"].(string)
	}
	if len(observed) != len(expected) {
		t.Fatalf("default traversal returned %d/%d incidents", len(observed), len(expected))
	}
	for i := range expected {
		if observed[i] != expected[i] {
			t.Fatalf("same-timestamp keyset order/skip at %d: want %s got %s", i, expected[i], observed[i])
		}
	}

	capResp, capBody := requestPage(communityID, pagerToken, url.Values{"limit": {"100"}})
	capItems := incidentResponseItems(t, capBody)
	if capResp.StatusCode != http.StatusOK || len(capItems) != 100 || capBody["next_cursor"] == "" {
		t.Fatalf("100-item cap page: got %d items status=%d body=%v", len(capItems), capResp.StatusCode, capBody)
	}
	lastCursor := capBody["next_cursor"].(string)
	finalResp, finalBody := requestPage(communityID, pagerToken, url.Values{"limit": {"100"}, "cursor": {lastCursor}})
	finalItems := incidentResponseItems(t, finalBody)
	if finalResp.StatusCode != http.StatusOK || len(finalItems) != 5 || finalBody["next_cursor"] != "" {
		t.Fatalf("final page should contain five items and empty cursor: got %d status=%d body=%v", len(finalItems), finalResp.StatusCode, finalBody)
	}

	firstCursor := firstBody["next_cursor"].(string)
	for _, test := range []struct {
		name      string
		community uuid.UUID
		token     string
		query     url.Values
	}{
		{"different caller", communityID, otherMemberToken, url.Values{"cursor": {firstCursor}}},
		{"different community", otherCommunityID, adminToken, url.Values{"cursor": {firstCursor}}},
		{"different filters", communityID, pagerToken, url.Values{"status": {"closed"}, "cursor": {firstCursor}}},
		{"invalid encoding", communityID, pagerToken, url.Values{"cursor": {"%%%"}}},
	} {
		resp, body := requestPage(test.community, test.token, test.query)
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%s cursor: expected 400/422, got %d body=%v", test.name, resp.StatusCode, body)
		}
	}
	for _, test := range []struct {
		name  string
		field string
		value any
	}{
		{"fabricated anchor id", "id", uuid.NewString()},
		{"mismatched timestamp", "created_at", "2000-01-01T00:00:00Z"},
		{"unsupported version", "v", float64(2)},
		{"wrong field type", "category", nil},
		{"unknown field", "forged", "value"},
	} {
		bad := tamperIncidentCursor(t, firstCursor, test.field, test.value)
		resp, body := requestPage(communityID, pagerToken, url.Values{"cursor": {bad}})
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%s: expected invalid cursor, got %d body=%v", test.name, resp.StatusCode, body)
		}
	}

	anchor := insertIncidentFixture(t, handlesDB, communityID, uuid.Nil, adminID, "other", "open", "common", "Visibility anchor")
	private := insertIncidentFixture(t, handlesDB, communityID, unitA, adminID, "other", "open", "unit", "Membership-dependent private row")
	fallback := insertIncidentFixture(t, handlesDB, communityID, uuid.Nil, adminID, "other", "open", "common", "Visibility fallback")
	for _, item := range []struct {
		id uuid.UUID
		t  time.Time
	}{{anchor.ID, fixedTime.Add(2 * time.Hour)}, {private.ID, fixedTime.Add(time.Hour)}, {fallback.ID, fixedTime.Add(-time.Hour)}} {
		if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE incidents SET created_at = $1 WHERE id = $2", item.t, item.id); err != nil {
			t.Fatalf("order visibility-change fixture: %v", err)
		}
	}
	anchorResp, anchorBody := requestPage(communityID, pagerToken, url.Values{"limit": {"1"}})
	if anchorResp.StatusCode != http.StatusOK || incidentResponseItems(t, anchorBody)[0]["id"] != anchor.ID.String() {
		t.Fatalf("visibility anchor page: status=%d body=%v", anchorResp.StatusCode, anchorBody)
	}
	anchorCursor := anchorBody["next_cursor"].(string)
	if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE unit_members SET valid_to = $1 WHERE unit_id = $2 AND user_id = $3", time.Now().Add(-time.Hour), unitA, pagerID); err != nil {
		t.Fatalf("expire private-unit membership: %v", err)
	}
	afterChangeResp, afterChangeBody := requestPage(communityID, pagerToken, url.Values{"limit": {"10"}, "cursor": {anchorCursor}})
	if afterChangeResp.StatusCode != http.StatusOK {
		t.Fatalf("page after visibility change: got %d body=%v", afterChangeResp.StatusCode, afterChangeBody)
	}
	for _, item := range incidentResponseItems(t, afterChangeBody) {
		if item["id"] == private.ID.String() {
			t.Fatal("next page reused a stale visibility grant for the private unit row")
		}
	}

	stale := firstCursor
	anchorID := expected[19]
	if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE incidents SET deleted_at = now() WHERE id = $1", anchorID); err != nil {
		t.Fatalf("delete cursor anchor: %v", err)
	}
	staleResp, staleBody := requestPage(communityID, pagerToken, url.Values{"cursor": {stale}})
	if staleResp.StatusCode != http.StatusBadRequest && staleResp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("deleted cursor anchor should require pagination restart: got %d body=%v", staleResp.StatusCode, staleBody)
	}
}

func insertIncidentFixture(t *testing.T, handlesDB db.Handles, communityID, unitID, creatorID uuid.UUID, category, status, scope, title string) db.Incident {
	t.Helper()
	unit := pgtype.UUID{}
	if unitID != uuid.Nil {
		unit = pgtype.UUID{Bytes: unitID, Valid: true}
	}
	incident, err := db.New(handlesDB.Write).InsertIncident(t.Context(), db.InsertIncidentParams{
		ID: uuid.New(), CommunityID: communityID, UnitID: unit, CreatedBy: creatorID,
		Title: title, Description: "Incident fixture description.", Category: category, Scope: scope,
	})
	if err != nil {
		t.Fatalf("insert incident fixture: %v", err)
	}
	if status != "open" {
		if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE incidents SET status = $1 WHERE id = $2", status, incident.ID); err != nil {
			t.Fatalf("set incident fixture status: %v", err)
		}
		incident.Status = status
	}
	return incident
}

func incidentResponseItems(t *testing.T, response map[string]any) []map[string]any {
	t.Helper()
	items, ok := response["items"].([]any)
	if !ok {
		t.Fatalf("response has no items array: %v", response)
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("incident item is not an object: %#v", item)
		}
		out = append(out, row)
	}
	return out
}

func tamperIncidentCursor(t *testing.T, cursor, field string, value any) string {
	t.Helper()
	payload, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		t.Fatalf("decode cursor fixture: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode cursor payload fixture: %v", err)
	}
	fields[field] = value
	payload, err = json.Marshal(fields)
	if err != nil {
		t.Fatalf("encode cursor payload fixture: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload)
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
