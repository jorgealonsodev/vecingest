package scoped_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jorgealonsodev/vecingest/internal/authz"
	"github.com/jorgealonsodev/vecingest/internal/authz/scoped"
	"github.com/jorgealonsodev/vecingest/internal/db"
)

// fakeQuerier is a small, hand-rolled double for authz.Querier -- no
// database needed to prove the resolver mechanism itself (design D-4).
type fakeQuerier struct {
	// officeRoles maps "officeID|userID" -> role for
	// GetOfficeMemberByOfficeAndUser.
	officeRoles map[string]string
	// communityViaOfficeRoles maps "communityID|userID" -> role for
	// ResolveCommunityRoleViaOffice (the admin/office leg).
	communityViaOfficeRoles map[string]string
	// communityViaUnitRoles maps "communityID|userID" -> role for
	// ResolveCommunityRoleViaUnit (the owner/tenant leg).
	communityViaUnitRoles map[string]string
	// unitCommunities maps unitID -> communityID for GetUnitCommunityID
	// (design D-4: the {unitId} route shape resolves via units.community_id).
	unitCommunities map[uuid.UUID]uuid.UUID
	// invitationCommunities maps invitationID -> communityID for
	// GetInvitationCommunityID (design D-4: the {invitationId} route
	// shape resolves via invitations.community_id).
	invitationCommunities map[uuid.UUID]uuid.UUID
}

func (f *fakeQuerier) GetUnitCommunityID(_ context.Context, unitID uuid.UUID) (uuid.UUID, error) {
	communityID, ok := f.unitCommunities[unitID]
	if !ok {
		return uuid.UUID{}, pgx.ErrNoRows
	}
	return communityID, nil
}

func (f *fakeQuerier) GetInvitationCommunityID(_ context.Context, invitationID uuid.UUID) (uuid.UUID, error) {
	communityID, ok := f.invitationCommunities[invitationID]
	if !ok {
		return uuid.UUID{}, pgx.ErrNoRows
	}
	return communityID, nil
}

func key(a, b uuid.UUID) string { return a.String() + "|" + b.String() }

func (f *fakeQuerier) GetOfficeMemberByOfficeAndUser(_ context.Context, arg db.GetOfficeMemberByOfficeAndUserParams) (db.OfficeMember, error) {
	role, ok := f.officeRoles[key(arg.OfficeID, arg.UserID)]
	if !ok {
		return db.OfficeMember{}, pgx.ErrNoRows
	}
	return db.OfficeMember{OfficeID: arg.OfficeID, UserID: arg.UserID, Role: role}, nil
}

func (f *fakeQuerier) ResolveCommunityRoleViaOffice(_ context.Context, arg db.ResolveCommunityRoleViaOfficeParams) (string, error) {
	role, ok := f.communityViaOfficeRoles[key(arg.CommunityID, arg.UserID)]
	if !ok {
		return "", pgx.ErrNoRows
	}
	return role, nil
}

func (f *fakeQuerier) ResolveCommunityRoleViaUnit(_ context.Context, arg db.ResolveCommunityRoleViaUnitParams) (string, error) {
	role, ok := f.communityViaUnitRoles[key(arg.CommunityID, arg.UserID)]
	if !ok {
		return "", pgx.ErrNoRows
	}
	return role, nil
}

func (f *fakeQuerier) ListOfficeMembershipsByUserID(_ context.Context, userID uuid.UUID) ([]db.ListOfficeMembershipsByUserIDRow, error) {
	var rows []db.ListOfficeMembershipsByUserIDRow
	for k, role := range f.officeRoles {
		if uid := uuid.MustParse(splitKey(k)[1]); uid == userID {
			rows = append(rows, db.ListOfficeMembershipsByUserIDRow{OfficeID: uuid.MustParse(splitKey(k)[0]), Role: role})
		}
	}
	return rows, nil
}

func (f *fakeQuerier) ListUnitMembershipsByUserID(_ context.Context, userID uuid.UUID) ([]db.ListUnitMembershipsByUserIDRow, error) {
	var rows []db.ListUnitMembershipsByUserIDRow
	for k, role := range f.communityViaUnitRoles {
		if uid := uuid.MustParse(splitKey(k)[1]); uid == userID {
			rows = append(rows, db.ListUnitMembershipsByUserIDRow{CommunityID: uuid.MustParse(splitKey(k)[0]), Role: role})
		}
	}
	return rows, nil
}

func splitKey(k string) [2]string {
	for i := 0; i < len(k); i++ {
		if k[i] == '|' {
			return [2]string{k[:i], k[i+1:]}
		}
	}
	return [2]string{k, ""}
}

// communityInput is a minimal authz.CommunityScoped input: the tenant id
// comes ONLY from the parsed path parameter, never from a header
// (authz-membership: "Header-supplied tenant id is ignored"). XTenant
// simulates a spoofed X-Community-Id-style header on the same request.
type communityInput struct {
	CommunityID string `path:"communityId"`
	XTenant     string `header:"X-Community-Id"`
}

func (i *communityInput) ScopeCommunityID() uuid.UUID { return uuid.MustParse(i.CommunityID) }

type communityOutput struct {
	Body struct {
		Role string `json:"role"`
	}
}

// newCommunityTestAPI registers one scoped.Community operation whose
// handler reports back the resolved membership's role and community id
// (or "PANIC" if the membership was invalid), so the test can assert on
// the plain HTTP response instead of reaching into huma internals.
func newCommunityTestAPI(t *testing.T, roles []authz.Role) (huma.API, *chi.Mux) {
	t.Helper()
	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("test", "0.0.1"))

	scoped.Community[communityInput, communityOutput](api, huma.Operation{
		OperationID: "getCommunityFixture",
		Method:      http.MethodGet,
		Path:        "/v1/communities/{communityId}/fixture",
	}, roles, func(_ context.Context, _ *communityInput, m authz.Membership) (*communityOutput, error) {
		out := &communityOutput{}
		out.Body.Role = safeRole(m)
		return out, nil
	})

	return api, r
}

// safeRole recovers from Membership's panicking accessors so the test
// handler can report "PANIC" instead of crashing the test process when
// membership is invalid.
func safeRole(m authz.Membership) (role string) {
	defer func() {
		if recover() != nil {
			role = "PANIC"
		}
	}()
	return string(m.Role())
}

func doGet(r *chi.Mux, ctx context.Context, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// authz-membership: A scoped constructor resolves membership before
// invoking the handler (task 1.12). The handler must receive a REAL,
// resolved Membership -- not the invalid zero value -- when the caller
// has a real membership row.
func TestCommunity_ResolvesMembershipBeforeHandler(t *testing.T) {
	communityID := uuid.New()
	userID := uuid.New()

	authz.Configure(&fakeQuerier{
		communityViaUnitRoles: map[string]string{key(communityID, userID): string(authz.RoleOwner)},
	})

	api, r := newCommunityTestAPI(t, []authz.Role{authz.RoleOwner, authz.RoleTenant})
	_ = api

	ctx := authz.ContextWithUserID(context.Background(), userID)
	w := doGet(r, ctx, "/v1/communities/"+communityID.String()+"/fixture", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !containsRole(w.Body.String(), "owner") {
		t.Fatalf("expected the handler to receive a resolved owner membership, got body: %s", w.Body.String())
	}
}

func containsRole(body, role string) bool {
	return len(body) > 0 && (indexOf(body, `"role":"`+role+`"`) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// authz-membership: Foreign resource id resolves to no membership
// (task 1.14). A caller with no membership row for the path community
// must be rejected, never reach the handler.
func TestCommunity_ForeignResourceResolvesToNoMembership(t *testing.T) {
	communityID := uuid.New()
	userID := uuid.New()
	authz.Configure(&fakeQuerier{})

	_, r := newCommunityTestAPI(t, []authz.Role{authz.RoleOwner})
	ctx := authz.ContextWithUserID(context.Background(), userID)
	w := doGet(r, ctx, "/v1/communities/"+communityID.String()+"/fixture", nil)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a foreign resource, got %d: %s", w.Code, w.Body.String())
	}
}

// authz-membership: Header-supplied tenant id is ignored (task 1.14). A
// request carrying a valid path resource but a different
// X-Community-Id-style header must resolve from the path only.
func TestCommunity_HeaderSuppliedTenantIDIgnored(t *testing.T) {
	realCommunityID := uuid.New()
	spoofedCommunityID := uuid.New()
	userID := uuid.New()

	authz.Configure(&fakeQuerier{
		communityViaUnitRoles: map[string]string{
			key(realCommunityID, userID):    string(authz.RoleOwner),
			key(spoofedCommunityID, userID): string(authz.RoleAdmin),
		},
	})

	_, r := newCommunityTestAPI(t, []authz.Role{authz.RoleOwner, authz.RoleAdmin})
	ctx := authz.ContextWithUserID(context.Background(), userID)
	w := doGet(r, ctx, "/v1/communities/"+realCommunityID.String()+"/fixture", map[string]string{
		"X-Community-Id": spoofedCommunityID.String(),
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !containsRole(w.Body.String(), "owner") {
		t.Fatalf("expected the path community's role (owner), the header must be ignored; got body: %s", w.Body.String())
	}
}

// authz-membership: Wrong role on own resource rejected (task 1.16). A
// tenant membership rejected against an owner-only operation.
func TestCommunity_WrongRoleRejected403(t *testing.T) {
	communityID := uuid.New()
	userID := uuid.New()
	authz.Configure(&fakeQuerier{
		communityViaUnitRoles: map[string]string{key(communityID, userID): string(authz.RoleTenant)},
	})

	_, r := newCommunityTestAPI(t, []authz.Role{authz.RoleOwner})
	ctx := authz.ContextWithUserID(context.Background(), userID)
	w := doGet(r, ctx, "/v1/communities/"+communityID.String()+"/fixture", nil)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a tenant calling an owner-only operation, got %d: %s", w.Code, w.Body.String())
	}
}

// office-management: Admin scope derived from office_members (task
// 1.16). An admin resolves community access via office_members ∪ their
// office_id, never a client-supplied office id -- exercised here by
// proving the office leg of the resolver (ResolveCommunityRoleViaOffice)
// actually grants access.
func TestCommunity_AdminScopeDerivedFromOfficeMembers(t *testing.T) {
	communityID := uuid.New()
	adminUserID := uuid.New()
	authz.Configure(&fakeQuerier{
		communityViaOfficeRoles: map[string]string{key(communityID, adminUserID): string(authz.RoleAdmin)},
	})

	_, r := newCommunityTestAPI(t, []authz.Role{authz.RoleAdmin})
	ctx := authz.ContextWithUserID(context.Background(), adminUserID)
	w := doGet(r, ctx, "/v1/communities/"+communityID.String()+"/fixture", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for the admin resolved via office_members, got %d: %s", w.Code, w.Body.String())
	}
	if !containsRole(w.Body.String(), "admin") {
		t.Fatalf("expected the resolved role to be admin, got body: %s", w.Body.String())
	}
}
