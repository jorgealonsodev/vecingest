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

// unitInput is a minimal authz.UnitScoped input: the caller declares
// which unit it targets; community membership is resolved indirectly
// via the unit's own community_id (design D-4).
type unitInput struct {
	UnitID string `path:"unitId"`
}

func (i *unitInput) ScopeUnitID() uuid.UUID { return uuid.MustParse(i.UnitID) }

func newUnitTestAPI(t *testing.T, roles []authz.Role) (huma.API, *chi.Mux) {
	t.Helper()
	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("test", "0.0.1"))

	scoped.Unit[unitInput, communityOutput](api, huma.Operation{
		OperationID: "getUnitFixture",
		Method:      http.MethodGet,
		Path:        "/v1/units/{unitId}/fixture",
	}, roles, func(_ context.Context, _ *unitInput, m authz.Membership) (*communityOutput, error) {
		out := &communityOutput{}
		out.Body.Role = safeRole(m)
		return out, nil
	})

	return api, r
}

// unit-management: Unit Member Management Scoped To Community (task
// 4.11). A caller whose membership IS tied to the unit's own community
// (resolved via units.community_id, never a path/body-supplied
// community id) reaches the handler with a resolved community
// Membership.
func TestUnit_ResolvesCommunityMembershipViaUnitID(t *testing.T) {
	unitID := uuid.New()
	communityID := uuid.New()
	userID := uuid.New()

	authz.Configure(&fakeQuerier{
		unitCommunities:       map[uuid.UUID]uuid.UUID{unitID: communityID},
		communityViaUnitRoles: map[string]string{key(communityID, userID): string(authz.RoleOwner)},
	})

	_, r := newUnitTestAPI(t, []authz.Role{authz.RoleOwner, authz.RoleTenant})
	ctx := authz.ContextWithUserID(context.Background(), userID)
	w := doGet(r, ctx, "/v1/units/"+unitID.String()+"/fixture", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !containsRole(w.Body.String(), "owner") {
		t.Fatalf("expected the resolved owner membership via the unit's community, got body: %s", w.Body.String())
	}
}

// unit-management: Member of another community cannot list members
// (task 4.11 scenario). A unit belonging to a DIFFERENT community than
// the caller's own membership must resolve to no membership -- 404,
// never confirming the unit's existence.
func TestUnit_ForeignCommunityViaUnitResolvesToNoMembership(t *testing.T) {
	unitID := uuid.New()
	foreignCommunityID := uuid.New()
	userID := uuid.New()

	authz.Configure(&fakeQuerier{
		unitCommunities: map[uuid.UUID]uuid.UUID{unitID: foreignCommunityID},
		// userID has no row for foreignCommunityID in either leg.
	})

	_, r := newUnitTestAPI(t, []authz.Role{authz.RoleOwner, authz.RoleTenant})
	ctx := authz.ContextWithUserID(context.Background(), userID)
	w := doGet(r, ctx, "/v1/units/"+unitID.String()+"/fixture", nil)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a caller with no membership tied to the unit's community, got %d: %s", w.Code, w.Body.String())
	}
}

// unit-management: an unknown unitId (no row at all) must ALSO resolve
// to 404, exactly like a foreign community -- never a different error
// shape that would distinguish "unit doesn't exist" from "unit exists
// but I have no membership tied to it" (D-4: "Foreign resource → 404").
func TestUnit_UnknownUnitResolvesToNotFound(t *testing.T) {
	userID := uuid.New()
	authz.Configure(&fakeQuerier{})

	_, r := newUnitTestAPI(t, []authz.Role{authz.RoleOwner})
	ctx := authz.ContextWithUserID(context.Background(), userID)
	w := doGet(r, ctx, "/v1/units/"+uuid.New().String()+"/fixture", nil)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown unit, got %d: %s", w.Code, w.Body.String())
	}
}
