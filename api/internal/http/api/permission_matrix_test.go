package api_test

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"gopkg.in/yaml.v3"

	"github.com/jorgealonsodev/vecingest/internal/authz"
	"github.com/jorgealonsodev/vecingest/internal/config"
	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/health"
	httpapi "github.com/jorgealonsodev/vecingest/internal/http/api"
	"github.com/jorgealonsodev/vecingest/internal/http/handlers"
	"github.com/jorgealonsodev/vecingest/internal/http/router"
)

// The permission matrix (authz-membership: Permission-Matrix Test At
// 100% Route Coverage; PRD §10.1 M1 gate item 1; design Testing
// Strategy, "Integration" row).
//
// The ROW SET is generated at test time from api/openapi/openapi.yaml,
// never hand-listed: every documented operation must be either
// exercised (matrixCases) or exempted with a written reason
// (matrixExemptions, plus every authz.PublicOperations entry, whose
// reason already lives in that reviewed file). An operation that is
// neither fails TestPermissionMatrix_RouteCoverage, so a newly
// registered route cannot ship without a matrix row. Its sufficiency
// rests on the boot assertion's A3 check (the chi-served route set never
// exceeds the documented set), exactly as the spec states.
//
// The ALLOWED ROLES per row are read from the live registration's own
// authz marker (op.Metadata[authz.MetadataKey]), never restated here,
// so the matrix checks the roles the server actually enforces.

// matrixOpenAPIPath is the published document, relative to this
// package's directory (go test runs with the package dir as cwd).
var matrixOpenAPIPath = filepath.Join("..", "..", "..", "openapi", "openapi.yaml")

// documentedOp is one operation of the published OpenAPI document.
type documentedOp struct {
	Method string
	Path   string
	ID     string
}

func (o documentedOp) key() string { return o.Method + " " + o.Path }

// loadDocumentedOperations parses every operation of the published
// OpenAPI document.
func loadDocumentedOperations(t *testing.T) []documentedOp {
	t.Helper()
	raw, err := os.ReadFile(matrixOpenAPIPath)
	if err != nil {
		t.Fatalf("read %s: %v", matrixOpenAPIPath, err)
	}
	var doc struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v", matrixOpenAPIPath, err)
	}
	methods := map[string]bool{"get": true, "put": true, "post": true, "delete": true, "options": true, "head": true, "patch": true, "trace": true}
	var ops []documentedOp
	for path, item := range doc.Paths {
		for method, node := range item {
			if !methods[method] {
				continue
			}
			var op struct {
				OperationID string `yaml:"operationId"`
			}
			if err := node.Decode(&op); err != nil {
				t.Fatalf("decode %s %s: %v", method, path, err)
			}
			ops = append(ops, documentedOp{Method: strings.ToUpper(method), Path: path, ID: op.OperationID})
		}
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].key() < ops[j].key() })
	return ops
}

// matrixCoverageGaps returns one message per documented operation the
// matrix neither exercises nor exempts, and one per matrix or exemption
// entry naming an operation the document no longer has (a stale row is
// a row that silently stopped testing anything).
func matrixCoverageGaps(ops []documentedOp, exercised map[string]bool, exempt map[string]string) []string {
	var gaps []string
	documented := make(map[string]bool, len(ops))
	for _, op := range ops {
		documented[op.ID] = true
		_, isExempt := exempt[op.ID]
		switch {
		case exercised[op.ID] && isExempt:
			gaps = append(gaps, fmt.Sprintf("%s (%s) is both exercised and exempt; pick one", op.ID, op.key()))
		case !exercised[op.ID] && !isExempt:
			gaps = append(gaps, fmt.Sprintf("%s (%s) is documented but has no permission-matrix row and no exemption", op.ID, op.key()))
		}
	}
	for id := range exercised {
		if !documented[id] {
			gaps = append(gaps, fmt.Sprintf("matrix row %s names no documented operation (stale row)", id))
		}
	}
	for id := range exempt {
		if !documented[id] {
			gaps = append(gaps, fmt.Sprintf("exemption %s names no documented operation (stale exemption)", id))
		}
	}
	sort.Strings(gaps)
	return gaps
}

// TestPermissionMatrix_CoverageCheckFailsOnAnUncoveredRoute is task
// 8.9: authz-membership's "A route missing from the generated matrix
// fails the check". A scoped route that appears in the document but has
// no matrix row, and a matrix row whose route is gone, are both gaps.
func TestPermissionMatrix_CoverageCheckFailsOnAnUncoveredRoute(t *testing.T) {
	ops := []documentedOp{
		{Method: "GET", Path: "/v1/communities/{id}", ID: "getCommunity"},
		{Method: "POST", Path: "/v1/communities/{id}/widgets", ID: "createWidget"},
		{Method: "GET", Path: "/v1/health/live", ID: "healthLive"},
	}
	exercised := map[string]bool{"getCommunity": true, "deleteGadget": true}
	exempt := map[string]string{"healthLive": "liveness probe"}

	gaps := matrixCoverageGaps(ops, exercised, exempt)
	joined := strings.Join(gaps, "\n")
	if len(gaps) != 2 || !strings.Contains(joined, "createWidget") || !strings.Contains(joined, "deleteGadget") {
		t.Fatalf("expected exactly two gaps (uncovered createWidget, stale deleteGadget), got %v", gaps)
	}

	if gaps := matrixCoverageGaps(ops[:1], map[string]bool{"getCommunity": true}, nil); len(gaps) != 0 {
		t.Fatalf("expected a fully covered document to have no gaps, got %v", gaps)
	}
}

// matrixWorld is one tenant side's set of addressable resources, built
// fresh for every matrix case so a destructive row (delete, revoke)
// never changes what the next row sees.
type matrixWorld struct {
	OfficeID     uuid.UUID
	CommunityID  uuid.UUID
	UnitID       uuid.UUID
	MemberID     uuid.UUID
	MemberUserID uuid.UUID
	MemberEmail  string
	InvitationID uuid.UUID
	IncidentID   uuid.UUID
}

// matrixRequest is one HTTP request a matrix case issues.
type matrixRequest struct {
	Method string
	Path   string
	JSON   any    // JSON body, or nil
	CSV    string // multipart/form-data "file" body when non-empty
}

// matrixCases are the exercised rows, keyed by operationId. Each builds
// a VALID request against one world, so an allowed caller on its own
// resource gets a 2xx and a denial can only come from authorization,
// never from input validation.
var matrixCases = map[string]func(w matrixWorld, nonce string) matrixRequest{
	"createCommunity": func(w matrixWorld, n string) matrixRequest {
		return matrixRequest{Method: "POST", Path: "/v1/communities", JSON: map[string]any{"office_id": w.OfficeID, "name": "Matrix " + n}}
	},
	"getCommunity": func(w matrixWorld, _ string) matrixRequest {
		return matrixRequest{Method: "GET", Path: "/v1/communities/" + w.CommunityID.String()}
	},
	"updateCommunity": func(w matrixWorld, n string) matrixRequest {
		return matrixRequest{Method: "PATCH", Path: "/v1/communities/" + w.CommunityID.String(), JSON: map[string]any{"city": "City " + n}}
	},
	"listInvitations": func(w matrixWorld, _ string) matrixRequest {
		return matrixRequest{Method: "GET", Path: "/v1/communities/" + w.CommunityID.String() + "/invitations"}
	},
	"createInvitation": func(w matrixWorld, n string) matrixRequest {
		return matrixRequest{Method: "POST", Path: "/v1/communities/" + w.CommunityID.String() + "/invitations", JSON: map[string]any{
			"unit_id": w.UnitID, "email": "matrix-invite-" + n + "@example.com", "role": "owner",
		}}
	},
	"createIncident": func(w matrixWorld, n string) matrixRequest {
		return matrixRequest{Method: "POST", Path: "/v1/communities/" + w.CommunityID.String() + "/incidents", JSON: map[string]any{
			"title": "Matrix incident " + n, "description": "An incident created by the permission matrix.",
			"category": "other", "scope": "common",
		}}
	},
	"listIncidents": func(w matrixWorld, _ string) matrixRequest {
		return matrixRequest{Method: "GET", Path: "/v1/communities/" + w.CommunityID.String() + "/incidents"}
	},
	"getIncident": func(w matrixWorld, _ string) matrixRequest {
		return matrixRequest{Method: "GET", Path: "/v1/incidents/" + w.IncidentID.String()}
	},
	"createUnit": func(w matrixWorld, _ string) matrixRequest {
		return matrixRequest{Method: "POST", Path: "/v1/communities/" + w.CommunityID.String() + "/units", JSON: map[string]any{"type": "flat"}}
	},
	"importUnits": func(w matrixWorld, _ string) matrixRequest {
		return matrixRequest{
			Method: "POST", Path: "/v1/communities/" + w.CommunityID.String() + "/units/import?dry_run=true",
			CSV: "portal,floor,door,type,coefficient,owner_name,owner_dni_cif\nA,1,D0,flat,10,Jane Doe,12345678A\n",
		}
	},
	"getUnitImportTemplate": func(w matrixWorld, _ string) matrixRequest {
		return matrixRequest{Method: "GET", Path: "/v1/communities/" + w.CommunityID.String() + "/units/import/template"}
	},
	"revokeInvitation": func(w matrixWorld, _ string) matrixRequest {
		return matrixRequest{Method: "DELETE", Path: "/v1/invitations/" + w.InvitationID.String()}
	},
	"resendInvitation": func(w matrixWorld, _ string) matrixRequest {
		return matrixRequest{Method: "POST", Path: "/v1/invitations/" + w.InvitationID.String() + "/resend"}
	},
	"listUnitMembers": func(w matrixWorld, _ string) matrixRequest {
		return matrixRequest{Method: "GET", Path: "/v1/units/" + w.UnitID.String() + "/members"}
	},
	"updateUnitMember": func(w matrixWorld, _ string) matrixRequest {
		return matrixRequest{Method: "PATCH", Path: "/v1/units/" + w.UnitID.String() + "/members/" + w.MemberID.String(), JSON: map[string]any{"tenure": "bare_owner"}}
	},
	"deleteUnitMember": func(w matrixWorld, _ string) matrixRequest {
		return matrixRequest{Method: "DELETE", Path: "/v1/units/" + w.UnitID.String() + "/members/" + w.MemberID.String()}
	},
	// scoped.Self rows: no path resource, so there is nothing foreign to
	// address. Their denial is ABSENCE: the response must never carry an
	// identifier of the foreign tenant.
	"listMyCommunities": func(matrixWorld, string) matrixRequest {
		return matrixRequest{Method: "GET", Path: "/v1/communities"}
	},
	"getMyOffices": func(matrixWorld, string) matrixRequest {
		return matrixRequest{Method: "GET", Path: "/v1/offices/me"}
	},
	"listMyOfficeMembers": func(matrixWorld, string) matrixRequest {
		return matrixRequest{Method: "GET", Path: "/v1/offices/me/members"}
	},
	"addOfficeMember": func(w matrixWorld, _ string) matrixRequest {
		return matrixRequest{Method: "POST", Path: "/v1/offices/me/members", JSON: map[string]any{"email": w.MemberEmail}}
	},
}

// matrixExemptions are documented operations with no tenant resource to
// own or to be foreign to, each with the reason and where its own
// access rule is tested instead. authz.PublicOperations entries are
// exempt on top of these, with the reasons that reviewed file carries.
var matrixExemptions = map[string]string{
	"createOffice": "platform-level and superadmin-only: there is no tenant resource yet; " +
		"the non-superadmin 403 is TestOffice_CreationRestrictedToSuperadmin",
	"acceptInvitation": "public, addressed by a one-time code rather than a tenant resource; " +
		"invitation cross-tenant isolation is covered in invitation_test.go",
	"previewInvitation": "public, addressed by a one-time code rather than a tenant resource; " +
		"invitation cross-tenant isolation is covered in invitation_test.go",
	"logout":         "acts only on the caller's own session; no tenant resource",
	"getMe":          "identity-scoped: the caller's own user row and memberships (me_test.go)",
	"listMySessions": "identity-scoped: the caller's own session family (TestAuthFlow_SessionsListAndRevoke)",
	"revokeMySession": "identity-scoped: a foreign session family answers 404 " +
		"(TestAuthFlow_SessionsListAndRevoke)",
	"enrollMFA": "acts only on the caller's own second factor; no tenant resource",
	"verifyMFA": "acts only on the caller's own second factor; no tenant resource",
}

// matrixExemptionsWithPublic merges matrixExemptions with every
// authz.PublicOperations entry, mapped to its operationId through ops.
func matrixExemptionsWithPublic(ops []documentedOp) map[string]string {
	out := make(map[string]string, len(matrixExemptions)+len(authz.PublicOperations))
	for id, reason := range matrixExemptions {
		out[id] = reason
	}
	for _, op := range ops {
		for _, e := range authz.PublicOperations {
			if e.Method == op.Method && e.Path == op.Path {
				out[op.ID] = "authz.PublicOperations: " + e.Reason
			}
		}
	}
	return out
}

func exercisedOperationIDs() map[string]bool {
	out := make(map[string]bool, len(matrixCases))
	for id := range matrixCases {
		out[id] = true
	}
	return out
}

// liveOperationMarkers registers the real API (no database needed:
// registration never touches one) and returns every operation's authz
// marker, keyed by operationId. An operation with no marker is absent.
func liveOperationMarkers(t *testing.T) map[string]authz.Marker {
	t.Helper()
	_, hapi, err := httpapi.New(httpapi.Config{
		Router:   router.Config{AppEnv: config.AppEnvDevelopment, CorsOrigins: []string{testOrigin}},
		Deps:     &handlers.Deps{},
		Registry: health.NewRegistry(),
		Title:    "Vecingest API (matrix)",
		Version:  "test",
	})
	if err != nil {
		t.Fatalf("build api: %v", err)
	}
	markers := make(map[string]authz.Marker)
	for _, item := range hapi.OpenAPI().Paths {
		for _, op := range []*huma.Operation{item.Get, item.Put, item.Post, item.Delete, item.Options, item.Head, item.Patch, item.Trace} {
			if op == nil {
				continue
			}
			if m, ok := op.Metadata[authz.MetadataKey].(authz.Marker); ok {
				markers[op.OperationID] = m
			}
		}
	}
	return markers
}

// TestPermissionMatrix_RouteCoverage is the 100 % route-coverage half of
// the matrix (task 8.13; PRD §10.1 M1 gate item 1). It needs no
// database: it checks the published document against the matrix rows,
// and the rows against the live registration's own authz markers -- a
// tenant-scoped operation can be neither exempted nor left out, and an
// exercised row must really be tenant-scoped.
func TestPermissionMatrix_RouteCoverage(t *testing.T) {
	ops := loadDocumentedOperations(t)
	exempt := matrixExemptionsWithPublic(ops)
	exercised := exercisedOperationIDs()

	gaps := matrixCoverageGaps(ops, exercised, exempt)

	markers := liveOperationMarkers(t)
	for id := range markers {
		if !exercised[id] {
			gaps = append(gaps, fmt.Sprintf("%s carries an authz marker (tenant-scoped) but has no permission-matrix row", id))
		}
	}
	for id := range exercised {
		if _, ok := markers[id]; !ok {
			gaps = append(gaps, fmt.Sprintf("matrix row %s is not tenant-scoped (no authz marker); exempt it with a reason instead", id))
		}
	}
	for id, reason := range exempt {
		if strings.TrimSpace(reason) == "" {
			gaps = append(gaps, fmt.Sprintf("exemption %s has no reason", id))
		}
	}
	if len(gaps) != 0 {
		t.Fatalf("permission matrix does not cover every documented route:\n  %s", strings.Join(gaps, "\n  "))
	}

	covered := 0
	for _, op := range ops {
		if exercised[op.ID] || exempt[op.ID] != "" {
			covered++
		}
	}
	t.Logf("permission-matrix route coverage: %d/%d documented operations covered (%.0f%%): %d exercised (%d tenant-scoped markers), %d exempt with a reason",
		covered, len(ops), 100*float64(covered)/float64(len(ops)), len(exercised), len(markers), len(ops)-len(exercised))
}

// matrixCaller is one authenticated caller of the matrix.
type matrixCaller struct {
	Name  string
	Token string
	// Role is the caller's role inside tenant A, or "" for a caller with
	// no tenant membership at all (outsider, superadmin).
	Role authz.Role
	// OfficeMember is true when Role comes from an office_members row,
	// i.e. the caller also resolves office-scoped operations in A.
	OfficeMember bool
}

// unusablePasswordHash is not a PHC string, so no password can ever
// verify against it: a matrix member account cannot log in.
const unusablePasswordHash = "not-a-login-account" //nolint:gosec // G101: deliberately unusable placeholder, never a credential

// cheapUser inserts a user row that will never log in, skipping the
// Argon2id hash createUser pays for.
func cheapUser(t *testing.T, q *db.Queries, email string) uuid.UUID {
	t.Helper()
	u, err := q.InsertUser(t.Context(), db.InsertUserParams{
		ID: uuid.New(), Email: email, PasswordHash: unusablePasswordHash, Name: "Matrix User", Locale: "es",
	})
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return u.ID
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("random bytes: %v", err)
	}
	return b
}

// newMatrixWorld builds fresh addressable resources inside an existing
// office/community: a unit, an owner member on it, and a pending
// invitation for that unit.
func newMatrixWorld(t *testing.T, q *db.Queries, officeID, communityID uuid.UUID) matrixWorld {
	t.Helper()
	unit, err := q.InsertUnit(t.Context(), db.InsertUnitParams{ID: uuid.New(), CommunityID: communityID, Type: "flat"})
	if err != nil {
		t.Fatalf("insert unit: %v", err)
	}
	email := "matrix-member-" + uuid.NewString() + "@example.com"
	memberUser := cheapUser(t, q, email)
	member, err := q.InsertUnitMember(t.Context(), db.InsertUnitMemberParams{
		ID: uuid.New(), UnitID: unit.ID, CommunityID: communityID, UserID: memberUser, Role: "owner", Tenure: "full_owner",
	})
	if err != nil {
		t.Fatalf("insert unit member: %v", err)
	}
	inv, err := q.InsertInvitation(t.Context(), db.InsertInvitationParams{
		ID:            uuid.New(),
		CommunityID:   communityID,
		UnitID:        pgtype.UUID{Bytes: unit.ID, Valid: true},
		Email:         pgtype.Text{String: "matrix-pending-" + uuid.NewString() + "@example.com", Valid: true},
		Role:          "owner",
		TokenHash:     randomBytes(t, 32),
		ShortCodeHash: randomBytes(t, 32),
		ExpiresAt:     time.Now().Add(7 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("insert invitation: %v", err)
	}
	return matrixWorld{
		OfficeID: officeID, CommunityID: communityID, UnitID: unit.ID,
		MemberID: member.ID, MemberUserID: memberUser, MemberEmail: email, InvitationID: inv.ID,
	}
}

// matrixRequestSeq numbers matrix requests so each comes from its own
// simulated client address: the router's per-IP budget (60/min) is a
// request-protection concern this matrix does not test, and one shared
// address would turn most rows into 429s.
var matrixRequestSeq int

// doMatrixRequest issues req with token and returns the status and the
// raw body (the Self rows scan it for foreign identifiers).
func doMatrixRequest(t *testing.T, client *http.Client, srvURL, token string, req matrixRequest) (int, string) {
	t.Helper()
	var body io.Reader
	contentType := ""
	switch {
	case req.CSV != "":
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		part, err := mw.CreateFormFile("file", "units.csv")
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := part.Write([]byte(req.CSV)); err != nil {
			t.Fatalf("write form file: %v", err)
		}
		if err := mw.Close(); err != nil {
			t.Fatalf("close multipart writer: %v", err)
		}
		body, contentType = &buf, mw.FormDataContentType()
	case req.JSON != nil:
		b, err := json.Marshal(req.JSON)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		body, contentType = bytes.NewReader(b), "application/json"
	}
	httpReq, err := http.NewRequest(req.Method, srvURL+req.Path, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	matrixRequestSeq++
	httpReq.Header.Set("X-Forwarded-For", fmt.Sprintf("10.0.0.1, 198.18.%d.%d", matrixRequestSeq/250, matrixRequestSeq%250+1))
	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

// TestPermissionMatrix_ForeignResourceDenied is the Testcontainers half
// (tasks 8.10/8.11; authz-membership: "Foreign community access denied"):
// every exercised operation × every caller × own/foreign resource.
//
//   - Foreign resource (tenant B), any caller: 403 or 404.
//   - Caller with no tenant membership (outsider, superadmin), on tenant
//     A too: 403 or 404 -- superadmin is not a tenant bypass.
//   - Own resource, role allowed by the operation's live marker: 2xx --
//     the positive control that proves a denial above is authorization
//     and not a malformed request.
//   - Own resource, member of the scope but role not allowed: 403.
//   - Own resource, no membership in that scope kind (owner/tenant on an
//     office-scoped operation): 403 or 404.
//   - scoped.Self rows: never 5xx, and the body never carries an
//     identifier of tenant B.
func TestPermissionMatrix_ForeignResourceDenied(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)
	q := db.New(handlesDB.Write)
	markers := liveOperationMarkers(t)

	// Tenant A: office A with admin + admin_staff (both with an active
	// factor, so their sessions clear the mandatory-TOTP gate), one
	// community, and an owner and a tenant holding units in it.
	officeA, adminA := seedOfficeWithAdmin(t, handlesDB, "matrix-admin-a@example.com")
	staffA := createUser(t, handlesDB, "matrix-staff-a@example.com", false)
	if _, err := q.InsertOfficeMember(t.Context(), db.InsertOfficeMemberParams{
		ID: uuid.New(), OfficeID: officeA, UserID: staffA, Role: "admin_staff",
	}); err != nil {
		t.Fatalf("insert admin_staff: %v", err)
	}
	seedActiveMFA(t, handlesDB, staffA)
	communityA := seedCommunity(t, handlesDB, officeA, "Matrix Community A")
	ownerA := createUser(t, handlesDB, "matrix-owner-a@example.com", false)
	seedUnitOwner(t, handlesDB, communityA, ownerA)
	tenantA := createUser(t, handlesDB, "matrix-tenant-a@example.com", false)
	tenantUnit := seedUnit(t, handlesDB, communityA)
	if _, err := q.InsertUnitMember(t.Context(), db.InsertUnitMemberParams{
		ID: uuid.New(), UnitID: tenantUnit, CommunityID: communityA, UserID: tenantA, Role: "tenant", Tenure: "full_owner",
	}); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	// Tenant B: another office and community nobody above belongs to.
	officeB, _ := seedOfficeWithAdmin(t, handlesDB, "matrix-admin-b@example.com")
	communityB := seedCommunity(t, handlesDB, officeB, "Matrix Community B")

	outsider := createUser(t, handlesDB, "matrix-outsider@example.com", false)
	superadmin := createUser(t, handlesDB, "matrix-superadmin@example.com", true)

	callers := []matrixCaller{
		{Name: "admin", Token: mintAccessToken(t, deps, handlesDB, adminA, false), Role: authz.RoleAdmin, OfficeMember: true},
		{Name: "admin_staff", Token: mintAccessToken(t, deps, handlesDB, staffA, false), Role: authz.RoleAdminStaff, OfficeMember: true},
		{Name: "owner", Token: mintAccessToken(t, deps, handlesDB, ownerA, false), Role: authz.RoleOwner},
		{Name: "tenant", Token: mintAccessToken(t, deps, handlesDB, tenantA, false), Role: authz.RoleTenant},
		{Name: "outsider", Token: mintAccessToken(t, deps, handlesDB, outsider, false)},
		{Name: "superadmin", Token: mintAccessToken(t, deps, handlesDB, superadmin, true)},
	}

	ids := make([]string, 0, len(matrixCases))
	for id := range matrixCases {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	assertions := 0
	for _, opID := range ids {
		build := matrixCases[opID]
		marker, ok := markers[opID]
		if !ok {
			t.Fatalf("matrix row %s has no live authz marker (TestPermissionMatrix_RouteCoverage explains)", opID)
		}
		for _, c := range callers {
			if marker.Kind == authz.KindSelf {
				// A fresh foreign world exists while the Self row runs, so
				// a leak would have something to leak.
				foreign := newMatrixWorld(t, q, officeB, communityB)
				own := newMatrixWorld(t, q, officeA, communityA)
				req := build(own, uuid.NewString()[:8])
				status, body := doMatrixRequest(t, client, srv.URL, c.Token, req)
				assertions++
				if status >= 500 || status == http.StatusUnauthorized {
					t.Errorf("%s as %s: expected a non-5xx, authenticated answer, got %d body=%s", opID, c.Name, status, body)
				}
				for _, foreignID := range []uuid.UUID{officeB, communityB, foreign.UnitID, foreign.MemberID, foreign.InvitationID} {
					if strings.Contains(body, foreignID.String()) {
						t.Errorf("%s as %s: response leaks foreign tenant id %s: %s", opID, c.Name, foreignID, body)
					}
				}
				continue
			}

			for _, side := range []string{"own", "foreign"} {
				var w matrixWorld
				if side == "own" {
					w = newMatrixWorld(t, q, officeA, communityA)
				} else {
					w = newMatrixWorld(t, q, officeB, communityB)
				}
				if opID == "getIncident" {
					incident, err := q.InsertIncident(t.Context(), db.InsertIncidentParams{
						ID: uuid.New(), CommunityID: w.CommunityID, CreatedBy: w.MemberUserID,
						Title: "Matrix readable incident", Description: "Visible common incident.",
						Category: "other", Scope: "common",
					})
					if err != nil {
						t.Fatalf("insert matrix incident: %v", err)
					}
					w.IncidentID = incident.ID
				}
				req := build(w, uuid.NewString()[:8])
				status, body := doMatrixRequest(t, client, srv.URL, c.Token, req)
				assertions++

				memberOfScope := c.Role != "" && (marker.Kind != authz.KindOffice || c.OfficeMember)
				switch {
				case side == "foreign" || !memberOfScope:
					if status != http.StatusForbidden && status != http.StatusNotFound {
						t.Errorf("%s %s as %s on %s resource: expected 403 or 404, got %d body=%s", req.Method, req.Path, c.Name, side, status, body)
					}
				case marker.Kind == authz.KindIncident:
					if status < 200 || status > 299 {
						t.Errorf("%s %s as %s on own visible incident: expected 2xx, got %d body=%s", req.Method, req.Path, c.Name, status, body)
					}
				case authz.RoleAllowed(c.Role, marker.Roles):
					if status < 200 || status > 299 {
						t.Errorf("%s %s as %s on own resource: role is allowed, expected 2xx, got %d body=%s", req.Method, req.Path, c.Name, status, body)
					}
				default:
					if status != http.StatusForbidden {
						t.Errorf("%s %s as %s on own resource: role not allowed, expected 403, got %d body=%s", req.Method, req.Path, c.Name, status, body)
					}
				}
			}
		}
	}
	t.Logf("permission matrix: %d operations × %d callers × own/foreign = %d assertions", len(ids), len(callers), assertions)
}
