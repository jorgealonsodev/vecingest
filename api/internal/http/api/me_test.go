package api_test

import (
	"net/http"
	"net/http/cookiejar"
	"reflect"
	"sort"
	"testing"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/db"
)

// getMe calls GET /v1/me with token and returns the decoded body, failing
// the test on any non-200.
func getMe(t *testing.T, client *http.Client, srvURL, token string) map[string]any {
	t.Helper()
	resp, body := doJSON(t, client, http.MethodGet, srvURL+"/v1/me", nil, bearer(token))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on GET /v1/me, got %d body=%v", resp.StatusCode, body)
	}
	return body
}

// membershipEntries extracts body["memberships"] as a slice of maps. It
// fails the test when the field is absent or not a JSON array -- an
// absent field and an empty array are different wire shapes, and the
// spec requires the array.
func membershipEntries(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, ok := body["memberships"]
	if !ok {
		t.Fatalf("expected a memberships field in GET /v1/me, got %v", body)
	}
	list, ok := raw.([]any)
	if !ok {
		t.Fatalf("expected memberships to be a JSON array, got %T (%v)", raw, raw)
	}
	out := make([]map[string]any, 0, len(list))
	for _, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			t.Fatalf("expected each membership to be an object, got %T", e)
		}
		out = append(out, m)
	}
	return out
}

// membershipKeys renders entries as sorted "scope|id|name|role" strings
// so assertions compare the whole set independent of order.
func membershipKeys(entries []map[string]any) []string {
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, e["scope"].(string)+"|"+e["id"].(string)+"|"+e["name"].(string)+"|"+e["role"].(string))
	}
	sort.Strings(keys)
	return keys
}

// seedOfficeMember inserts an office and a member row with role for
// userID, returning the office id.
func seedOfficeMember(t *testing.T, handlesDB db.Handles, userID uuid.UUID, officeName, role string) uuid.UUID {
	t.Helper()
	q := db.New(handlesDB.Write)
	office, err := q.InsertOffice(t.Context(), db.InsertOfficeParams{
		ID: uuid.New(), Name: officeName, Cif: uuid.NewString()[:8],
	})
	if err != nil {
		t.Fatalf("insert office: %v", err)
	}
	if _, err := q.InsertOfficeMember(t.Context(), db.InsertOfficeMemberParams{
		ID: uuid.New(), OfficeID: office.ID, UserID: userID, Role: role,
	}); err != nil {
		t.Fatalf("insert office member: %v", err)
	}
	return office.ID
}

// user-profile: GET /v1/me Response Shape -- all three scenarios, plus
// the bootstrap invariant authz.ResolveSelf's doc comment names: an
// admin with no second factor must still read their own profile.
func TestMe_MembershipsResponseShape(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	t.Run("non-superadmin with no memberships gets an empty array", func(t *testing.T) {
		userID := createUser(t, handlesDB, "me-nobody@example.com", false)
		body := getMe(t, client, srv.URL, mintAccessToken(t, deps, handlesDB, userID, false))
		if body["is_superadmin"] != false {
			t.Fatalf("expected is_superadmin false, got %v", body)
		}
		if body["id"] != userID.String() || body["email"] != "me-nobody@example.com" {
			t.Fatalf("expected the caller's own id and email, got %v", body)
		}
		if entries := membershipEntries(t, body); len(entries) != 0 {
			t.Fatalf("expected an empty memberships array, got %v", entries)
		}
	})

	t.Run("superadmin gets is_superadmin true", func(t *testing.T) {
		userID := createUser(t, handlesDB, "me-root@example.com", true)
		body := getMe(t, client, srv.URL, mintAccessToken(t, deps, handlesDB, userID, true))
		if body["is_superadmin"] != true {
			t.Fatalf("expected is_superadmin true, got %v", body)
		}
		if entries := membershipEntries(t, body); len(entries) != 0 {
			t.Fatalf("expected a superadmin with no rows to get an empty memberships array, got %v", entries)
		}
	})

	t.Run("one office and one community membership are both typed entries", func(t *testing.T) {
		userID := createUser(t, handlesDB, "me-mixed@example.com", false)
		seedActiveMFA(t, handlesDB, userID)
		officeID := seedOfficeMember(t, handlesDB, userID, "Fincas Norte", "admin_staff")

		// The community belongs to an UNRELATED office, so the community
		// entry can only come from the unit_members leg.
		otherOffice, _ := seedOfficeWithAdmin(t, handlesDB, "me-mixed-other-admin@example.com")
		communityID := seedCommunity(t, handlesDB, otherOffice, "Calle Mayor 7")
		seedUnitOwner(t, handlesDB, communityID, userID)
		// A second unit owned in the same community must not produce a
		// second, identical entry.
		seedUnitOwner(t, handlesDB, communityID, userID)

		body := getMe(t, client, srv.URL, mintAccessToken(t, deps, handlesDB, userID, false))
		got := membershipKeys(membershipEntries(t, body))
		want := []string{
			"community|" + communityID.String() + "|Calle Mayor 7|owner",
			"office|" + officeID.String() + "|Fincas Norte|admin_staff",
		}
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected memberships:\n got  %v\n want %v", got, want)
		}
	})

	t.Run("admin without a second factor still reads their memberships", func(t *testing.T) {
		officeID, adminID := seedOfficeAdminUser(t, handlesDB, "me-unenrolled-admin@example.com")
		body := getMe(t, client, srv.URL, mintAccessToken(t, deps, handlesDB, adminID, false))
		got := membershipKeys(membershipEntries(t, body))
		want := []string{"office|" + officeID.String() + "|MFA Session Gate Office|admin"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected memberships:\n got  %v\n want %v", got, want)
		}
	})
}

// user-profile: Remote Session Listing and Revocation Endpoints --
// "Revoke a session leaves memberships unchanged". Two real logins give
// the caller two session families; revoking the second through the
// first must leave the first's GET /v1/me memberships byte-for-byte as
// they were.
func TestMe_RevokingASessionLeavesMembershipsUnchanged(t *testing.T) {
	srv, _, handlesDB := newTestServer(t)
	email := "me-revoke@example.com"
	userID := createUser(t, handlesDB, email, false)
	officeID, _ := seedOfficeWithAdmin(t, handlesDB, "me-revoke-office-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Plaza Sol 3")
	seedUnitOwner(t, handlesDB, communityID, userID)

	jarA, _ := cookiejar.New(nil)
	jarB, _ := cookiejar.New(nil)
	clientA := newClient(srv, jarA)
	clientB := newClient(srv, jarB)

	resp, body := login(t, clientA, srv.URL, email, "", "web")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login A: expected 200, got %d body=%v", resp.StatusCode, body)
	}
	tokenA := accessTokenFrom(t, body)
	resp, body = login(t, clientB, srv.URL, email, "", "web")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login B: expected 200, got %d body=%v", resp.StatusCode, body)
	}
	tokenB := accessTokenFrom(t, body)

	before := membershipEntries(t, getMe(t, clientA, srv.URL, tokenA))
	if len(before) != 1 {
		t.Fatalf("expected exactly one membership before revocation, got %v", before)
	}

	resp, body = doJSON(t, clientB, http.MethodGet, srv.URL+"/v1/me/sessions", nil, bearer(tokenB))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list sessions B: expected 200, got %d body=%v", resp.StatusCode, body)
	}
	sessions, _ := body["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("expected B to see exactly its own session, got %v", body)
	}
	familyB, _ := sessions[0].(map[string]any)["family_id"].(string)

	resp, body = doJSON(t, clientA, http.MethodDelete, srv.URL+"/v1/me/sessions/"+familyB, nil, bearer(tokenA))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke B through A: expected 204, got %d body=%v", resp.StatusCode, body)
	}

	// The session really is revoked...
	resp, _ = doJSON(t, clientB, http.MethodGet, srv.URL+"/v1/me", nil, bearer(tokenB))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected the revoked session's token to be refused 401, got %d", resp.StatusCode)
	}
	// ...and the surviving session's memberships are untouched.
	after := membershipEntries(t, getMe(t, clientA, srv.URL, tokenA))
	if !reflect.DeepEqual(membershipKeys(before), membershipKeys(after)) {
		t.Fatalf("revoking a session changed memberships:\n before %v\n after  %v", before, after)
	}
}
