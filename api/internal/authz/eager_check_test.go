package authz

import (
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// design D-2 / V4: OnAddOperation is registered as an additive eager
// trigger, never the sole check. Registering an unmarked scoped
// operation through a huma.Config carrying authz.EagerCheck must panic
// immediately at registration time, giving the earliest possible error
// site -- AssertScopedRegistration remains the authoritative, non-panic
// boot-time gate (assert_test.go), so this is a second, earlier signal,
// not a replacement.
func TestEagerCheck_PanicsOnUnmarkedScopedOperation(t *testing.T) {
	r := chi.NewRouter()
	cfg := huma.DefaultConfig("test", "0.0.1")
	cfg.OnAddOperation = append(cfg.OnAddOperation, EagerCheck)
	api := humachi.New(r, cfg)

	defer func() {
		if recover() == nil {
			t.Fatalf("expected registering an unmarked scoped operation to panic")
		}
	}()
	huma.Register(api, huma.Operation{
		OperationID: "eagerUnmarkedOp",
		Method:      http.MethodGet,
		Path:        "/v1/communities/{communityId}/eager-unmarked",
	}, assertFixtureHandler)
}

// Triangulation: a marked scoped operation, and a non-scoped public
// operation, must NOT panic -- the hook is scoped to exactly A1's rule.
func TestEagerCheck_DoesNotPanicOnMarkedOrNonScopedOperations(t *testing.T) {
	r := chi.NewRouter()
	cfg := huma.DefaultConfig("test", "0.0.1")
	cfg.OnAddOperation = append(cfg.OnAddOperation, EagerCheck)
	api := humachi.New(r, cfg)

	huma.Register(api, huma.Operation{
		OperationID: "eagerMarkedOp",
		Method:      http.MethodGet,
		Path:        "/v1/communities/{communityId}/eager-marked",
		Metadata: map[string]any{
			MetadataKey: Marker{Kind: KindCommunity, Roles: []Role{RoleAdmin}},
		},
	}, assertFixtureHandler)

	huma.Register(api, huma.Operation{
		OperationID: "eagerPublicOp",
		Method:      http.MethodGet,
		Path:        "/v1/auth/eager-public",
	}, assertFixtureHandler)
}
