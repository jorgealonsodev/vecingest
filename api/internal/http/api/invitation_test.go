package api_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/lockout"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/password"
)

// uuidPgtype/pgtypeText adapt uuid.UUID/string to the pgtype.UUID/
// pgtype.Text shapes db.InsertInvitationParams needs for its nullable
// unit_id/email columns, mirroring handlers.optionalText's pattern.
func uuidPgtype(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func pgtypeText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}

// seedUnit inserts a bare unit (no member) under communityID, for
// invitation tests that need a unit as the invitation's target but no
// pre-existing membership.
func seedUnit(t *testing.T, handlesDB db.Handles, communityID uuid.UUID) uuid.UUID {
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
	return unit.ID
}

// doFromIPClient builds and sends a request with a caller-controlled
// resolved client IP (via X-Forwarded-For, matching the dev-mode
// ClientIPFromXFFTrustedProxies(1) fallback every other integration
// test in this package relies on) and arbitrary extra headers -- unlike
// newClient/doJSON, which always inject a FIXED simulated IP, this is
// needed to exercise the IP+device-scoped enumeration lockout across
// distinct simulated callers.
func doFromIPClient(t *testing.T, srvClient *http.Client, url, method, ip string, body any, headers map[string]string) (*http.Response, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Forwarded-For", "10.0.0.1, "+ip)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := srvClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var parsed map[string]any
	dec := json.NewDecoder(resp.Body)
	_ = dec.Decode(&parsed)
	return resp, parsed
}

// inviteHeaders is the mandatory device-fingerprint header pair every
// preview/accept call must carry (design D-6).
func inviteHeaders(platform, appVersion string) map[string]string {
	return map[string]string{"X-Platform": platform, "X-App-Version": appVersion}
}

// invitations: Invitation Creation With Hashed, One-Time-Visible Secrets
// (both scenarios).
func TestInvitation_CreationExposesShortCodeExactlyOnceAndStoresOnlyHashes(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-create-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Create Community")
	unitID := seedUnit(t, handlesDB, communityID)
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)

	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": "invitee-1@example.com", "role": "owner",
	}, map[string]string{"Authorization": "Bearer " + adminToken})
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected the admin's invitation creation to succeed, got %d body=%v", resp.StatusCode, body)
	}
	shortCode, _ := body["short_code"].(string)
	tokenStr, _ := body["token"].(string)
	if len(shortCode) != 8 {
		t.Fatalf("expected an 8-character short_code in the creation response, got %q", shortCode)
	}
	if tokenStr == "" {
		t.Fatalf("expected a plaintext token in the creation response, got %v", body)
	}
	invID, _ := body["id"].(string)
	if invID == "" {
		t.Fatalf("expected an invitation id, got %v", body)
	}

	// Stored invitation contains only hashes: no plaintext token or
	// short_code column at all (the row shape itself enforces this --
	// db.Invitation has TokenHash/ShortCodeHash, never a plaintext
	// field) and, defensively, neither hash byte slice equals the
	// plaintext.
	row, err := db.New(handlesDB.Write).GetInvitationByID(t.Context(), db.GetInvitationByIDParams{
		ID: uuid.MustParse(invID), CommunityID: communityID,
	})
	if err != nil {
		t.Fatalf("get invitation by id: %v", err)
	}
	if string(row.TokenHash) == tokenStr || string(row.ShortCodeHash) == shortCode {
		t.Fatalf("expected stored hashes to differ from the plaintext secrets")
	}

	// No later read of that invitation returns the plaintext again.
	resp, listBody := doJSON(t, client, http.MethodGet, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", nil,
		map[string]string{"Authorization": "Bearer " + adminToken})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing invitations, got %d body=%v", resp.StatusCode, listBody)
	}
	raw, _ := json.Marshal(listBody)
	if strings.Contains(string(raw), shortCode) || strings.Contains(string(raw), tokenStr) {
		t.Fatalf("expected the list response to never re-expose the plaintext secret, got: %s", raw)
	}
}

// invitations: Explicit Status Column (all three scenarios).
func TestInvitation_ExplicitStatusColumnAndReadTimeExpiry(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-status-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Status Community")
	unitID := seedUnit(t, handlesDB, communityID)
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	// Scenario 1: invitation created with status pending.
	_, createBody := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": "invite-status-1@example.com", "role": "owner",
	}, auth)
	invID, _ := createBody["id"].(string)
	shortCode, _ := createBody["short_code"].(string)
	if createBody["status"] != "pending" {
		t.Fatalf("expected status=pending on creation, got %v", createBody)
	}

	// Scenario 2: accept transitions status transactionally.
	resp, acceptBody := doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, "203.0.113.51",
		map[string]any{
			"short_code": shortCode, "name": "Invite Status User", "password": "correct-horse-battery-staple-1",
			"consent": true, "platform": "web",
		}, inviteHeaders("ios", "1.0.0"))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected accept to succeed, got %d body=%v", resp.StatusCode, acceptBody)
	}

	_, listBody := doJSON(t, client, http.MethodGet, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", nil, auth)
	invitations, _ := listBody["invitations"].([]any)
	found := false
	for _, raw := range invitations {
		m, _ := raw.(map[string]any)
		if m["id"] == invID {
			found = true
			if m["status"] != "accepted" {
				t.Fatalf("expected status=accepted after a successful accept, got %v", m["status"])
			}
		}
	}
	if !found {
		t.Fatalf("expected to find the accepted invitation in the list, got %v", invitations)
	}

	// Scenario 3: a pending invitation past expiry reads as expired
	// BEFORE the sweep job runs.
	_, createBody2 := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": "invite-status-2@example.com", "role": "tenant",
	}, auth)
	invID2, _ := createBody2["id"].(string)
	if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE invitations SET expires_at = now() - interval '1 day' WHERE id = $1", uuid.MustParse(invID2)); err != nil {
		t.Fatalf("force-expire invitation: %v", err)
	}

	_, listBody2 := doJSON(t, client, http.MethodGet, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", nil, auth)
	invitations2, _ := listBody2["invitations"].([]any)
	found = false
	for _, raw := range invitations2 {
		m, _ := raw.(map[string]any)
		if m["id"] == invID2 {
			found = true
			if m["status"] != "expired" {
				t.Fatalf("expected status=expired for a past-expiry pending invitation read before the sweep runs, got %v", m["status"])
			}
		}
	}
	if !found {
		t.Fatalf("expected to find the force-expired invitation in the list, got %v", invitations2)
	}
}

// invitations: Fourteen-Day Expiry And Single Use (both scenarios).
func TestInvitation_ExpiryAndSingleUseEnforced(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-expiry-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Expiry Community")
	unitID := seedUnit(t, handlesDB, communityID)
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	// Scenario 1: accept after expiry rejected.
	_, expiredCreate := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": "invite-expiry-1@example.com", "role": "owner",
	}, auth)
	expiredID, _ := expiredCreate["id"].(string)
	expiredCode, _ := expiredCreate["short_code"].(string)
	if _, err := handlesDB.Write.Exec(t.Context(), "UPDATE invitations SET expires_at = now() - interval '1 day' WHERE id = $1", uuid.MustParse(expiredID)); err != nil {
		t.Fatalf("force-expire invitation: %v", err)
	}
	resp, body := doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, "203.0.113.52",
		map[string]any{
			"short_code": expiredCode, "name": "Expired User", "password": "correct-horse-battery-staple-1",
			"consent": true, "platform": "web",
		}, inviteHeaders("ios", "1.0.0"))
	if resp.StatusCode < 400 {
		t.Fatalf("expected accept on an expired invitation to be rejected, got %d body=%v", resp.StatusCode, body)
	}

	// Scenario 2: second accept on an already-accepted invitation rejected.
	_, reuseCreate := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": "invite-expiry-2@example.com", "role": "owner",
	}, auth)
	reuseCode, _ := reuseCreate["short_code"].(string)

	resp, body = doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, "203.0.113.53",
		map[string]any{
			"short_code": reuseCode, "name": "Reuse User", "password": "correct-horse-battery-staple-1",
			"consent": true, "platform": "web",
		}, inviteHeaders("ios", "1.0.0"))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected the first accept to succeed, got %d body=%v", resp.StatusCode, body)
	}

	resp, body = doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, "203.0.113.54",
		map[string]any{
			"short_code": reuseCode, "name": "Reuse User Again", "password": "correct-horse-battery-staple-1",
			"consent": true, "platform": "web",
		}, inviteHeaders("android", "1.0.0"))
	if resp.StatusCode < 400 {
		t.Fatalf("expected a second accept on an already-accepted invitation to be rejected, got %d body=%v", resp.StatusCode, body)
	}
}

// invitations: Preview Endpoint Is POST, Never GET With A Query
// Credential (both scenarios).
func TestInvitation_PreviewIsPostOnlyAndNeverAGetQueryParameter(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-preview-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Preview Community")
	unitID := seedUnit(t, handlesDB, communityID)
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)

	_, createBody := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": "invite-preview-1@example.com", "role": "tenant",
	}, map[string]string{"Authorization": "Bearer " + adminToken})
	shortCode, _ := createBody["short_code"].(string)

	// Scenario 1: preview via POST body returns community/unit/role and
	// creates no account.
	resp, body := doFromIPClient(t, client, srv.URL+"/v1/invitations/preview", http.MethodPost, "203.0.113.60",
		map[string]any{"short_code": shortCode}, inviteHeaders("web", "1.0.0"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 previewing a valid invitation, got %d body=%v", resp.StatusCode, body)
	}
	if body["community_id"] != communityID.String() {
		t.Fatalf("expected community_id %s in the preview response, got %v", communityID, body["community_id"])
	}
	if body["role"] != "tenant" {
		t.Fatalf("expected role=tenant in the preview response, got %v", body["role"])
	}
	if _, ok := body["email"]; ok {
		t.Fatalf("expected the preview response to never include the invited email, got %v", body)
	}
	if _, err := db.New(handlesDB.Write).GetUserByEmail(t.Context(), "invite-preview-1@example.com"); err == nil {
		t.Fatalf("expected preview to create no account, but a user now exists for that email")
	}

	// Scenario 2: the registered API surface has no GET operation with
	// token/short_code as a query parameter.
	resp, err := client.Get(srv.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("fetch openapi.json: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var oapi map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&oapi); err != nil {
		t.Fatalf("decode openapi.json: %v", err)
	}
	paths, _ := oapi["paths"].(map[string]any)
	for path, rawItem := range paths {
		item, _ := rawItem.(map[string]any)
		getOp, ok := item["get"].(map[string]any)
		if !ok {
			continue
		}
		params, _ := getOp["parameters"].([]any)
		for _, rawParam := range params {
			p, _ := rawParam.(map[string]any)
			name, _ := p["name"].(string)
			in, _ := p["in"].(string)
			if in == "query" && (name == "token" || name == "short_code") {
				t.Fatalf("found a GET operation %q with %q as a query parameter -- forbidden by design D-6", path, name)
			}
		}
	}
}

// invitations: Accept Creates Or Links An Account Without Revealing
// Prior Existence (all three scenarios).
func TestInvitation_AcceptCreatesOrLinksAnAccount(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-accept-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Accept Community")
	unitID := seedUnit(t, handlesDB, communityID)
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	// Scenario 1: accept with no existing account creates user and
	// membership together.
	newEmail := "invite-accept-new@example.com"
	if _, err := db.New(handlesDB.Write).GetUserByEmail(t.Context(), newEmail); err == nil {
		t.Fatalf("test setup: expected no pre-existing account for %s", newEmail)
	}
	_, createBody1 := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": newEmail, "role": "owner",
	}, auth)
	code1, _ := createBody1["short_code"].(string)

	resp, body1 := doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, "203.0.113.70",
		map[string]any{
			"short_code": code1, "name": "New Account User", "password": "correct-horse-battery-staple-1",
			"consent": true, "platform": "ios",
		}, inviteHeaders("ios", "1.0.0"))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected accept (new account) to succeed, got %d body=%v", resp.StatusCode, body1)
	}
	newUser, err := db.New(handlesDB.Write).GetUserByEmail(t.Context(), newEmail)
	if err != nil {
		t.Fatalf("expected a new user to have been created for %s: %v", newEmail, err)
	}
	members, lerr := db.New(handlesDB.Write).ListUnitMembersByUnitID(t.Context(), db.ListUnitMembersByUnitIDParams{UnitID: unitID, CommunityID: communityID})
	if lerr != nil {
		t.Fatalf("list unit members: %v", lerr)
	}
	foundMember := false
	for _, m := range members {
		if m.UserID == newUser.ID {
			foundMember = true
		}
	}
	if !foundMember {
		t.Fatalf("expected a unit_member row linking the new user to the unit, got members: %v", members)
	}

	// Scenario 2: accept with an existing account links the membership
	// without duplicating the user. REWRITTEN as part of the review
	// lineage review-0e1833930adf141a correction: this scenario used to
	// assert that the linking branch succeeds on ANY policy-valid
	// password, which is exactly the account-takeover the review found.
	// The password field is now load-bearing -- a wrong one is refused
	// and mints nothing, and only the account's OWN password links it.
	existingEmail := "invite-accept-existing@example.com"
	existingUserID := createUser(t, handlesDB, existingEmail, false)
	unitID2 := seedUnit(t, handlesDB, communityID)
	_, createBody2 := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID2.String(), "email": existingEmail, "role": "tenant",
	}, auth)
	code2, _ := createBody2["short_code"].(string)

	resp, wrongBody := doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, "203.0.113.72",
		map[string]any{
			"short_code": code2, "name": "Existing Account User", "password": "a-different-valid-password-1",
			"consent": true, "platform": "ios",
		}, inviteHeaders("android", "2.0.0"))
	if resp.StatusCode < 400 || wrongBody["access_token"] != nil {
		t.Fatalf("expected accept for an existing account with the WRONG password to be refused with no session, got %d body=%v", resp.StatusCode, wrongBody)
	}

	resp, body2 := doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, "203.0.113.71",
		map[string]any{
			"short_code": code2, "name": "Existing Account User", "password": testPassword,
			"consent": true, "platform": "ios",
		}, inviteHeaders("android", "2.0.0"))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected accept (existing account) to succeed, got %d body=%v", resp.StatusCode, body2)
	}
	linkedUser, err := db.New(handlesDB.Write).GetUserByEmail(t.Context(), existingEmail)
	if err != nil || linkedUser.ID != existingUserID {
		t.Fatalf("expected the SAME existing user id %s, got %v (err=%v)", existingUserID, linkedUser, err)
	}

	// Scenario 3: both responses have the same shape.
	keys1 := make(map[string]bool)
	for k := range body1 {
		keys1[k] = true
	}
	keys2 := make(map[string]bool)
	for k := range body2 {
		keys2[k] = true
	}
	if len(keys1) != len(keys2) {
		t.Fatalf("expected both accept responses to share the same key set, got %v vs %v", keys1, keys2)
	}
	for k := range keys1 {
		if !keys2[k] {
			t.Fatalf("expected key %q present in both accept responses, missing from the existing-account response: %v", k, body2)
		}
	}
}

// invitations: Resend And Revoke (both scenarios).
func TestInvitation_ResendAndRevoke(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-resend-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Resend Community")
	unitID := seedUnit(t, handlesDB, communityID)
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	// Scenario 1: resend increments sent_count.
	_, createBody := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": "invite-resend-1@example.com", "role": "owner",
	}, auth)
	invID, _ := createBody["id"].(string)
	if createBody["sent_count"] != float64(1) {
		t.Fatalf("expected sent_count=1 on creation, got %v", createBody["sent_count"])
	}
	resp, resendBody := doJSON(t, client, http.MethodPost, srv.URL+"/v1/invitations/"+invID+"/resend", nil, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on resend, got %d body=%v", resp.StatusCode, resendBody)
	}
	if resendBody["sent_count"] != float64(2) {
		t.Fatalf("expected sent_count=2 after resend, got %v", resendBody)
	}

	// Scenario 2: revoked invitation fails both preview and accept.
	_, revokeCreate := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": "invite-resend-2@example.com", "role": "owner",
	}, auth)
	revokeID, _ := revokeCreate["id"].(string)
	revokeCode, _ := revokeCreate["short_code"].(string)

	resp, delBody := doJSON(t, client, http.MethodDelete, srv.URL+"/v1/invitations/"+revokeID, nil, auth)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 200/204 revoking an invitation, got %d body=%v", resp.StatusCode, delBody)
	}

	resp, previewBody := doFromIPClient(t, client, srv.URL+"/v1/invitations/preview", http.MethodPost, "203.0.113.80",
		map[string]any{"short_code": revokeCode}, inviteHeaders("web", "1.0.0"))
	if resp.StatusCode < 400 {
		t.Fatalf("expected preview of a revoked invitation to be rejected, got %d body=%v", resp.StatusCode, previewBody)
	}

	resp, acceptBody := doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, "203.0.113.81",
		map[string]any{
			"short_code": revokeCode, "name": "Revoked User", "password": "correct-horse-battery-staple-1",
			"consent": true, "platform": "web",
		}, inviteHeaders("web", "1.0.0"))
	if acceptBody != nil && acceptBody["access_token"] != nil {
		t.Fatalf("expected accept of a revoked invitation to be rejected, got a session: %v", acceptBody)
	}
	if resp.StatusCode < 400 {
		t.Fatalf("expected accept of a revoked invitation to be rejected, got %d body=%v", resp.StatusCode, acceptBody)
	}
}

// invitations: Enumeration Lockout Is IP+Device Scoped, Not
// Per-Invitation (both scenarios).
func TestInvitation_EnumerationLockoutIsIPAndDeviceScoped(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-lockout-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Lockout Community")
	unitID := seedUnit(t, handlesDB, communityID)
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	// Scenario 1: 10 failed guesses from ONE ip+device lock an 11th
	// attempt from that SAME pair, even against 10 DIFFERENT
	// nonexistent codes.
	lockedIP := "203.0.113.90"
	headers := inviteHeaders("ios", "9.9.9")
	var lastResp *http.Response
	for i := 0; i < 10; i++ {
		lastResp, _ = doFromIPClient(t, client, srv.URL+"/v1/invitations/preview", http.MethodPost, lockedIP,
			map[string]any{"short_code": fmt.Sprintf("NOCODE%02d", i)}, headers)
		if lastResp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 for guess %d against a nonexistent code, got %d", i, lastResp.StatusCode)
		}
	}
	resp, body := doFromIPClient(t, client, srv.URL+"/v1/invitations/preview", http.MethodPost, lockedIP,
		map[string]any{"short_code": "STILLBAD"}, headers)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on the 11th attempt from the same locked ip+device pair, got %d body=%v", resp.StatusCode, body)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Fatalf("expected a Retry-After header on the 429 response (design D-6), got none")
	}

	// Scenario 2: failed_attempts alone does not enforce the lockout.
	// Ten DISTINCT ip+device pairs each successfully resolve the SAME
	// real invitation (incrementing its failed_attempts evidence column
	// ten times); an 11th DISTINCT pair must NOT be locked out by that
	// per-invitation counter alone.
	_, createBody := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": "invite-lockout-target@example.com", "role": "owner",
	}, auth)
	invID, _ := createBody["id"].(string)
	targetCode, _ := createBody["short_code"].(string)

	for i := 0; i < 10; i++ {
		ip := fmt.Sprintf("203.0.113.%d", 100+i)
		resp, body := doFromIPClient(t, client, srv.URL+"/v1/invitations/preview", http.MethodPost, ip,
			map[string]any{"short_code": targetCode}, inviteHeaders("web", fmt.Sprintf("%d.0.0", i)))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected pair %d's preview of a real invitation to succeed, got %d body=%v", i, resp.StatusCode, body)
		}
	}

	row, err := db.New(handlesDB.Write).GetInvitationByID(t.Context(), db.GetInvitationByIDParams{ID: uuid.MustParse(invID), CommunityID: communityID})
	if err != nil {
		t.Fatalf("get invitation by id: %v", err)
	}
	if row.FailedAttempts < 10 {
		t.Fatalf("expected failed_attempts >= 10 after 10 resolved previews, got %d", row.FailedAttempts)
	}

	resp, body = doFromIPClient(t, client, srv.URL+"/v1/invitations/preview", http.MethodPost, "203.0.113.200",
		map[string]any{"short_code": targetCode}, inviteHeaders("web", "11.0.0"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected an 11th DISTINCT ip+device pair to NOT be locked out by the per-invitation failed_attempts counter alone, got %d body=%v", resp.StatusCode, body)
	}
}

// invitations: Cross-Tenant Isolation Proven By Test (both scenarios).
func TestInvitation_CrossTenantIsolation(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeA, adminA := seedOfficeWithAdmin(t, handlesDB, "invite-tenant-a-admin@example.com")
	communityA := seedCommunity(t, handlesDB, officeA, "Invite Tenant A")
	unitA := seedUnit(t, handlesDB, communityA)
	tokenA := mintAccessToken(t, deps, handlesDB, adminA, false)

	officeB, adminB := seedOfficeWithAdmin(t, handlesDB, "invite-tenant-b-admin@example.com")
	seedCommunity(t, handlesDB, officeB, "Invite Tenant B")
	tokenB := mintAccessToken(t, deps, handlesDB, adminB, false)

	_, createBody := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityA.String()+"/invitations", map[string]any{
		"unit_id": unitA.String(), "email": "invite-tenant-a-invitee@example.com", "role": "owner",
	}, map[string]string{"Authorization": "Bearer " + tokenA})
	invitationAID, _ := createBody["id"].(string)

	// Scenario 1: admin of community B cannot list community A's
	// invitations.
	resp, listBody := doJSON(t, client, http.MethodGet, srv.URL+"/v1/communities/"+communityA.String()+"/invitations", nil,
		map[string]string{"Authorization": "Bearer " + tokenB})
	if resp.StatusCode == http.StatusOK {
		invitations, _ := listBody["invitations"].([]any)
		for _, raw := range invitations {
			m, _ := raw.(map[string]any)
			if m["id"] == invitationAID {
				t.Fatalf("expected community A's invitation to never be visible to community B, got it in the list: %v", invitations)
			}
		}
	} else if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 403/404 (or a 200 with A's invitation absent) for B listing A's community, got %d body=%v", resp.StatusCode, listBody)
	}

	// Scenario 2: admin of community B cannot revoke community A's
	// invitation.
	resp, revokeBody := doJSON(t, client, http.MethodDelete, srv.URL+"/v1/invitations/"+invitationAID, nil,
		map[string]string{"Authorization": "Bearer " + tokenB})
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 403/404 for B revoking A's invitation, got %d body=%v", resp.StatusCode, revokeBody)
	}

	// The invitation must still be perfectly usable by A afterwards
	// (B's attempt had no side effect).
	row, err := db.New(handlesDB.Write).GetInvitationByID(t.Context(), db.GetInvitationByIDParams{ID: uuid.MustParse(invitationAID), CommunityID: communityA})
	if err != nil {
		t.Fatalf("get invitation by id: %v", err)
	}
	if row.Status != "pending" {
		t.Fatalf("expected community A's invitation to remain pending after B's rejected attempts, got status=%s", row.Status)
	}
}

// invitations / design D-6 task 6.17: the invitation-email job is
// enqueued in the SAME transaction that creates the invitation row (via
// river.InsertTx), never as a separate, potentially-lost side effect.
func TestInvitation_EmailJobEnqueuedInCreationTransaction(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-queue-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Queue Community")
	unitID := seedUnit(t, handlesDB, communityID)
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)

	var before int
	if err := handlesDB.Write.QueryRow(t.Context(), "SELECT count(*) FROM river_job WHERE kind = 'invitation_email'").Scan(&before); err != nil {
		t.Fatalf("count invitation_email jobs before: %v", err)
	}

	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": "invite-queue-1@example.com", "role": "owner",
	}, map[string]string{"Authorization": "Bearer " + adminToken})
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected invitation creation to succeed, got %d body=%v", resp.StatusCode, body)
	}

	var after int
	if err := handlesDB.Write.QueryRow(t.Context(), "SELECT count(*) FROM river_job WHERE kind = 'invitation_email'").Scan(&after); err != nil {
		t.Fatalf("count invitation_email jobs after: %v", err)
	}
	if after != before+1 {
		t.Fatalf("expected exactly one new invitation_email river_job row, before=%d after=%d", before, after)
	}
}

// invitations / design D-6, task 6.18/6.19: the daily invitations.expire
// sweep transitions past-expiry pending rows and leaves a still-valid
// pending row untouched.
func TestInvitation_DailySweepTransitionsExpiredPendingRows(t *testing.T) {
	_, deps, handlesDB := newTestServer(t)
	_ = deps

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-sweep-admin@example.com")
	_ = adminID
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Sweep Community")
	unitID := seedUnit(t, handlesDB, communityID)

	q := db.New(handlesDB.Write)
	expired, err := q.InsertInvitation(t.Context(), db.InsertInvitationParams{
		ID: uuid.New(), CommunityID: communityID, UnitID: uuidPgtype(unitID),
		Email: pgtypeText("invite-sweep-expired@example.com"), Role: "owner",
		TokenHash: []byte("sweep-expired-token-hash-000000"), ShortCodeHash: []byte("sweep-expired-code-hash-0000000"),
		ExpiresAt: time.Now().Add(-24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("insert expired invitation: %v", err)
	}
	future, err := q.InsertInvitation(t.Context(), db.InsertInvitationParams{
		ID: uuid.New(), CommunityID: communityID, UnitID: uuidPgtype(unitID),
		Email: pgtypeText("invite-sweep-future@example.com"), Role: "owner",
		TokenHash: []byte("sweep-future-token-hash-0000000"), ShortCodeHash: []byte("sweep-future-code-hash-00000000"),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("insert future invitation: %v", err)
	}

	affected, err := q.SweepExpiredInvitations(t.Context())
	if err != nil {
		t.Fatalf("sweep expired invitations: %v", err)
	}
	if affected < 1 {
		t.Fatalf("expected at least 1 row swept, got %d", affected)
	}

	expiredRow, err := q.GetInvitationByID(t.Context(), db.GetInvitationByIDParams{ID: expired.ID, CommunityID: communityID})
	if err != nil {
		t.Fatalf("get expired invitation: %v", err)
	}
	if expiredRow.Status != "blocked" {
		t.Fatalf("expected the past-expiry pending invitation to be swept to status=blocked, got %s", expiredRow.Status)
	}

	futureRow, err := q.GetInvitationByID(t.Context(), db.GetInvitationByIDParams{ID: future.ID, CommunityID: communityID})
	if err != nil {
		t.Fatalf("get future invitation: %v", err)
	}
	if futureRow.Status != "pending" {
		t.Fatalf("expected the still-valid pending invitation to remain untouched by the sweep, got %s", futureRow.Status)
	}
}

// invitations (security correction, review lineage
// review-0e1833930adf141a: R1-accept-invitation-account-takeover /
// R3-accept-links-existing-account-without-credential-proof). An
// invitation secret alone MUST NOT mint a session for a pre-existing
// account: invitation creation takes a fully caller-supplied email and
// hands the creating admin the plaintext short code back, so linking
// without credential proof turns any admin/admin_staff into a
// cross-tenant account-takeover primitive.
func TestInvitation_AcceptRequiresTheExistingAccountsOwnPassword(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-takeover-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Takeover Community")
	unitID := seedUnit(t, handlesDB, communityID)
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	victimEmail := "invite-takeover-victim@example.com"
	victimID := createUser(t, handlesDB, victimEmail, false)

	_, createBody := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": victimEmail, "role": "owner",
	}, auth)
	shortCode, _ := createBody["short_code"].(string)

	// The exploit: the invitation's creator accepts it themselves,
	// supplying a password of their own choosing.
	resp, body := doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, "203.0.113.60",
		map[string]any{
			"short_code": shortCode, "name": "Attacker", "password": "attacker-chosen-password-1",
			"consent": true, "platform": "ios",
		}, inviteHeaders("ios", "1.0.0"))
	if resp.StatusCode < 400 {
		t.Fatalf("expected accept for a pre-existing account WITHOUT that account's password to be rejected, got %d body=%v", resp.StatusCode, body)
	}
	if body["access_token"] != nil || body["refresh_token"] != nil {
		t.Fatalf("expected NO session to be minted for the victim account, got tokens in %v", body)
	}
	members, err := db.New(handlesDB.Write).ListUnitMembersByUnitID(t.Context(), db.ListUnitMembersByUnitIDParams{UnitID: unitID, CommunityID: communityID})
	if err != nil {
		t.Fatalf("list unit members: %v", err)
	}
	for _, m := range members {
		if m.UserID == victimID {
			t.Fatalf("expected the rejected accept to leave the victim account unlinked, got membership %v", m)
		}
	}

	// The rejected attempt rolled back rather than consuming the
	// invitation, so the real account owner can still accept it.
	resp, body = doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, "203.0.113.61",
		map[string]any{
			"short_code": shortCode, "name": "Victim", "password": testPassword,
			"consent": true, "platform": "ios",
		}, inviteHeaders("ios", "1.0.0"))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected accept with the account's OWN password to succeed, got %d body=%v", resp.StatusCode, body)
	}
	if body["access_token"] == nil {
		t.Fatalf("expected the legitimate owner's accept to still issue a session, got %v", body)
	}
}

// invitations (security correction, review lineage
// review-0e1833930adf141a: R3-enumeration-lockout-keyed-on-client-
// controlled-headers). The lockout key must be keyed on the address
// leg alone: the device leg is fully client-supplied, so including it
// let a single caller reset the counter on every request.
func TestInvitation_EnumerationLockoutIgnoresClientControlledDeviceHeaders(t *testing.T) {
	srv, _, _ := newTestServer(t)
	client := newClient(srv, nil)

	ip := "203.0.113.150"
	for i := 0; i < 10; i++ {
		resp, body := doFromIPClient(t, client, srv.URL+"/v1/invitations/preview", http.MethodPost, ip,
			map[string]any{"short_code": fmt.Sprintf("ROTATE%02d", i)},
			inviteHeaders("ios", fmt.Sprintf("%d.0.0", i)))
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 for guess %d against a nonexistent code, got %d body=%v", i, resp.StatusCode, body)
		}
	}

	resp, body := doFromIPClient(t, client, srv.URL+"/v1/invitations/preview", http.MethodPost, ip,
		map[string]any{"short_code": "ROTATE99"}, inviteHeaders("android", "99.0.0"))
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected the 11th attempt from the SAME address to be locked out despite a different X-Platform/X-App-Version pair on every request, got %d body=%v", resp.StatusCode, body)
	}
}

// R1-accept-invitation-bypasses-login-lockout (review lineage
// review-e72754dc7521b57a). POST /v1/auth/accept-invitation runs a full
// password check against a pre-existing account chosen freely by
// whoever created the invitation, and a wrong password rolls the whole
// accept back -- the invitation stays pending and the same short code
// is replayable indefinitely. Routing those failures through the
// per-address invitation counter alone leaves an unauthenticated
// password-guessing oracle that never locks the victim's account and
// never raises the alert the login path raises. Repeated wrong
// passwords here MUST lock the account exactly as they would through
// POST /v1/auth/login.
func TestInvitation_AcceptWrongPasswordLocksTheAccountLikeLogin(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-lockout-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Lockout Community")
	unitID := seedUnit(t, handlesDB, communityID)
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)

	// The victim is an arbitrary pre-existing account: the creation DTO
	// takes the email verbatim, so the attacker picks the target.
	victimEmail := "invite-lockout-victim@example.com"
	createUser(t, handlesDB, victimEmail, false)

	_, createBody := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": victimEmail, "role": "owner",
	}, map[string]string{"Authorization": "Bearer " + adminToken})
	shortCode, _ := createBody["short_code"].(string)
	if shortCode == "" {
		t.Fatalf("test setup: expected a short_code from invitation creation, got %v", createBody)
	}

	const attackerIP = "203.0.113.90"
	accept := func(pw string) (*http.Response, map[string]any) {
		return doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, attackerIP,
			map[string]any{
				"short_code": shortCode, "name": "Guesser", "password": pw,
				"consent": true, "platform": "ios",
			}, inviteHeaders("ios", "1.0.0"))
	}

	// Exactly the login path's own budget of wrong guesses, replaying
	// the one short code (each failure rolls the accept back).
	for i := range lockout.Threshold {
		resp, body := accept(fmt.Sprintf("wrong-password-guess-number-%d", i))
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("guess %d: expected 401 invalid credentials, got %d body=%v", i+1, resp.StatusCode, body)
		}
	}

	// The victim's ACCOUNT must now be locked, exactly as it would be
	// after the same number of failed logins -- proven from a DIFFERENT
	// client address, so this is the email-scoped leg of d.Lockout and
	// not merely the attacker's own IP budget.
	resp, body := doFromIPClient(t, client, srv.URL+"/v1/auth/login", http.MethodPost, "203.0.113.91",
		map[string]any{"email": victimEmail, "password": testPassword, "platform": "web"}, nil)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected login with the CORRECT password to be locked out (429) after %d wrong passwords through accept-invitation, got %d body=%v", lockout.Threshold, resp.StatusCode, body)
	}

	// And accept-invitation itself must honour that same lock, or the
	// oracle simply continues on the endpoint that opened it.
	resp, body = accept(testPassword)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected accept-invitation to refuse a locked-out account with 429, got %d body=%v", resp.StatusCode, body)
	}
}

// R1-plaintext-short-code-persisted-in-job-row (review lineage
// review-e72754dc7521b57a). The invitation email job is inserted with
// river.InsertTx inside the creation transaction, so its payload is a
// DURABLE row in river_job -- pending, and retained after completion.
// Carrying the short code there in plaintext hands a directly usable
// credential to anyone with SELECT on that table, a database backup or
// a read replica, which is exactly the guarantee
// invitations.short_code_hash exists to provide. The email genuinely
// needs the plaintext to send it, so the payload carries it SEALED with
// ENCRYPTION_KEY (the same AES-256-GCM primitive user_mfa's TOTP
// secrets use) and the worker opens it at send time.
func TestInvitation_JobRowNeverPersistsThePlaintextShortCode(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-jobrow-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Job Row Community")
	unitID := seedUnit(t, handlesDB, communityID)
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)

	_, createBody := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": "invite-jobrow@example.com", "role": "owner",
	}, map[string]string{"Authorization": "Bearer " + adminToken})
	shortCode, _ := createBody["short_code"].(string)
	if shortCode == "" {
		t.Fatalf("test setup: expected a short_code from invitation creation, got %v", createBody)
	}

	var args []byte
	if err := handlesDB.Write.QueryRow(t.Context(),
		`SELECT args FROM river_job WHERE kind = 'invitation_email' ORDER BY id DESC LIMIT 1`,
	).Scan(&args); err != nil {
		t.Fatalf("read the persisted invitation_email job row: %v", err)
	}

	if strings.Contains(string(args), shortCode) {
		t.Fatalf("the persisted river_job row holds the plaintext short code, a directly usable credential: %s", args)
	}

	// ... and it must still be the REAL code, sealed -- dropping it
	// would satisfy the assertion above while silently breaking every
	// invitation email.
	var payload struct {
		ShortCodeEncrypted []byte `json:"short_code_encrypted"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		t.Fatalf("decode the persisted job payload: %v", err)
	}
	opened, err := mfa.DecryptSecret(testEncryptionKey, payload.ShortCodeEncrypted)
	if err != nil {
		t.Fatalf("expected the persisted job payload to carry the short code sealed under ENCRYPTION_KEY: %v", err)
	}
	if string(opened) != shortCode {
		t.Fatalf("expected the sealed payload to open to the issued short code %q, got %q", shortCode, opened)
	}
}

// R1-invitation-short-code-hash-offline-recoverable (review lineage
// review-c4efc3f92d076299). The short code is 8 symbols from a 32-symbol
// alphabet -- about 40 bits -- and it was persisted as a single-round
// unsalted SHA-256 digest under a UNIQUE index. Full enumeration of that
// preimage space is minutes of commodity GPU work, so the digest
// protected nothing against an adversary who can read the invitations
// table: a backup, a read replica, or anyone with SELECT. That is the
// SAME adversary this candidate names when it seals the very same short
// code before letting it reach a durable river_job row, and recovering
// one pending code is enough on its own -- accept resolves by short code
// and issues a session plus a membership with no second secret.
func TestInvitation_ShortCodeDigestIsNotOfflineEnumerable(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-digest-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Digest Community")
	unitID := seedUnit(t, handlesDB, communityID)
	auth := map[string]string{"Authorization": "Bearer " + mintAccessToken(t, deps, handlesDB, adminID, false)}

	_, createBody := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": "invite-digest@example.com", "role": "owner",
	}, auth)
	shortCode, _ := createBody["short_code"].(string)
	if shortCode == "" {
		t.Fatalf("test setup: expected a short code in the creation response, got %v", createBody)
	}

	var stored []byte
	if err := handlesDB.Write.QueryRow(t.Context(),
		`SELECT short_code_hash FROM invitations WHERE community_id = $1`, communityID,
	).Scan(&stored); err != nil {
		t.Fatalf("read short_code_hash: %v", err)
	}

	// The exact computation an attacker holding nothing but the table
	// would run over a 32^8 candidate list.
	bare := sha256.Sum256([]byte(shortCode))
	if bytes.Equal(stored, bare[:]) {
		t.Fatalf("invitations.short_code_hash is a bare unsalted SHA-256 of the short code: ~40 bits of entropy is offline-enumerable, so anyone who can read this table recovers every pending invitation's plaintext code")
	}

	// It is keyed on ENCRYPTION_KEY specifically -- a secret that lives
	// in the process environment and never in the database, which is the
	// whole point. Under a different key the same code no longer
	// resolves, so the digest cannot have been computed from the code
	// alone.
	deps.MFAKey = sha256.Sum256([]byte("a different ENCRYPTION_KEY entirely"))
	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/invitations/preview", map[string]any{
		"short_code": shortCode,
	}, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected the short-code digest to be keyed on ENCRYPTION_KEY (a different key must not resolve the same code), got %d body=%v", resp.StatusCode, body)
	}
}

// ============================================================================
// Review lineage review-f855997b550a986d. The three findings below all land
// on AcceptInvitation, and all three are ordering findings, so they are
// written as ordering assertions: each one names a step that must not have
// run yet when the response was produced.
// ============================================================================

// R1-accept-invitation-runs-argon2id-before-any-validity-check and
// R3-accept-invitation-status-checked-only-after-credential-work are the same
// defect seen from two sides: AcceptInvitation never called invitationUsable,
// so a dead invitation still drove the full credential leg -- the password
// policy (which reaches HIBP over the network in the production wiring) and
// then Argon2id -- on an unauthenticated endpoint, and status was enforced
// only by the conditional UPDATE much later.
//
// The password below is 13 characters: long enough for the DTO's static
// minLength:12, short enough to fail password.PasswordPolicy's 15-character
// no-MFA floor. That makes the policy OBSERVABLE in the response. Against a
// revoked invitation the endpoint must answer 404 (the invitation is dead), so
// a 422 AUTH_PASSWORD_TOO_SHORT_NO_MFA is proof the credential leg ran first.
func TestInvitation_AcceptDoesNoCredentialWorkForADeadInvitation(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-order-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Ordering Community")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	// policyViolating passes the huma schema (12) and fails the policy (15).
	const policyViolating = "short-pw-123x"

	// Branch 1: the invited address has NO account, so the dead invitation
	// used to reach password.Hash -- one Argon2id per replay, unbounded.
	newCode := revokedInvitationCode(t, client, srv.URL, communityID, seedUnit(t, handlesDB, communityID), "invite-order-new@example.com", auth)
	resp, body := doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, "203.0.113.120",
		map[string]any{
			"short_code": newCode, "name": "Ordering New", "password": policyViolating,
			"consent": true, "platform": "ios",
		}, inviteHeaders("ios", "1.0.0"))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected a revoked invitation to be refused 404 BEFORE any password work runs, got %d body=%v", resp.StatusCode, body)
	}

	// Branch 2: the invited address HAS an account, which is the branch the
	// existing revoked-accept assertion never exercised. A wrong password
	// here must not reach password.Verify at all, so it must not record an
	// account-lockout failure against the invited address either: revocation
	// has to terminate the guessing path, not merely fail it later.
	existingEmail := "invite-order-existing@example.com"
	createUser(t, handlesDB, existingEmail, false)
	existingCode := revokedInvitationCode(t, client, srv.URL, communityID, seedUnit(t, handlesDB, communityID), existingEmail, auth)

	const guesserIP = "203.0.113.121"
	for i := range lockout.Threshold {
		resp, body = doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, guesserIP,
			map[string]any{
				"short_code": existingCode, "name": "Ordering Existing", "password": fmt.Sprintf("wrong-password-guess-number-%d", i),
				"consent": true, "platform": "ios",
			}, inviteHeaders("ios", "1.0.0"))
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("guess %d: expected a revoked invitation to be refused 404 on the LINKING branch too, got %d body=%v", i+1, resp.StatusCode, body)
		}
	}

	// The victim's account must be untouched: a revoked invitation can no
	// longer be used to lock anyone out.
	resp, body = doFromIPClient(t, client, srv.URL+"/v1/auth/login", http.MethodPost, "203.0.113.122",
		map[string]any{"email": existingEmail, "password": testPassword, "platform": "web"}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected the invited account to still log in normally: a REVOKED invitation must not drive its lockout, got %d body=%v", resp.StatusCode, body)
	}
}

// revokedInvitationCode creates an invitation for email, revokes it, and
// returns its (now dead) short code.
func revokedInvitationCode(t *testing.T, client *http.Client, srvURL string, communityID, unitID uuid.UUID, email string, auth map[string]string) string {
	t.Helper()
	_, created := doJSON(t, client, http.MethodPost, srvURL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": email, "role": "owner",
	}, auth)
	id, _ := created["id"].(string)
	code, _ := created["short_code"].(string)
	if id == "" || code == "" {
		t.Fatalf("test setup: expected an invitation id and short_code, got %v", created)
	}
	resp, body := doJSON(t, client, http.MethodDelete, srvURL+"/v1/invitations/"+id, nil, auth)
	if resp.StatusCode >= 400 {
		t.Fatalf("test setup: revoke invitation: %d body=%v", resp.StatusCode, body)
	}
	return code
}

// R1-accept-invitation-mints-a-session-without-the-totp-challenge. Accept's
// linking branch verified a password and issued a full session while
// POST /v1/auth/login refused exactly that for the same account. Two doors,
// one bypass: the invitation's creator picks the invited email freely and
// reads the plaintext short code out of the creation response, so an attacker
// holding a victim's password but not the victim's authenticator got a working
// session here instead of the 403 login returns.
func TestInvitation_AcceptChallengesTheSecondFactorExactlyLikeLogin(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-totp-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite TOTP Community")
	unitID := seedUnit(t, handlesDB, communityID)
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	victimEmail := "invite-totp-victim@example.com"
	victimID := createUser(t, handlesDB, victimEmail, false)
	secret := seedActiveMFAWithSecret(t, handlesDB, deps.MFAKey, victimID)

	// Login already refuses this exact request. Accept must agree.
	resp, body := login(t, client, srv.URL, victimEmail, "", "web")
	if resp.StatusCode != http.StatusForbidden || body["code"] != "AUTH_MFA_REQUIRED" {
		t.Fatalf("test premise: login must refuse a password-only login for this account, got %d body=%v", resp.StatusCode, body)
	}

	_, created := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": victimEmail, "role": "owner",
	}, auth)
	code, _ := created["short_code"].(string)

	accept := func(totpCode, ip string) (*http.Response, map[string]any) {
		body := map[string]any{
			"short_code": code, "name": "TOTP Victim", "password": testPassword,
			"consent": true, "platform": "web",
		}
		if totpCode != "" {
			body["totp_code"] = totpCode
		}
		return doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, ip, body, inviteHeaders("web", "1.0.0"))
	}

	resp, body = accept("", "203.0.113.130")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected accept-invitation to refuse a password-only accept for an account with an ACTIVE second factor, exactly as login does, got %d body=%v", resp.StatusCode, body)
	}
	if body["code"] != "AUTH_MFA_REQUIRED" {
		t.Fatalf("expected AUTH_MFA_REQUIRED, the same code login returns for the same account, got %v", body)
	}
	if body["access_token"] != nil {
		t.Fatalf("expected NO session minted for a password-only accept, got %v", body)
	}
	if n := liveSessionCount(t, handlesDB, victimID); n != 0 {
		t.Fatalf("expected NO session row for an accept rejected at the TOTP challenge, got %d", n)
	}

	// And the invitation must survive the refusal, or the genuine owner
	// could never complete it.
	resp, body = accept(validTOTPCode(t, secret), "203.0.113.131")
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected accept WITH a valid code to succeed, got %d body=%v", resp.StatusCode, body)
	}
	if body["access_token"] == nil {
		t.Fatalf("expected a session once the second factor was presented, got %v", body)
	}
}

// R3-resend-invitation-dispatches-nothing. Resend incremented sent_count,
// wrote an audit row, returned 200 and dispatched nothing, on the one endpoint
// whose entire purpose is delivery -- and sent_count is the operator-visible
// evidence field, so it recorded sends that never happened.
//
// The chosen fix ROTATES the short code (see the handler comment for why the
// alternative -- persisting a recoverable plaintext -- was rejected), so this
// asserts all three halves of that contract: a job is enqueued, the new code
// works, and the superseded one does not.
func TestInvitation_ResendRotatesTheCodeAndActuallyDispatches(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-redispatch-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Redispatch Community")
	unitID := seedUnit(t, handlesDB, communityID)
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	_, created := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": "invite-redispatch@example.com", "role": "owner",
	}, auth)
	invID, _ := created["id"].(string)
	originalCode, _ := created["short_code"].(string)

	var before int
	if err := handlesDB.Write.QueryRow(t.Context(), "SELECT count(*) FROM river_job WHERE kind = 'invitation_email'").Scan(&before); err != nil {
		t.Fatalf("count invitation_email jobs before resend: %v", err)
	}

	resp, resent := doJSON(t, client, http.MethodPost, srv.URL+"/v1/invitations/"+invID+"/resend", nil, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on resend, got %d body=%v", resp.StatusCode, resent)
	}

	var after int
	if err := handlesDB.Write.QueryRow(t.Context(), "SELECT count(*) FROM river_job WHERE kind = 'invitation_email'").Scan(&after); err != nil {
		t.Fatalf("count invitation_email jobs after resend: %v", err)
	}
	if after != before+1 {
		t.Fatalf("expected resend to enqueue exactly one invitation_email job: sent_count is operator-visible evidence of a send, before=%d after=%d", before, after)
	}

	newCode, _ := resent["short_code"].(string)
	if newCode == "" {
		t.Fatalf("expected resend to return the newly issued short code so the paper/voice delivery path still works, got %v", resent)
	}
	if newCode == originalCode {
		t.Fatalf("expected resend to issue a NEW short code, got the same one back")
	}

	resp, body := doFromIPClient(t, client, srv.URL+"/v1/invitations/preview", http.MethodPost, "203.0.113.140",
		map[string]any{"short_code": originalCode}, inviteHeaders("web", "1.0.0"))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected the SUPERSEDED short code to stop working after a resend, got %d body=%v", resp.StatusCode, body)
	}

	resp, body = doFromIPClient(t, client, srv.URL+"/v1/invitations/preview", http.MethodPost, "203.0.113.141",
		map[string]any{"short_code": newCode}, inviteHeaders("web", "1.0.0"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected the newly issued short code to resolve the invitation, got %d body=%v", resp.StatusCode, body)
	}
}

// createUserWithPassword is createUser with a caller-chosen password, for the
// tests that need an existing credential the CURRENT policy would reject
// (shorter than its no-MFA floor), as an account legitimately has when it
// set that password under TOTP or before the policy last tightened.
func createUserWithPassword(t *testing.T, handlesDB db.Handles, email, pw string) uuid.UUID {
	t.Helper()
	hash, err := password.Hash(pw)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := db.New(handlesDB.Write).InsertUser(t.Context(), db.InsertUserParams{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: hash,
		Name:         "Test User",
		Locale:       "es",
	})
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return user.ID
}

// R3-accept-invitation-applies-password-policy-to-an-existing-credential.
// The linking branch VERIFIES an existing credential; it never sets one. So
// today's policy has no business re-litigating it: a 12-14 character password
// is legitimate for an account with TOTP active (and was set under exactly
// that rule), and an account whose password predates a policy change -- or
// has since appeared in HIBP -- must still be able to link with it. Running
// the policy (with totpActive hardcoded false) before the account lookup
// locked every such owner out of every invitation, even with the right
// password and the right code.
func TestInvitation_AcceptDoesNotPolicyCheckAnExistingAccountsCorrectPassword(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-legacy-pw-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Legacy Password Community")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	// 13 characters: above the with-MFA floor, below the no-MFA one.
	const shortButLegitimate = "thirteen-char"
	if n := len(shortButLegitimate); n < password.MinLengthWithMFA || n >= password.MinLengthNoMFA {
		t.Fatalf("test premise: want a password in [%d, %d), got %d chars", password.MinLengthWithMFA, password.MinLengthNoMFA, n)
	}

	invite := func(email string) string {
		unitID := seedUnit(t, handlesDB, communityID)
		_, created := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
			"unit_id": unitID.String(), "email": email, "role": "owner",
		}, auth)
		code, _ := created["short_code"].(string)
		if code == "" {
			t.Fatalf("test setup: expected a short_code from invitation creation, got %v", created)
		}
		return code
	}
	accept := func(code, totpCode, ip string) (*http.Response, map[string]any) {
		body := map[string]any{
			"short_code": code, "name": "Legacy Password Owner", "password": shortButLegitimate,
			"consent": true, "platform": "web",
		}
		if totpCode != "" {
			body["totp_code"] = totpCode
		}
		return doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, ip, body, inviteHeaders("web", "1.0.0"))
	}

	// With TOTP active: the policy itself allows this password.
	totpEmail := "invite-legacy-pw-totp@example.com"
	totpUserID := createUserWithPassword(t, handlesDB, totpEmail, shortButLegitimate)
	secret := seedActiveMFAWithSecret(t, handlesDB, deps.MFAKey, totpUserID)
	resp, body := accept(invite(totpEmail), validTOTPCode(t, secret), "203.0.113.140")
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected an account with TOTP active to link with its own correct 13-char password and a valid code, got %d body=%v", resp.StatusCode, body)
	}
	if body["access_token"] == nil {
		t.Fatalf("expected a session for the linked TOTP account, got %v", body)
	}

	// Without TOTP: today's policy would refuse this password if it were
	// being SET, but it is only being verified, so it must still link.
	plainEmail := "invite-legacy-pw-plain@example.com"
	createUserWithPassword(t, handlesDB, plainEmail, shortButLegitimate)
	resp, body = accept(invite(plainEmail), "", "203.0.113.141")
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected an existing account to link with its own correct password regardless of today's policy, got %d body=%v", resp.StatusCode, body)
	}
}

// The other half of the same correction: dropping the policy from the
// linking branch must not reopen the prior-existence disclosure the step-3
// comment guards. A policy-violating password that does NOT match the
// account must answer exactly what the create branch answers for the same
// password -- the same 422, code and details -- while a policy-compliant
// wrong password still answers 401.
func TestInvitation_AcceptPolicyViolationIsIndistinguishableAcrossBranches(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-parity-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Parity Community")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	invite := func(email string) string {
		unitID := seedUnit(t, handlesDB, communityID)
		_, created := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
			"unit_id": unitID.String(), "email": email, "role": "owner",
		}, auth)
		code, _ := created["short_code"].(string)
		if code == "" {
			t.Fatalf("test setup: expected a short_code from invitation creation, got %v", created)
		}
		return code
	}
	accept := func(code, pw, ip string) (*http.Response, map[string]any) {
		return doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, ip,
			map[string]any{
				"short_code": code, "name": "Parity", "password": pw,
				"consent": true, "platform": "web",
			}, inviteHeaders("web", "1.0.0"))
	}

	// Passes the request schema's static 12-char floor, fails the policy.
	const tooShort = "short-wrong1"

	unknownEmail := "invite-parity-unknown@example.com"
	existingEmail := "invite-parity-existing@example.com"
	createUser(t, handlesDB, existingEmail, false)

	unknownResp, unknownBody := accept(invite(unknownEmail), tooShort, "203.0.113.150")
	existingCode := invite(existingEmail)
	existingResp, existingBody := accept(existingCode, tooShort, "203.0.113.151")

	if unknownResp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("test premise: expected the create branch to refuse a policy-violating password with 422, got %d body=%v", unknownResp.StatusCode, unknownBody)
	}
	if existingResp.StatusCode != unknownResp.StatusCode {
		t.Fatalf("expected a policy-violating WRONG password to answer the same status for an existing account as for an unknown one (%d), got %d body=%v", unknownResp.StatusCode, existingResp.StatusCode, existingBody)
	}
	if existingBody["code"] != unknownBody["code"] {
		t.Fatalf("expected the same error code on both branches, got existing=%v unknown=%v", existingBody["code"], unknownBody["code"])
	}
	if existingBody["message"] != unknownBody["message"] {
		t.Fatalf("expected the same error message on both branches, got existing=%v unknown=%v", existingBody["message"], unknownBody["message"])
	}
	if fmt.Sprint(existingBody["details"]) != fmt.Sprint(unknownBody["details"]) {
		t.Fatalf("expected the same error details on both branches, got existing=%v unknown=%v", existingBody["details"], unknownBody["details"])
	}
	if existingBody["access_token"] != nil {
		t.Fatalf("expected NO session for a wrong password, got %v", existingBody)
	}

	// A policy-compliant wrong password is still plain invalid credentials.
	resp, body := accept(existingCode, "a-compliant-but-wrong-password", "203.0.113.152")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a policy-compliant wrong password on an existing account, got %d body=%v", resp.StatusCode, body)
	}
}
