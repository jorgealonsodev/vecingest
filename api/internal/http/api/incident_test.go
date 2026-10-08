package api_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
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
	createdID, err := uuid.Parse(incidentStringField(t, adminOutput, "id"))
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
	items := incidentResponseItems(t, list)
	if len(items) != 1 || items[0]["id"] != incident.ID.String() {
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
		seen[incidentStringField(t, item, "id")] = true
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
			ids[incidentStringField(t, item, "id")] = true
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
		{"status": {"future_status"}},
		{"category": {"unknown"}},
		{"unit_id": {"not-a-uuid"}},
		{"unit_id": {foreignUnit.String()}},
		{"unit_id": {deletedUnit.String()}},
		{"limit": {"0"}},
		{"limit": {"101"}},
		{"limit": {"-1"}},
		{"limit": {"many"}},
		{"cursor": {"not-a-cursor"}},
		{"cursor": {strings.Repeat("a", 1025)}},
		{"unexpected": {"x"}},
		{"category": {""}},
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
	// The default page is 20, so 105 rows need six pages; the bound fails the
	// test instead of looping forever if the cursor chain never terminates.
	pages := collectIncidentPages(t, "default traversal", len(all)/20+2, func(cursor string) map[string]any {
		query := url.Values{}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		resp, body := requestPage(communityID, pagerToken, query)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("default page (cursor=%q): got %d body=%v", cursor, resp.StatusCode, body)
		}
		return body
	})
	firstBody := pages[0]
	firstCursor := incidentStringField(t, firstBody, "next_cursor")
	if first := incidentResponseItems(t, firstBody); len(first) != 20 || firstCursor == "" {
		t.Fatalf("default page size/cursor: got %d items next=%q", len(first), firstCursor)
	}
	observed := make([]string, 0, len(all))
	for i, body := range pages {
		ids := incidentPageIDs(t, body)
		if len(ids) > 20 {
			t.Fatalf("default page %d exceeded 20: %d", i, len(ids))
		}
		observed = append(observed, ids...)
	}
	assertIncidentIDs(t, "same-timestamp default traversal", observed, expected)

	capResp, capBody := requestPage(communityID, pagerToken, url.Values{"limit": {"100"}})
	if capResp.StatusCode != http.StatusOK {
		t.Fatalf("100-item cap page: got %d body=%v", capResp.StatusCode, capBody)
	}
	capItems := incidentResponseItems(t, capBody)
	lastCursor := incidentStringField(t, capBody, "next_cursor")
	if len(capItems) != 100 || lastCursor == "" {
		t.Fatalf("100-item cap page: got %d items next=%q", len(capItems), lastCursor)
	}
	finalResp, finalBody := requestPage(communityID, pagerToken, url.Values{"limit": {"100"}, "cursor": {lastCursor}})
	if finalResp.StatusCode != http.StatusOK {
		t.Fatalf("final cap page: got %d body=%v", finalResp.StatusCode, finalBody)
	}
	finalItems := incidentResponseItems(t, finalBody)
	if finalNext := incidentStringField(t, finalBody, "next_cursor"); len(finalItems) != 5 || finalNext != "" {
		t.Fatalf("final page should contain five items and empty cursor: got %d items next=%q", len(finalItems), finalNext)
	}

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

	// The admin administers both communities through one office, so the same
	// caller is legitimately allowed to page either of them. Replaying or
	// rebinding a cursor therefore keeps caller, version and filters equal and
	// isolates the community component of the cursor binding.
	otherRows := make([]string, 0, 2)
	for range 2 {
		row := insertIncidentFixture(t, handlesDB, otherCommunityID, uuid.Nil, adminID, "elevator", "open", "common", "Other community page item")
		if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE incidents SET created_at = $1 WHERE id = $2", fixedTime, row.ID); err != nil {
			t.Fatalf("set other-community cursor timestamp: %v", err)
		}
		otherRows = append(otherRows, row.ID.String())
	}
	sort.Sort(sort.Reverse(sort.StringSlice(otherRows)))
	adminPage := func(community uuid.UUID, query url.Values) map[string]any {
		t.Helper()
		resp, body := requestPage(community, adminToken, query)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("admin page in community %s %v: got %d body=%v", community, query, resp.StatusCode, body)
		}
		return body
	}
	adminCursorA := incidentStringField(t, adminPage(communityID, url.Values{"limit": {"1"}}), "next_cursor")
	otherFirst := adminPage(otherCommunityID, url.Values{"limit": {"1"}})
	adminCursorB := incidentStringField(t, otherFirst, "next_cursor")
	if adminCursorA == "" || adminCursorB == "" {
		t.Fatalf("admin cursors must be non-empty: community A %q, community B %q", adminCursorA, adminCursorB)
	}
	// Positive controls: the same caller continues its own community-B cursor,
	// and re-encoding that cursor without changing a value is still accepted,
	// so a rejection below comes from the rewritten community and nothing else.
	otherSecond := adminPage(otherCommunityID, url.Values{"limit": {"1"}, "cursor": {adminCursorB}})
	assertIncidentIDs(t, "admin community-B traversal", append(incidentPageIDs(t, otherFirst), incidentPageIDs(t, otherSecond)...), otherRows)
	reencoded := adminPage(otherCommunityID, url.Values{"limit": {"1"}, "cursor": {tamperIncidentCursor(t, adminCursorB, "community", otherCommunityID.String())}})
	assertIncidentIDs(t, "re-encoded community-B cursor", incidentPageIDs(t, reencoded), otherRows[1:])
	for _, test := range []struct {
		name   string
		cursor string
	}{
		{"same caller replays a community-A cursor in community B", adminCursorA},
		{"community-B cursor rebound only in its community field", tamperIncidentCursor(t, adminCursorB, "community", communityID.String())},
	} {
		resp, body := requestPage(otherCommunityID, adminToken, url.Values{"limit": {"1"}, "cursor": {test.cursor}})
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%s: expected 400/422, got %d body=%v", test.name, resp.StatusCode, body)
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
	visiblePage := func(label string, query url.Values) []string {
		t.Helper()
		resp, body := requestPage(communityID, pagerToken, query)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: got %d body=%v", label, resp.StatusCode, body)
		}
		return incidentPageIDs(t, body)
	}
	anchorResp, anchorBody := requestPage(communityID, pagerToken, url.Values{"limit": {"1"}})
	if anchorResp.StatusCode != http.StatusOK {
		t.Fatalf("visibility anchor page: got %d body=%v", anchorResp.StatusCode, anchorBody)
	}
	assertIncidentIDs(t, "visibility anchor page", incidentPageIDs(t, anchorBody), []string{anchor.ID.String()})
	anchorCursor := incidentStringField(t, anchorBody, "next_cursor")
	privateAnchorResp, privateAnchorBody := requestPage(communityID, pagerToken, url.Values{"limit": {"2"}})
	if privateAnchorResp.StatusCode != http.StatusOK {
		t.Fatalf("private anchor page: got %d body=%v", privateAnchorResp.StatusCode, privateAnchorBody)
	}
	assertIncidentIDs(t, "private anchor page", incidentPageIDs(t, privateAnchorBody), []string{anchor.ID.String(), private.ID.String()})
	privateCursor := incidentStringField(t, privateAnchorBody, "next_cursor")

	// Positive recheck: while unit A membership is current, a common anchor and
	// a unit-private anchor both stay visible, so each cursor continues with
	// exactly the next rows in (created_at, id) order.
	assertIncidentIDs(t, "still-visible common anchor", visiblePage("still-visible common anchor", url.Values{"limit": {"10"}, "cursor": {anchorCursor}}),
		append([]string{private.ID.String()}, expected[:9]...))
	assertIncidentIDs(t, "still-visible private anchor", visiblePage("still-visible private anchor", url.Values{"limit": {"10"}, "cursor": {privateCursor}}), expected[:10])

	if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE unit_members SET valid_to = $1 WHERE unit_id = $2 AND user_id = $3", time.Now().Add(-time.Hour), unitA, pagerID); err != nil {
		t.Fatalf("expire private-unit membership: %v", err)
	}
	// The still-visible common anchor keeps paginating: the private row drops
	// out without skipping any other row, while the anchor that lost its grant
	// requires a pagination restart.
	assertIncidentIDs(t, "common anchor after visibility change", visiblePage("common anchor after visibility change", url.Values{"limit": {"10"}, "cursor": {anchorCursor}}), expected[:10])
	lostResp, lostBody := requestPage(communityID, pagerToken, url.Values{"limit": {"10"}, "cursor": {privateCursor}})
	if lostResp.StatusCode != http.StatusBadRequest && lostResp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("anchor that lost visibility should require pagination restart: got %d body=%v", lostResp.StatusCode, lostBody)
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

func TestIncident_FilteredKeysetPaginationTraversal(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)
	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "incident-filter-page-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Incident Filter Page Community")
	unitA, unitB := seedUnit(t, handlesDB, communityID), seedUnit(t, handlesDB, communityID)
	memberID := createUser(t, handlesDB, "incident-filter-page-member@example.com", false)
	seedIncidentUnitMember(t, handlesDB, unitA, communityID, memberID, "owner")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	memberToken := mintAccessToken(t, deps, handlesDB, memberID, false)

	// Status, category and target cycle with coprime periods, so every filter
	// matches rows interleaved with non-matching ones. created_at repeats in
	// groups of three, so tied timestamps fall inside filtered pages, and some
	// matching rows are soft-deleted and must never appear.
	type pagedIncident struct {
		id        string
		createdAt time.Time
		status    string
		category  string
		unit      uuid.UUID
	}
	statuses := []string{"open", "assigned", "closed"}
	categories := []string{"elevator", "plumbing", "noise_and_coexistence", "other"}
	base := time.Now().UTC().Truncate(time.Microsecond)
	rows := make([]pagedIncident, 0, 90)
	for i := range 90 {
		unit, scope := uuid.Nil, "common"
		switch i % 5 {
		case 2, 3:
			unit, scope = unitA, "unit"
		case 4:
			unit, scope = unitB, "unit"
		}
		row := insertIncidentFixture(t, handlesDB, communityID, unit, adminID, categories[i%4], statuses[i%3], scope, "Filtered page item")
		createdAt := base.Add(-time.Duration(i/3) * time.Minute)
		if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE incidents SET created_at = $1 WHERE id = $2", createdAt, row.ID); err != nil {
			t.Fatalf("set filtered page timestamp: %v", err)
		}
		if i%11 == 7 {
			if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE incidents SET deleted_at = now() WHERE id = $1", row.ID); err != nil {
				t.Fatalf("soft-delete filtered page fixture: %v", err)
			}
			continue
		}
		rows = append(rows, pagedIncident{id: row.ID.String(), createdAt: createdAt, status: statuses[i%3], category: categories[i%4], unit: unit})
	}
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].createdAt.Equal(rows[j].createdAt) {
			return rows[i].createdAt.After(rows[j].createdAt)
		}
		return rows[i].id > rows[j].id
	})

	for _, test := range []struct {
		name     string
		token    string
		admin    bool // admins also see private rows of units they do not hold
		status   string
		category string // public vocabulary: noise is stored as noise_and_coexistence
		unit     uuid.UUID
		limit    int
	}{
		{"member status", memberToken, false, "assigned", "", uuid.Nil, 4},
		{"member public noise category", memberToken, false, "", "noise", uuid.Nil, 4},
		{"member own unit", memberToken, false, "", "", unitA, 5},
		{"member status, category and unit", memberToken, false, "assigned", "noise", unitA, 1},
		{"admin other unit", adminToken, true, "", "", unitB, 3},
		{"admin status and category", adminToken, true, "closed", "plumbing", uuid.Nil, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			query := url.Values{"limit": {strconv.Itoa(test.limit)}}
			if test.status != "" {
				query.Set("status", test.status)
			}
			if test.category != "" {
				query.Set("category", test.category)
			}
			if test.unit != uuid.Nil {
				query.Set("unit_id", test.unit.String())
			}
			var want []string
			for _, row := range rows {
				publicCategory := row.category
				if publicCategory == "noise_and_coexistence" {
					publicCategory = "noise"
				}
				visible := test.admin || row.unit == uuid.Nil || row.unit == unitA
				if visible && (test.status == "" || row.status == test.status) &&
					(test.category == "" || publicCategory == test.category) &&
					(test.unit == uuid.Nil || row.unit == test.unit) {
					want = append(want, row.id)
				}
			}
			if len(want) <= test.limit {
				t.Fatalf("fixture must span several pages: %d matching rows for limit %d", len(want), test.limit)
			}

			pages := collectIncidentPages(t, test.name, len(want)/test.limit+2, func(cursor string) map[string]any {
				pageQuery := url.Values{}
				for key, values := range query {
					pageQuery[key] = values
				}
				if cursor != "" {
					pageQuery.Set("cursor", cursor)
				}
				resp, body := doJSON(t, client, http.MethodGet,
					srv.URL+"/v1/communities/"+communityID.String()+"/incidents?"+pageQuery.Encode(), nil,
					map[string]string{"Authorization": "Bearer " + test.token})
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("filtered page %v: got %d body=%v", pageQuery, resp.StatusCode, body)
				}
				return body
			})
			seen := make(map[string]int, len(want))
			got := make([]string, 0, len(want))
			for page, body := range pages {
				items := incidentResponseItems(t, body)
				if len(items) > test.limit {
					t.Fatalf("page %d exceeded limit %d: %d items", page, test.limit, len(items))
				}
				for _, item := range items {
					id := incidentStringField(t, item, "id")
					if previous, duplicate := seen[id]; duplicate {
						t.Fatalf("incident %s returned on page %d and again on page %d", id, previous, page)
					}
					seen[id] = page
					got = append(got, id)
					if test.status != "" && item["status"] != test.status {
						t.Errorf("page %d row %s violates status filter %q: %v", page, id, test.status, item)
					}
					if test.category != "" && item["category"] != test.category {
						t.Errorf("page %d row %s violates category filter %q: %v", page, id, test.category, item)
					}
					if test.unit != uuid.Nil && item["unit_id"] != test.unit.String() {
						t.Errorf("page %d row %s violates unit filter %s: %v", page, id, test.unit, item)
					}
				}
			}
			assertIncidentIDs(t, test.name, got, want)
			if wantPages := (len(want) + test.limit - 1) / test.limit; len(pages) != wantPages {
				t.Errorf("%d matching rows at limit %d should take %d pages, took %d", len(want), test.limit, wantPages, len(pages))
			}
		})
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

// incidentStringField fails the test with the decoded object, instead of
// panicking on a type assertion, when a string field it relies on is absent.
func incidentStringField(t *testing.T, object map[string]any, field string) string {
	t.Helper()
	value, ok := object[field].(string)
	if !ok {
		t.Fatalf("expected string field %q in %v", field, object)
	}
	return value
}

// incidentPageIDs returns the ids of one list page in response order.
func incidentPageIDs(t *testing.T, body map[string]any) []string {
	t.Helper()
	items := incidentResponseItems(t, body)
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, incidentStringField(t, item, "id"))
	}
	return ids
}

// assertIncidentIDs requires an exact ordered match, so a missing, extra,
// duplicated or reordered row is reported at its first differing position.
func assertIncidentIDs(t *testing.T, label string, got, want []string) {
	t.Helper()
	// G602 on want[i] below is a gosec false positive: i ranges over
	// min(len(got), len(want)), so it is strictly less than both lengths by
	// construction. gosec cannot follow min()'s provenance, and rewriting the
	// loop as `for i, gotID := range got` with an explicit
	// `if i >= len(want) { break }` guard does not satisfy it either, so the
	// clearer form is kept and the rule suppressed per line, following the
	// documented-suppression convention already used across this module.
	for i := range min(len(got), len(want)) {
		if got[i] != want[i] { //nolint:gosec // G602: i < len(want) by construction, see above
			t.Fatalf("%s: order/skip at %d: want %s got %s", label, i, want[i], got[i]) //nolint:gosec // G602: same false positive as the comparison above
		}
	}
	if len(got) != len(want) {
		t.Fatalf("%s: returned %d/%d incidents: got %v want %v", label, len(got), len(want), got, want)
	}
}

// collectIncidentPages fetches the first page (empty cursor) and follows
// next_cursor until it is empty. A chain longer than maxPages fails the test
// rather than looping forever when pagination never terminates.
func collectIncidentPages(t *testing.T, label string, maxPages int, fetch func(cursor string) map[string]any) []map[string]any {
	t.Helper()
	var pages []map[string]any
	cursor := ""
	for {
		if len(pages) == maxPages {
			t.Fatalf("%s: cursor chain did not end within %d pages", label, maxPages)
		}
		body := fetch(cursor)
		pages = append(pages, body)
		cursor = incidentStringField(t, body, "next_cursor")
		if cursor == "" {
			return pages
		}
	}
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
