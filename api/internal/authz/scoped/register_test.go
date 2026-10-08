package scoped_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
	// mfaDisabledUsers marks which user ids IsUserMFAEnabled must report
	// as NOT enabled (auth-mfa-totp delta: Mandatory TOTP For Admin And
	// Admin_staff Scope Access). Every user id absent from this set
	// defaults to enabled=true, so every test written before this gate
	// existed keeps resolving exactly as it did.
	mfaDisabledUsers     map[uuid.UUID]bool
	incidentCommunities  map[uuid.UUID]uuid.UUID
	incidentLookupErr    error
	visibleIncidentErr   error
	visibleIncidentKeys  map[string]bool
	incidentLookupCalls  []uuid.UUID
	visibleIncidentCalls []db.GetVisibleIncidentByIDParams
	incidentCallOrder    []string
}

// IsUserMFAEnabled defaults to true (enabled) for any user id not
// explicitly listed in mfaDisabledUsers -- see that field's doc comment.
func (f *fakeQuerier) IsUserMFAEnabled(_ context.Context, userID uuid.UUID) (bool, error) {
	return !f.mfaDisabledUsers[userID], nil
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

func (f *fakeQuerier) GetIncidentCommunityID(_ context.Context, incidentID uuid.UUID) (uuid.UUID, error) {
	f.incidentCallOrder = append(f.incidentCallOrder, "lookup")
	f.incidentLookupCalls = append(f.incidentLookupCalls, incidentID)
	if f.incidentLookupErr != nil {
		return uuid.UUID{}, f.incidentLookupErr
	}
	communityID, ok := f.incidentCommunities[incidentID]
	if !ok {
		return uuid.UUID{}, pgx.ErrNoRows
	}
	return communityID, nil
}

func (f *fakeQuerier) GetVisibleIncidentByID(_ context.Context, arg db.GetVisibleIncidentByIDParams) (db.Incident, error) {
	f.incidentCallOrder = append(f.incidentCallOrder, "visible")
	f.visibleIncidentCalls = append(f.visibleIncidentCalls, arg)
	if f.visibleIncidentErr != nil {
		return db.Incident{}, f.visibleIncidentErr
	}
	if !f.visibleIncidentKeys[incidentKey(arg.ID, arg.CommunityID, arg.UserID)] {
		return db.Incident{}, pgx.ErrNoRows
	}
	return db.Incident{ID: arg.ID, CommunityID: arg.CommunityID, CreatedBy: arg.UserID}, nil
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

// adminSessionContext is the context an ADMIN request actually arrives
// with in production: the caller's user id plus the second-factor fact
// the Bearer middleware reads off the access token's mfa claim
// (api.bearerAuthAndRateLimit). Since the mandatory-TOTP gate became
// per-session rather than per-account (review lineage
// review-0e1833930adf141a), a test that gives an admin only a user id is
// describing a password-only session, which the gate correctly refuses.
//
// Fixing the tests here rather than loosening the gate is the point: the
// callers below are exercising resolution and role checks, not the MFA
// gate, so they need a session shaped like the one those routes are
// reached with.
func adminSessionContext(userID uuid.UUID) context.Context {
	return authz.ContextWithMFAAuthenticated(authz.ContextWithUserID(context.Background(), userID), true)
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
	ctx := adminSessionContext(adminUserID)
	w := doGet(r, ctx, "/v1/communities/"+communityID.String()+"/fixture", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for the admin resolved via office_members, got %d: %s", w.Code, w.Body.String())
	}
	if !containsRole(w.Body.String(), "admin") {
		t.Fatalf("expected the resolved role to be admin, got body: %s", w.Body.String())
	}
}

// officeInput/officeOutput/newOfficeTestAPI mirror the community fixture
// above, for scoped.Office's own resolver path.
type officeInput struct {
	OfficeID string `path:"officeId"`
}

func (i *officeInput) ScopeOfficeID() uuid.UUID { return uuid.MustParse(i.OfficeID) }

type officeOutput struct {
	Body struct {
		Role string `json:"role"`
	}
}

func newOfficeTestAPI(t *testing.T, roles []authz.Role) (huma.API, *chi.Mux) {
	t.Helper()
	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("test", "0.0.1"))

	scoped.Office[officeInput, officeOutput](api, huma.Operation{
		OperationID: "getOfficeFixture",
		Method:      http.MethodGet,
		Path:        "/v1/offices/{officeId}/fixture",
	}, roles, func(_ context.Context, _ *officeInput, m authz.Membership) (*officeOutput, error) {
		out := &officeOutput{}
		out.Body.Role = safeRole(m)
		return out, nil
	})

	return api, r
}

// auth-mfa-totp delta: Mandatory TOTP For Admin And Admin_staff Scope
// Access, scenario "Admin without TOTP blocked from admin-scoped
// route" -- an admin resolved via office_members (the community-scoped
// office leg) with no active TOTP is rejected with the distinguishable
// MFA code, never the generic 404 a foreign resource or a plain 403
// wrong-role rejection would render.
func TestCommunity_AdminWithoutMFARejected(t *testing.T) {
	communityID := uuid.New()
	adminUserID := uuid.New()
	authz.Configure(&fakeQuerier{
		communityViaOfficeRoles: map[string]string{key(communityID, adminUserID): string(authz.RoleAdmin)},
		mfaDisabledUsers:        map[uuid.UUID]bool{adminUserID: true},
	})

	_, r := newCommunityTestAPI(t, []authz.Role{authz.RoleAdmin})
	ctx := authz.ContextWithUserID(context.Background(), adminUserID)
	w := doGet(r, ctx, "/v1/communities/"+communityID.String()+"/fixture", nil)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for an admin without TOTP, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "AUTH_MFA_ENROLLMENT_REQUIRED") {
		t.Fatalf("expected the distinguishable MFA code, got body: %s", w.Body.String())
	}
}

// admin_staff shares the identical gate.
func TestCommunity_AdminStaffWithoutMFARejected(t *testing.T) {
	communityID := uuid.New()
	staffUserID := uuid.New()
	authz.Configure(&fakeQuerier{
		communityViaOfficeRoles: map[string]string{key(communityID, staffUserID): string(authz.RoleAdminStaff)},
		mfaDisabledUsers:        map[uuid.UUID]bool{staffUserID: true},
	})

	_, r := newCommunityTestAPI(t, []authz.Role{authz.RoleAdmin, authz.RoleAdminStaff})
	ctx := authz.ContextWithUserID(context.Background(), staffUserID)
	w := doGet(r, ctx, "/v1/communities/"+communityID.String()+"/fixture", nil)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for admin_staff without TOTP, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "AUTH_MFA_ENROLLMENT_REQUIRED") {
		t.Fatalf("expected the distinguishable MFA code, got body: %s", w.Body.String())
	}
}

// auth-mfa-totp delta scenario "Admin with active TOTP accesses
// admin-scoped routes normally" -- the sibling positive case for the
// SAME admin identity as TestCommunity_AdminWithoutMFARejected, proving
// the gate is TOTP-state-driven, not a blanket admin rejection.
func TestCommunity_AdminWithMFAAllowed(t *testing.T) {
	communityID := uuid.New()
	adminUserID := uuid.New()
	authz.Configure(&fakeQuerier{
		communityViaOfficeRoles: map[string]string{key(communityID, adminUserID): string(authz.RoleAdmin)},
		// mfaDisabledUsers deliberately empty: this admin has active TOTP.
	})

	_, r := newCommunityTestAPI(t, []authz.Role{authz.RoleAdmin})
	ctx := adminSessionContext(adminUserID)
	w := doGet(r, ctx, "/v1/communities/"+communityID.String()+"/fixture", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for an admin with active TOTP, got %d: %s", w.Code, w.Body.String())
	}
}

// The bypass this gate was rewritten to close (review lineage
// review-0e1833930adf141a, R1-mandatory-totp-gate-is-only-an-enrollment-
// flag), at the resolver layer: the account HAS an active factor
// (mfaDisabledUsers is empty, so IsUserMFAEnabled reports true) and the
// SESSION still never used it. The old gate read only the account flag
// and let this through, which is exactly how a password-only session
// could enroll a factor and walk in.
//
// The code must be AUTH_MFA_REQUIRED, not AUTH_MFA_ENROLLMENT_REQUIRED:
// there is nothing left to enroll, the caller has to log in again with
// a code.
func TestCommunity_AdminEnrolledButSessionNotSecondFactorAuthenticatedRejected(t *testing.T) {
	communityID := uuid.New()
	adminUserID := uuid.New()
	authz.Configure(&fakeQuerier{
		communityViaOfficeRoles: map[string]string{key(communityID, adminUserID): string(authz.RoleAdmin)},
	})

	_, r := newCommunityTestAPI(t, []authz.Role{authz.RoleAdmin})
	// User id only: a session that authenticated on a password alone.
	ctx := authz.ContextWithUserID(context.Background(), adminUserID)
	w := doGet(r, ctx, "/v1/communities/"+communityID.String()+"/fixture", nil)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for an admin whose SESSION never passed a second factor, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "AUTH_MFA_REQUIRED") {
		t.Fatalf("expected AUTH_MFA_REQUIRED (log in again with a code), got body: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "AUTH_MFA_ENROLLMENT_REQUIRED") {
		t.Fatalf("expected NOT the enrollment code: the factor already exists, so an enrollment screen would refuse this caller with a 409; got body: %s", w.Body.String())
	}
}

// auth-mfa-totp delta scenario "Owner without TOTP is unaffected" --
// owner/tenant (resolved via the unit leg) keep TOTP fully optional.
func TestCommunity_OwnerWithoutMFAUnaffected(t *testing.T) {
	communityID := uuid.New()
	ownerUserID := uuid.New()
	authz.Configure(&fakeQuerier{
		communityViaUnitRoles: map[string]string{key(communityID, ownerUserID): string(authz.RoleOwner)},
		mfaDisabledUsers:      map[uuid.UUID]bool{ownerUserID: true},
	})

	_, r := newCommunityTestAPI(t, []authz.Role{authz.RoleOwner, authz.RoleTenant, authz.RoleAdmin, authz.RoleAdminStaff})
	ctx := authz.ContextWithUserID(context.Background(), ownerUserID)
	w := doGet(r, ctx, "/v1/communities/"+communityID.String()+"/fixture", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for an owner with no TOTP (optional for that role), got %d: %s", w.Code, w.Body.String())
	}
}

// The identical gate applies to scoped.Office's own direct resolution
// path (GetOfficeMemberByOfficeAndUser), not only the community-via-
// office leg above.
func TestOffice_AdminWithoutMFARejected(t *testing.T) {
	officeID := uuid.New()
	adminUserID := uuid.New()
	authz.Configure(&fakeQuerier{
		officeRoles:      map[string]string{key(officeID, adminUserID): string(authz.RoleAdmin)},
		mfaDisabledUsers: map[uuid.UUID]bool{adminUserID: true},
	})

	_, r := newOfficeTestAPI(t, []authz.Role{authz.RoleAdmin})
	ctx := authz.ContextWithUserID(context.Background(), adminUserID)
	w := doGet(r, ctx, "/v1/offices/"+officeID.String()+"/fixture", nil)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for an office admin without TOTP, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "AUTH_MFA_ENROLLMENT_REQUIRED") {
		t.Fatalf("expected the distinguishable MFA code, got body: %s", w.Body.String())
	}
}

func TestOffice_AdminWithMFAAllowed(t *testing.T) {
	officeID := uuid.New()
	adminUserID := uuid.New()
	authz.Configure(&fakeQuerier{
		officeRoles: map[string]string{key(officeID, adminUserID): string(authz.RoleAdmin)},
	})

	_, r := newOfficeTestAPI(t, []authz.Role{authz.RoleAdmin})
	ctx := adminSessionContext(adminUserID)
	w := doGet(r, ctx, "/v1/offices/"+officeID.String()+"/fixture", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for an office admin with active TOTP, got %d: %s", w.Code, w.Body.String())
	}
}

type incidentInput struct {
	IncidentID     string `path:"incidentId"`
	HeaderCallerID string `header:"X-User-ID"`
	Body           struct {
		CallerID string `json:"caller_id,omitempty"`
	}
}

func (i *incidentInput) ScopeIncidentID() uuid.UUID { return uuid.MustParse(i.IncidentID) }

type incidentOutput struct {
	Body struct {
		IncidentID     string `json:"incident_id"`
		CommunityID    string `json:"community_id"`
		CallerID       string `json:"caller_id"`
		BodyCallerID   string `json:"body_caller_id"`
		HeaderCallerID string `json:"header_caller_id"`
	}
}

func newIncidentTestAPI(t *testing.T, calls *int) (huma.API, *chi.Mux) {
	t.Helper()
	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("test", "0.0.1"))
	scoped.Incident[incidentInput, incidentOutput](api, huma.Operation{
		OperationID: "incidentAuthorizationFixture",
		Method:      http.MethodPost,
		Path:        "/v1/incidents/{incidentId}/authorization-fixture",
	}, func(_ context.Context, in *incidentInput, access authz.IncidentAccess) (*incidentOutput, error) {
		*calls++
		out := &incidentOutput{}
		out.Body.IncidentID = access.IncidentID().String()
		out.Body.CommunityID = access.CommunityID().String()
		out.Body.CallerID = access.CallerID().String()
		out.Body.BodyCallerID = in.Body.CallerID
		out.Body.HeaderCallerID = in.HeaderCallerID
		return out, nil
	})
	return api, r
}

func incidentKey(incidentID, communityID, callerID uuid.UUID) string {
	return key(incidentID, communityID) + "|" + callerID.String()
}

func postIncidentFixture(r *chi.Mux, ctx context.Context, incidentID uuid.UUID, callerBody, callerHeader string) *httptest.ResponseRecorder {
	body := strings.NewReader(`{"caller_id":"` + callerBody + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/incidents/"+incidentID.String()+"/authorization-fixture", body).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", callerHeader)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestIncident_CreatorAccessRequiresVisibleQueryAndUsesExactResource(t *testing.T) {
	incidentID, communityID, creatorID := uuid.New(), uuid.New(), uuid.New()
	q := &fakeQuerier{
		incidentCommunities: map[uuid.UUID]uuid.UUID{incidentID: communityID},
		visibleIncidentKeys: map[string]bool{incidentKey(incidentID, communityID, creatorID): true},
	}
	authz.Configure(q)
	calls := 0
	_, r := newIncidentTestAPI(t, &calls)
	w := postIncidentFixture(r, authz.ContextWithUserID(context.Background(), creatorID), incidentID, uuid.NewString(), uuid.NewString())

	if w.Code != http.StatusOK || calls != 1 {
		t.Fatalf("creator should reach handler only after visibility proof: status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), incidentID.String()) || !strings.Contains(w.Body.String(), communityID.String()) || !strings.Contains(w.Body.String(), creatorID.String()) {
		t.Fatalf("access did not carry the exact incident, resolved community and authenticated caller: %s", w.Body.String())
	}
	if len(q.incidentCallOrder) != 2 || q.incidentCallOrder[0] != "lookup" || q.incidentCallOrder[1] != "visible" {
		t.Fatalf("visibility must be checked after route-resource lookup and before handler: %v", q.incidentCallOrder)
	}
	if len(q.visibleIncidentCalls) != 1 || q.visibleIncidentCalls[0] != (db.GetVisibleIncidentByIDParams{ID: incidentID, CommunityID: communityID, UserID: creatorID}) {
		t.Fatalf("visible lookup did not bind exact resource/community/authenticated user: %+v", q.visibleIncidentCalls)
	}
}

func TestIncident_InvisibleMissingAndDeletedResourcesDoNotReachHandler(t *testing.T) {
	incidentID, communityID, callerID := uuid.New(), uuid.New(), uuid.New()
	for _, tc := range []struct {
		name             string
		query            *fakeQuerier
		wantVisibleCalls int
	}{
		{name: "missing or deleted", query: &fakeQuerier{}, wantVisibleCalls: 0},
		{name: "invisible", query: &fakeQuerier{incidentCommunities: map[uuid.UUID]uuid.UUID{incidentID: communityID}}, wantVisibleCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			authz.Configure(tc.query)
			calls := 0
			_, r := newIncidentTestAPI(t, &calls)
			w := postIncidentFixture(r, authz.ContextWithUserID(context.Background(), callerID), incidentID, "", "")
			if w.Code != http.StatusNotFound || calls != 0 {
				t.Fatalf("denied incident should be an opaque 404 before handler: status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
			}
			if strings.Contains(w.Body.String(), incidentID.String()) || strings.Contains(w.Body.String(), communityID.String()) {
				t.Fatalf("404 response leaked resource existence details: %s", w.Body.String())
			}
			if len(tc.query.visibleIncidentCalls) != tc.wantVisibleCalls {
				t.Fatalf("visible query calls=%d, want %d", len(tc.query.visibleIncidentCalls), tc.wantVisibleCalls)
			}
		})
	}
}

func TestIncident_AuthenticationCannotBeSuppliedByBodyOrHeader(t *testing.T) {
	incidentID, communityID, authenticatedID, forgedID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	q := &fakeQuerier{
		incidentCommunities: map[uuid.UUID]uuid.UUID{incidentID: communityID},
		visibleIncidentKeys: map[string]bool{incidentKey(incidentID, communityID, authenticatedID): true},
	}
	authz.Configure(q)
	calls := 0
	_, r := newIncidentTestAPI(t, &calls)
	w := postIncidentFixture(r, authz.ContextWithUserID(context.Background(), authenticatedID), incidentID, forgedID.String(), forgedID.String())
	if w.Code != http.StatusOK || calls != 1 ||
		!strings.Contains(w.Body.String(), `"caller_id":"`+authenticatedID.String()+`"`) ||
		!strings.Contains(w.Body.String(), `"body_caller_id":"`+forgedID.String()+`"`) ||
		!strings.Contains(w.Body.String(), `"header_caller_id":"`+forgedID.String()+`"`) {
		t.Fatalf("body/header spoof was not separated from authenticated caller identity: status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
	}

	calls = 0
	for _, ctx := range []context.Context{
		context.Background(),
		authz.ContextWithUserID(context.Background(), uuid.Nil),
	} {
		q.incidentCallOrder = nil
		q.incidentLookupCalls = nil
		q.visibleIncidentCalls = nil
		w = postIncidentFixture(r, ctx, incidentID, authenticatedID.String(), authenticatedID.String())
		if w.Code != http.StatusUnauthorized || calls != 0 || len(q.incidentCallOrder) != 0 {
			t.Fatalf("missing or zero authenticated identity reached resolver/handler: status=%d calls=%d queries=%v", w.Code, calls, q.incidentCallOrder)
		}
	}
}

func TestIncident_DatabaseFailuresRemainInternalErrors(t *testing.T) {
	incidentID, communityID, callerID := uuid.New(), uuid.New(), uuid.New()
	for _, tc := range []struct {
		name  string
		query *fakeQuerier
	}{
		{name: "resource lookup", query: &fakeQuerier{incidentLookupErr: context.DeadlineExceeded}},
		{name: "visibility lookup", query: &fakeQuerier{incidentCommunities: map[uuid.UUID]uuid.UUID{incidentID: communityID}, visibleIncidentErr: context.DeadlineExceeded}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			authz.Configure(tc.query)
			calls := 0
			_, r := newIncidentTestAPI(t, &calls)
			w := postIncidentFixture(r, authz.ContextWithUserID(context.Background(), callerID), incidentID, "", "")
			if w.Code != http.StatusInternalServerError || calls != 0 {
				t.Fatalf("database failure misclassified or reached handler: status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
			}
			if strings.Contains(w.Body.String(), context.DeadlineExceeded.Error()) {
				t.Fatalf("database failure details leaked externally: %s", w.Body.String())
			}
		})
	}
}

func TestIncident_ZeroValueAccessIsRejected(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("zero-value IncidentAccess must not expose authorization identifiers")
		}
	}()
	var access authz.IncidentAccess
	_ = access.IncidentID()
}

func TestIncident_ConstructorPassesBootAssertion(t *testing.T) {
	api, r := newIncidentTestAPI(t, new(int))
	item := api.OpenAPI().Paths["/v1/incidents/{incidentId}/authorization-fixture"]
	if item == nil || item.Post == nil {
		t.Fatal("expected the incident authorization fixture in the OpenAPI model")
	}
	marker, ok := item.Post.Metadata[authz.MetadataKey].(authz.Marker)
	if !ok || marker.Kind != authz.KindIncident || len(marker.Roles) != 0 {
		t.Fatalf("typed incident constructor must stamp the dedicated role-free marker, got %#v", item.Post.Metadata[authz.MetadataKey])
	}
	if err := authz.AssertScopedRegistration(api.OpenAPI(), r, nil); err != nil {
		t.Fatalf("typed incident registration should satisfy boot authorization assertion: %v", err)
	}
}
