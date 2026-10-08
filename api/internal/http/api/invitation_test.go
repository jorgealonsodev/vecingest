package api_test

import (
	"bytes"
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
	// without duplicating the user.
	existingEmail := "invite-accept-existing@example.com"
	existingUserID := createUser(t, handlesDB, existingEmail, false)
	unitID2 := seedUnit(t, handlesDB, communityID)
	_, createBody2 := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID2.String(), "email": existingEmail, "role": "tenant",
	}, auth)
	code2, _ := createBody2["short_code"].(string)

	resp, body2 := doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, "203.0.113.71",
		map[string]any{
			"short_code": code2, "name": "Existing Account User", "password": "correct-horse-battery-staple-1",
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
