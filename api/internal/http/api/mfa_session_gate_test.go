package api_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
)

// These tests close R1-mandatory-totp-gate-is-only-an-enrollment-flag
// (review lineages review-0e1833930adf141a and review-e72754dc7521b57a).
// The gate they exercise is SECOND-FACTOR AUTHENTICATION, a per-session
// fact, not the durable per-account enrollment flag the first
// implementation read. Every one of them fails against that earlier
// implementation.
//
// They deliberately avoid mintAccessToken: the caller's session must be
// produced by the REAL login path, because how the session was
// authenticated is precisely what is under test. A helper that minted
// the session fact directly would prove nothing about login.

// seedActiveMFAWithSecret installs a REAL, ACTIVE TOTP factor for
// userID -- a freshly generated secret, encrypted under the same key
// the handlers decrypt with -- and returns its base32 form so the test
// can compute genuinely valid codes for it. It is the seeded sibling of
// seedActiveMFA (office_test.go), which stores a placeholder no code can
// ever match: enough when the test only needs enabled_at to be
// non-NULL, useless once a test has to actually PASS a TOTP challenge.
//
// Seeding the factor rather than enrolling it over HTTP keeps these
// tests focused on the login challenge; mfa_test.go already covers the
// enroll/verify endpoints that put a factor there in production.
func seedActiveMFAWithSecret(t *testing.T, handlesDB db.Handles, key [32]byte, userID uuid.UUID) string {
	t.Helper()
	secret, err := mfa.GenerateSecret()
	if err != nil {
		t.Fatalf("generate totp secret: %v", err)
	}
	encrypted, err := mfa.EncryptSecret(key, secret)
	if err != nil {
		t.Fatalf("encrypt totp secret: %v", err)
	}
	q := db.New(handlesDB.Write)
	if _, err := q.UpsertUserMFA(t.Context(), db.UpsertUserMFAParams{
		UserID:              userID,
		TotpSecretEncrypted: encrypted,
	}); err != nil {
		t.Fatalf("seed user_mfa: %v", err)
	}
	if err := q.ConfirmUserMFAEnrollment(t.Context(), userID); err != nil {
		t.Fatalf("confirm user_mfa enrollment: %v", err)
	}
	return mfa.Base32Secret(secret)
}

// login posts real credentials to the real endpoint. totpCode is
// omitted from the body entirely when empty, so "logged in without a
// code" is a request that never carries the field, exactly as a client
// that has no code to send would issue it.
func login(t *testing.T, client *http.Client, srv, email, totpCode, platform string) (*http.Response, map[string]any) {
	t.Helper()
	body := map[string]any{"email": email, "password": testPassword, "platform": platform}
	if totpCode != "" {
		body["totp_code"] = totpCode
	}
	return doJSON(t, client, http.MethodPost, srv+"/v1/auth/login", body, nil)
}

// liveSessionCount counts the caller's non-revoked session rows straight
// from Postgres, independent of any response body: a rejected login must
// leave NO session behind, and only the table can prove that.
func liveSessionCount(t *testing.T, handlesDB db.Handles, userID uuid.UUID) int {
	t.Helper()
	var count int
	if err := handlesDB.Write.Pool().QueryRow(t.Context(),
		"SELECT count(*) FROM sessions WHERE user_id = $1 AND revoked_at IS NULL", userID,
	).Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	return count
}

// seedOfficeAdminUser creates an office plus an admin member for it,
// with NO TOTP factor -- the caller decides whether and how one appears.
func seedOfficeAdminUser(t *testing.T, handlesDB db.Handles, email string) (officeID, adminID uuid.UUID) {
	t.Helper()
	q := db.New(handlesDB.Write)
	office, err := q.InsertOffice(t.Context(), db.InsertOfficeParams{
		ID: uuid.New(), Name: "MFA Session Gate Office", Cif: uuid.NewString()[:8],
	})
	if err != nil {
		t.Fatalf("insert office: %v", err)
	}
	adminID = createUser(t, handlesDB, email, false)
	if _, err := q.InsertOfficeMember(t.Context(), db.InsertOfficeMemberParams{
		ID: uuid.New(), OfficeID: office.ID, UserID: adminID, Role: "admin",
	}); err != nil {
		t.Fatalf("insert office member: %v", err)
	}
	return office.ID, adminID
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func accessTokenFrom(t *testing.T, body map[string]any) string {
	t.Helper()
	tok, _ := body["access_token"].(string)
	if tok == "" {
		t.Fatalf("expected an access_token in the login response, got %v", body)
	}
	return tok
}

// Scenario 1: an admin with an enrolled factor who logs in WITHOUT a
// code is rejected, and no session is issued. Against the enrollment-flag
// implementation this login returned 200 with a full session, which is
// the whole of bypass 1: a stolen password alone reached every
// admin-scoped route.
func TestMFAGate_AdminWithEnrolledFactorRejectedWithoutTOTPCode(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	email := "mfa-session-no-code@example.com"
	_, adminID := seedOfficeAdminUser(t, handlesDB, email)
	seedActiveMFAWithSecret(t, handlesDB, deps.MFAKey, adminID)

	resp, body := login(t, client, srv.URL, email, "", "web")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403: an account with an ACTIVE second factor must not authenticate on a password alone, got %d body=%v", resp.StatusCode, body)
	}
	if body["code"] != "AUTH_MFA_REQUIRED" {
		t.Fatalf("expected AUTH_MFA_REQUIRED, distinguishable from invalid credentials so a client can prompt for the code, got %v", body)
	}
	if body["access_token"] != nil {
		t.Fatalf("expected NO access token on a rejected login, got %v", body)
	}
	if n := liveSessionCount(t, handlesDB, adminID); n != 0 {
		t.Fatalf("expected NO session row for a login rejected at the TOTP challenge, got %d", n)
	}
}

// Scenario 2: the same admin, logging in WITH a valid code, reaches an
// admin-scoped route. This is the positive path the gate must not
// break -- without it, scenario 1 would be satisfied by an endpoint that
// simply rejects every admin.
func TestMFAGate_AdminLoginWithValidCodeReachesAdminScopedRoute(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	email := "mfa-session-with-code@example.com"
	officeID, adminID := seedOfficeAdminUser(t, handlesDB, email)
	secret := seedActiveMFAWithSecret(t, handlesDB, deps.MFAKey, adminID)

	resp, body := login(t, client, srv.URL, email, validTOTPCode(t, secret), "web")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200: a valid TOTP code must complete the login, got %d body=%v", resp.StatusCode, body)
	}
	accessToken := accessTokenFrom(t, body)

	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities", map[string]any{
		"office_id": officeID.String(), "name": "MFA Authenticated Community",
	}, bearer(accessToken))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected a second-factor-authenticated admin to reach an admin-scoped route, got %d body=%v", resp.StatusCode, body)
	}
}

// Scenario 3, the bypass-2 regression and the one that proves the fix.
// A password-only session enrolls a fresh factor through the ungated
// /v1/me/mfa/enroll + /verify pair and then calls an admin-scoped route.
// Under the enrollment-flag gate that call SUCCEEDED -- enrolling was
// all it took. It must now be rejected, because that session was never
// second-factor authenticated, while a fresh login carrying a code must
// still succeed (otherwise the account would be stranded).
func TestMFAGate_PasswordOnlySessionCannotEnrollItsWayPastTheGate(t *testing.T) {
	srv, _, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	email := "mfa-session-enroll-bypass@example.com"
	officeID, adminID := seedOfficeAdminUser(t, handlesDB, email)

	// No factor yet, so the password alone is a complete credential --
	// this is the legitimate bootstrap path, and an attacker holding a
	// stolen password gets exactly the same session.
	resp, body := login(t, client, srv.URL, email, "", "web")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200: an account with no second factor still logs in on its password, got %d body=%v", resp.StatusCode, body)
	}
	passwordOnlyToken := accessTokenFrom(t, body)
	auth := bearer(passwordOnlyToken)

	resp, enrollBody := doJSON(t, client, http.MethodPost, srv.URL+"/v1/me/mfa/enroll", nil, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on enroll, got %d body=%v", resp.StatusCode, enrollBody)
	}
	secret, _ := enrollBody["secret"].(string)
	if secret == "" {
		t.Fatalf("expected a base32 secret from enroll, got %v", enrollBody)
	}
	// The enrollment email code is read from the queued job: this test is
	// about the SESSION gate, so it plays an attacker who also controls
	// the mailbox and asserts activation still opens nothing.
	resp, verifyBody := doJSON(t, client, http.MethodPost, srv.URL+"/v1/me/mfa/verify", map[string]any{
		"code": validTOTPCode(t, secret), "email_code": enrollEmailCode(t, handlesDB, adminID),
	}, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 activating the freshly enrolled factor, got %d body=%v", resp.StatusCode, verifyBody)
	}

	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities", map[string]any{
		"office_id": officeID.String(), "name": "Bypass Community",
	}, auth)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403: a session that authenticated on a PASSWORD ALONE must not reach an admin-scoped route by enrolling a second factor after the fact, got %d body=%v", resp.StatusCode, body)
	}
	if body["code"] != "AUTH_MFA_REQUIRED" {
		t.Fatalf("expected AUTH_MFA_REQUIRED (re-login with a code), not the enrollment-precondition code -- the factor now EXISTS, the session just never used it; got %v", body)
	}

	// The account is not stranded: logging in again, now carrying a
	// code, reaches the very same route.
	resp, body = login(t, client, srv.URL, email, validTOTPCode(t, secret), "web")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected the re-login with a valid code to succeed, got %d body=%v", resp.StatusCode, body)
	}
	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities", map[string]any{
		"office_id": officeID.String(), "name": "Re-Login Community",
	}, bearer(accessTokenFrom(t, body)))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected the re-authenticated session to reach the admin-scoped route, got %d body=%v", resp.StatusCode, body)
	}
}

// Scenario 4: refresh preserves the fact. The access token lives 15
// minutes; if rotation dropped the second-factor fact, every elevated
// session would silently decay into a password-only one a quarter of an
// hour after login.
func TestMFAGate_RefreshPreservesSecondFactorFact(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	email := "mfa-session-refresh@example.com"
	officeID, adminID := seedOfficeAdminUser(t, handlesDB, email)
	secret := seedActiveMFAWithSecret(t, handlesDB, deps.MFAKey, adminID)

	// The native (body) transport keeps this test on the refresh token
	// itself, with no cookie/CSRF leg to confuse the assertion.
	resp, body := login(t, client, srv.URL, email, validTOTPCode(t, secret), "ios")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 logging in with a valid code, got %d body=%v", resp.StatusCode, body)
	}
	refreshToken, _ := body["refresh_token"].(string)
	if refreshToken == "" {
		t.Fatalf("expected a refresh_token on the native transport, got %v", body)
	}

	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/auth/refresh", map[string]any{
		"refresh_token": refreshToken,
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on refresh, got %d body=%v", resp.StatusCode, body)
	}
	refreshedToken := accessTokenFrom(t, body)

	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities", map[string]any{
		"office_id": officeID.String(), "name": "Refreshed Community",
	}, bearer(refreshedToken))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected a REFRESHED second-factor-authenticated session to still reach an admin-scoped route, got %d body=%v", resp.StatusCode, body)
	}
}

// Scenario 5: a community owner with no factor is unaffected. TOTP is
// mandatory for admin/admin_staff only (PRD §10.1); it stays optional
// for owners and tenants, who must keep logging in on a password alone.
func TestMFAGate_OwnerWithNoFactorIsUnaffected(t *testing.T) {
	srv, _, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, _ := seedOfficeAdminUser(t, handlesDB, "mfa-session-owner-office-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Owner's Own Community")
	email := "mfa-session-owner@example.com"
	ownerID := createUser(t, handlesDB, email, false)
	seedUnitOwner(t, handlesDB, communityID, ownerID)

	resp, body := login(t, client, srv.URL, email, "", "web")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200: an owner with no second factor must still log in on a password alone, got %d body=%v", resp.StatusCode, body)
	}
	accessToken := accessTokenFrom(t, body)

	resp, body = doJSON(t, client, http.MethodGet, srv.URL+"/v1/communities/"+communityID.String(), nil, bearer(accessToken))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected an owner's password-only session to reach a community-scoped read, got %d body=%v", resp.StatusCode, body)
	}
}

// R1-self-scope-skips-mandatory-totp-gate (review lineage
// review-c4efc3f92d076299). ResolveSelf -- the resolver behind EVERY
// scoped.Self operation -- never called requireMFAForAdminRoles, so the
// gate the two previous rounds built did not run on that route class at
// all. POST /v1/offices/me/members is registered through scoped.Self and
// inserts an office_members row with role admin_staff, which grants the
// target office-wide reach into every community of that office through
// the community resolver's office leg. An admin who never enrolled
// authenticates on a password alone (the login challenge only fires for
// accounts that already have an active factor), so that session -- the
// exact caller the gate is specified to refuse -- could still perform
// the escalation.
func TestMFAGate_SelfScopedPrivilegeGrantRefusedWithoutSecondFactor(t *testing.T) {
	srv, _, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	adminEmail := "self-scope-gate-admin@example.com"
	_, _ = seedOfficeAdminUser(t, handlesDB, adminEmail)
	targetEmail := "self-scope-gate-target@example.com"
	targetID := createUser(t, handlesDB, targetEmail, false)

	// No factor on the account, so login succeeds on a password alone
	// and the session it issues is NOT second-factor authenticated.
	resp, body := login(t, client, srv.URL, adminEmail, "", "web")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("test setup: an admin with no factor must still log in on a password alone, got %d body=%v", resp.StatusCode, body)
	}
	auth := bearer(accessTokenFrom(t, body))

	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/offices/me/members", map[string]any{
		"email": targetEmail,
	}, auth)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403: a session that never proved a second factor must not grant office-wide admin_staff through a scoped.Self route, got %d body=%v", resp.StatusCode, body)
	}
	if body["code"] != "AUTH_MFA_ENROLLMENT_REQUIRED" {
		t.Fatalf("expected AUTH_MFA_ENROLLMENT_REQUIRED (this admin has no factor at all, so the client's move is to enroll), got %v", body)
	}
	if n := officeMemberCount(t, handlesDB, targetID); n != 0 {
		t.Fatalf("expected NO office_members row for the target: the privilege-granting write must not have happened, got %d", n)
	}

	// The same session must not read the office's member list either --
	// getMyOffices and listMyOfficeMembers are Self-registered too, and
	// they answer from the very membership set the gate governs.
	resp, body = doJSON(t, client, http.MethodGet, srv.URL+"/v1/offices/me/members", nil, auth)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 on the Self-scoped office member listing for a session with no second factor, got %d body=%v", resp.StatusCode, body)
	}
}

// The positive path the gate must not break: the SAME admin, with a
// factor, logging in WITH a valid code, completes the same write. Without
// this the test above would be satisfied by a route that refuses every
// admin unconditionally.
func TestMFAGate_SelfScopedPrivilegeGrantSucceedsWithSecondFactor(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	adminEmail := "self-scope-gate-ok-admin@example.com"
	_, adminID := seedOfficeAdminUser(t, handlesDB, adminEmail)
	secret := seedActiveMFAWithSecret(t, handlesDB, deps.MFAKey, adminID)
	targetEmail := "self-scope-gate-ok-target@example.com"
	targetID := createUser(t, handlesDB, targetEmail, false)

	resp, body := login(t, client, srv.URL, adminEmail, validTOTPCode(t, secret), "web")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected a valid TOTP code to complete the login, got %d body=%v", resp.StatusCode, body)
	}
	auth := bearer(accessTokenFrom(t, body))

	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/offices/me/members", map[string]any{
		"email": targetEmail,
	}, auth)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected a second-factor-authenticated office admin to add staff through the Self-scoped route, got %d body=%v", resp.StatusCode, body)
	}
	if n := officeMemberCount(t, handlesDB, targetID); n != 1 {
		t.Fatalf("expected exactly one office_members row for the target, got %d", n)
	}
}

func officeMemberCount(t *testing.T, handlesDB db.Handles, userID uuid.UUID) int {
	t.Helper()
	var count int
	if err := handlesDB.Write.QueryRow(t.Context(),
		`SELECT count(*) FROM office_members WHERE user_id = $1`, userID,
	).Scan(&count); err != nil {
		t.Fatalf("count office members: %v", err)
	}
	return count
}
