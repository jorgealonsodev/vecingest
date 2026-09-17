package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
	"github.com/jorgealonsodev/vecingest/internal/http/handlers"
)

// mintAccessToken inserts a real session row (so the D-D revocation
// cache's fresh-restart fallback sees a live session, not a phantom
// revoked one) and mints a matching access token directly via the
// Issuer -- bypassing the full (TOTP-gated, for superadmin) login flow,
// exactly as bearer_test.go already does for its own unit tests.
func mintAccessToken(t *testing.T, deps *handlers.Deps, handlesDB db.Handles, userID uuid.UUID, isSuperadmin bool) string {
	t.Helper()
	familyID := uuid.New()
	_, refreshHash, err := token.GenerateRefresh()
	if err != nil {
		t.Fatalf("generate refresh: %v", err)
	}
	q := db.New(handlesDB.Write)
	if _, err := q.InsertSession(t.Context(), db.InsertSessionParams{
		ID:               uuid.New(),
		UserID:           userID,
		RefreshTokenHash: refreshHash,
		FamilyID:         familyID,
		Platform:         "web",
		ExpiresAt:        time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	access, err := deps.AccessIssuer.IssueAccess(userID, familyID, isSuperadmin)
	if err != nil {
		t.Fatalf("issue access: %v", err)
	}
	return access
}

// seedOfficeWithAdmin inserts an office and one admin office_members row
// directly (bypassing the HTTP bootstrap flow this file also tests),
// for tests whose focus is a DIFFERENT endpoint that merely needs an
// existing office+admin as a precondition. The admin is seeded with
// ACTIVE TOTP (seedActiveMFA): since auth-mfa-totp delta's Mandatory
// TOTP For Admin And Admin_staff Scope Access gate now runs at every
// community/office resolution, an admin test caller with no TOTP would
// be rejected before ever reaching the endpoint each of these tests
// actually means to exercise. A test that specifically wants an admin
// WITHOUT TOTP (to exercise that gate itself) seeds its own caller
// directly rather than using this helper -- see public_form_test.go.
func seedOfficeWithAdmin(t *testing.T, handlesDB db.Handles, adminEmail string) (officeID, adminID uuid.UUID) {
	t.Helper()
	q := db.New(handlesDB.Write)
	office, err := q.InsertOffice(t.Context(), db.InsertOfficeParams{
		ID:   uuid.New(),
		Name: "Seed Office",
		Cif:  uuid.NewString()[:8],
	})
	if err != nil {
		t.Fatalf("insert office: %v", err)
	}
	adminID = createUser(t, handlesDB, adminEmail, false)
	if _, err := q.InsertOfficeMember(t.Context(), db.InsertOfficeMemberParams{
		ID:       uuid.New(),
		OfficeID: office.ID,
		UserID:   adminID,
		Role:     "admin",
	}); err != nil {
		t.Fatalf("insert office member: %v", err)
	}
	seedActiveMFA(t, handlesDB, adminID)
	return office.ID, adminID
}

// seedActiveMFA inserts an ENABLED user_mfa row for userID directly
// (bypassing the enroll/verify HTTP flow this package's mfa_test.go
// exercises separately), for any test whose admin/admin_staff caller
// merely needs to satisfy the mandatory-TOTP gate as a precondition.
// The secret value itself is never used by these tests -- only
// enabled_at needs to be non-NULL.
func seedActiveMFA(t *testing.T, handlesDB db.Handles, userID uuid.UUID) {
	t.Helper()
	q := db.New(handlesDB.Write)
	if _, err := q.UpsertUserMFA(t.Context(), db.UpsertUserMFAParams{
		UserID:              userID,
		TotpSecretEncrypted: []byte("seed-placeholder-not-a-real-secret"),
	}); err != nil {
		t.Fatalf("seed user_mfa: %v", err)
	}
	if err := q.ConfirmUserMFAEnrollment(t.Context(), userID); err != nil {
		t.Fatalf("confirm user_mfa enrollment: %v", err)
	}
}

// office-management: Office Creation Restricted To Superadmin (both
// scenarios).
func TestOffice_CreationRestrictedToSuperadmin(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	regularID := createUser(t, handlesDB, "regular-admin@example.com", false)
	regularToken := mintAccessToken(t, deps, handlesDB, regularID, false)

	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/admin/offices", map[string]any{
		"name": "Denied Office", "cif": "B10000001",
		"admin_name": "Bootstrap Admin", "admin_email": "denied-admin@example.com",
	}, map[string]string{"Authorization": "Bearer " + regularToken})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for a non-superadmin caller, got %d body=%v", resp.StatusCode, body)
	}

	superID := createUser(t, handlesDB, "super@example.com", true)
	superToken := mintAccessToken(t, deps, handlesDB, superID, true)

	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/admin/offices", map[string]any{
		"name": "Allowed Office", "cif": "B10000002",
		"admin_name": "Bootstrap Admin", "admin_email": "allowed-admin@example.com",
	}, map[string]string{"Authorization": "Bearer " + superToken})
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected the superadmin's office creation to succeed, got %d body=%v", resp.StatusCode, body)
	}
	if body["cif"] != "B10000002" {
		t.Fatalf("expected the created office in the response, got %v", body)
	}
}

// office-management: First-Admin Bootstrap Without Invitation (both
// scenarios).
func TestOffice_FirstAdminBootstrapWithoutInvitation(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	superID := createUser(t, handlesDB, "super-bootstrap@example.com", true)
	superToken := mintAccessToken(t, deps, handlesDB, superID, true)

	adminEmail := "bootstrap-admin@example.com"
	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/admin/offices", map[string]any{
		"name": "Bootstrap Office", "cif": "B20000001",
		"admin_name": "Bootstrap Admin", "admin_email": adminEmail,
	}, map[string]string{"Authorization": "Bearer " + superToken})
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected office creation to succeed, got %d body=%v", resp.StatusCode, body)
	}

	resp, _ = doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/login", map[string]any{
		"email": adminEmail, "password": "a-random-guessed-password-12", "platform": "web",
	}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected login to fail for the bootstrap admin before forgot-password, got %d", resp.StatusCode)
	}

	var count int
	if err := handlesDB.Write.QueryRow(t.Context(), "SELECT count(*) FROM invitations").Scan(&count); err != nil {
		t.Fatalf("count invitations: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected zero invitations rows for a bootstrap admin, got %d", count)
	}
}

// office-management: Office Staff Addition Restricted To Existing
// Accounts (both scenarios).
func TestOfficeMembers_StaffAdditionRestrictedToExistingAccounts(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "office-admin@example.com")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)

	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/offices/me/members", map[string]any{
		"email": "unknown@example.com",
	}, map[string]string{"Authorization": "Bearer " + adminToken})
	if resp.StatusCode < 400 {
		t.Fatalf("expected failure for an email with no existing account, got %d body=%v", resp.StatusCode, body)
	}
	q := db.New(handlesDB.Write)
	if _, err := q.GetUserByEmail(t.Context(), "unknown@example.com"); err == nil {
		t.Fatalf("expected no user to have been created for an unknown email")
	}

	staffID := createUser(t, handlesDB, "office-staff@example.com", false)
	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/offices/me/members", map[string]any{
		"email": "office-staff@example.com",
	}, map[string]string{"Authorization": "Bearer " + adminToken})
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected success adding an existing account, got %d body=%v", resp.StatusCode, body)
	}
	member, err := q.GetOfficeMemberByOfficeAndUser(t.Context(), db.GetOfficeMemberByOfficeAndUserParams{
		OfficeID: officeID, UserID: staffID,
	})
	if err != nil {
		t.Fatalf("expected an office_members row for the added staff: %v", err)
	}
	if member.Role != "admin_staff" {
		t.Fatalf("expected role admin_staff, got %q", member.Role)
	}
}

// office-management: GET /v1/offices/me Returns Caller's Offices, and
// (by the requirement's own text) GET /v1/offices/me/members lists only
// members of the caller's own office(s).
func TestOffice_GetMyOfficesAndMembersScopedToCaller(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "own-office-admin@example.com")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	// A second, unrelated office+admin the caller has no membership in.
	seedOfficeWithAdmin(t, handlesDB, "foreign-office-admin@example.com")

	resp, body := doJSON(t, client, http.MethodGet, srv.URL+"/v1/offices/me", nil, map[string]string{
		"Authorization": "Bearer " + adminToken,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%v", resp.StatusCode, body)
	}
	offices, _ := body["offices"].([]any)
	if len(offices) != 1 {
		t.Fatalf("expected exactly one office (the caller's own), got %v", body)
	}
	first, _ := offices[0].(map[string]any)
	if first["id"] != officeID.String() {
		t.Fatalf("expected office %s, got %v", officeID, first["id"])
	}

	q := db.New(handlesDB.Write)
	staffID := createUser(t, handlesDB, "own-office-staff@example.com", false)
	if _, err := q.InsertOfficeMember(t.Context(), db.InsertOfficeMemberParams{
		ID: uuid.New(), OfficeID: officeID, UserID: staffID, Role: "admin_staff",
	}); err != nil {
		t.Fatalf("insert office member: %v", err)
	}

	resp, body = doJSON(t, client, http.MethodGet, srv.URL+"/v1/offices/me/members", nil, map[string]string{
		"Authorization": "Bearer " + adminToken,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%v", resp.StatusCode, body)
	}
	members, _ := body["members"].([]any)
	if len(members) != 2 {
		t.Fatalf("expected 2 members (admin + staff) of the caller's own office, got %v", body)
	}
}
