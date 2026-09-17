package scoped_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/authz"
	"github.com/jorgealonsodev/vecingest/internal/authz/scoped"
)

// invitationInput is a minimal authz.InvitationScoped input: the caller
// declares which invitation it targets; community membership is
// resolved indirectly via the invitation's own community_id (design
// D-4).
type invitationInput struct {
	InvitationID string `path:"invitationId"`
}

func (i *invitationInput) ScopeInvitationID() uuid.UUID { return uuid.MustParse(i.InvitationID) }

func newInvitationTestAPI(t *testing.T, roles []authz.Role) (huma.API, *chi.Mux) {
	t.Helper()
	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("test", "0.0.1"))

	scoped.Invitation[invitationInput, communityOutput](api, huma.Operation{
		OperationID: "getInvitationFixture",
		Method:      http.MethodGet,
		Path:        "/v1/invitations/{invitationId}/fixture",
	}, roles, func(_ context.Context, _ *invitationInput, m authz.Membership) (*communityOutput, error) {
		out := &communityOutput{}
		out.Body.Role = safeRole(m)
		return out, nil
	})

	return api, r
}

// invitations: Cross-Tenant Isolation Proven By Test (task 6.15/6.16). A
// caller whose membership IS tied to the invitation's own community
// (resolved via invitations.community_id, never a path/body-supplied
// community id) reaches the handler with a resolved community
// Membership.
func TestInvitation_ResolvesCommunityMembershipViaInvitationID(t *testing.T) {
	invitationID := uuid.New()
	communityID := uuid.New()
	userID := uuid.New()

	authz.Configure(&fakeQuerier{
		invitationCommunities:   map[uuid.UUID]uuid.UUID{invitationID: communityID},
		communityViaOfficeRoles: map[string]string{key(communityID, userID): string(authz.RoleAdmin)},
	})

	_, r := newInvitationTestAPI(t, []authz.Role{authz.RoleAdmin, authz.RoleAdminStaff})
	ctx := adminSessionContext(userID)
	w := doGet(r, ctx, "/v1/invitations/"+invitationID.String()+"/fixture", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !containsRole(w.Body.String(), "admin") {
		t.Fatalf("expected the resolved admin membership via the invitation's community, got body: %s", w.Body.String())
	}
}

// An admin of a DIFFERENT community than the one owning the invitation
// must resolve to no membership -- 404, never confirming the
// invitation's existence (invitations spec: "Admin of community B
// cannot revoke community A's invitation").
func TestInvitation_ForeignCommunityViaInvitationResolvesToNoMembership(t *testing.T) {
	invitationID := uuid.New()
	foreignCommunityID := uuid.New()
	userID := uuid.New()

	authz.Configure(&fakeQuerier{
		invitationCommunities: map[uuid.UUID]uuid.UUID{invitationID: foreignCommunityID},
		// userID has no row for foreignCommunityID in either leg.
	})

	_, r := newInvitationTestAPI(t, []authz.Role{authz.RoleAdmin, authz.RoleAdminStaff})
	ctx := authz.ContextWithUserID(context.Background(), userID)
	w := doGet(r, ctx, "/v1/invitations/"+invitationID.String()+"/fixture", nil)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a caller with no membership tied to the invitation's community, got %d: %s", w.Code, w.Body.String())
	}
}

// An unknown invitationId (no row at all) must ALSO resolve to 404,
// exactly like a foreign community (D-4: "Foreign resource → 404").
func TestInvitation_UnknownInvitationResolvesToNotFound(t *testing.T) {
	userID := uuid.New()
	authz.Configure(&fakeQuerier{})

	_, r := newInvitationTestAPI(t, []authz.Role{authz.RoleAdmin})
	ctx := authz.ContextWithUserID(context.Background(), userID)
	w := doGet(r, ctx, "/v1/invitations/"+uuid.New().String()+"/fixture", nil)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown invitation, got %d: %s", w.Code, w.Body.String())
	}
}
