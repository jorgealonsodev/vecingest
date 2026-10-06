package authz

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

type (
	assertFixtureInput  struct{}
	assertFixtureOutput struct{}
)

func assertFixtureHandler(_ context.Context, _ *assertFixtureInput) (*assertFixtureOutput, error) {
	return &assertFixtureOutput{}, nil
}

func newInMemoryAPI() (huma.API, chi.Router) {
	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("test", "0.0.1"))
	return api, r
}

// authz-membership: Unregistered scoped operation blocks boot (A1)
// (task 1.18). An operation under a scoped prefix registered via plain
// huma.Register carries no marker and must fail A1, naming the
// offending operation id.
func TestAssertScopedRegistration_A1_UnregisteredScopedOperationBlocksBoot(t *testing.T) {
	api, r := newInMemoryAPI()
	huma.Register(api, huma.Operation{
		OperationID: "unmarkedCommunityOp",
		Method:      http.MethodGet,
		Path:        "/v1/communities/{communityId}/unmarked",
	}, assertFixtureHandler)

	err := AssertScopedRegistration(api.OpenAPI(), r, nil)
	if err == nil {
		t.Fatalf("expected AssertScopedRegistration to fail for an unmarked scoped operation")
	}
	if !strings.Contains(err.Error(), "unmarkedCommunityOp") {
		t.Fatalf("expected the error to name the offending operation, got: %v", err)
	}
}

func TestAssertScopedRegistration_A1_UnmarkedIncidentOperationBlocksBoot(t *testing.T) {
	api, r := newInMemoryAPI()
	huma.Register(api, huma.Operation{
		OperationID: "unmarkedIncidentOp",
		Method:      http.MethodGet,
		Path:        "/v1/incidents/{incidentId}",
	}, assertFixtureHandler)

	err := AssertScopedRegistration(api.OpenAPI(), r, nil)
	if err == nil || !strings.Contains(err.Error(), "unmarkedIncidentOp") {
		t.Fatalf("expected the unmarked incident operation to block boot and be named, got: %v", err)
	}
}

// authz-membership: Marker without a registered resolver blocks boot
// (A2) (task 1.18). A marked operation whose declared scope kind has no
// registered resolver must fail A2.
func TestAssertScopedRegistration_A2_MarkerWithoutRegisteredResolverBlocksBoot(t *testing.T) {
	api, r := newInMemoryAPI()
	huma.Register(api, huma.Operation{
		OperationID: "unregisteredKindOp",
		Method:      http.MethodGet,
		Path:        "/v1/communities/{communityId}/unregistered-kind",
		Metadata: map[string]any{
			MetadataKey: Marker{Kind: Kind(99), Roles: []Role{RoleAdmin}},
		},
	}, assertFixtureHandler)

	err := AssertScopedRegistration(api.OpenAPI(), r, nil)
	if err == nil {
		t.Fatalf("expected AssertScopedRegistration to fail for a marker with no registered resolver")
	}
	if !strings.Contains(err.Error(), "unregisteredKindOp") {
		t.Fatalf("expected the error to name the offending operation, got: %v", err)
	}
}

// authz-membership: Hidden scoped operation blocks boot (A3) (task
// 1.19). An operation registered under a scoped prefix with
// Hidden: true is routed by chi but absent from Paths -- A3 must find it
// in the chi-vs-OpenAPI difference set and, absent from the allowlist,
// refuse to start.
func TestAssertScopedRegistration_A3_HiddenScopedOperationBlocksBoot(t *testing.T) {
	api, r := newInMemoryAPI()
	huma.Register(api, huma.Operation{
		OperationID: "hiddenCommunityOp",
		Method:      http.MethodGet,
		Path:        "/v1/communities/{communityId}/hidden",
		Hidden:      true,
	}, assertFixtureHandler)

	err := AssertScopedRegistration(api.OpenAPI(), r, nil)
	if err == nil {
		t.Fatalf("expected AssertScopedRegistration to fail for a Hidden, undocumented scoped route")
	}
	if !strings.Contains(err.Error(), "/v1/communities/{communityId}/hidden") {
		t.Fatalf("expected the error to name the offending route, got: %v", err)
	}
}

// authz-membership: Allowlisted public route boots normally (A3) (task
// 1.19). The same Hidden route, present in the allowlist with a stated
// reason, must boot successfully.
func TestAssertScopedRegistration_A3_AllowlistedRouteBootsNormally(t *testing.T) {
	api, r := newInMemoryAPI()
	huma.Register(api, huma.Operation{
		OperationID: "hiddenAllowlistedOp",
		Method:      http.MethodGet,
		Path:        "/v1/communities/{communityId}/hidden-allowlisted",
		Hidden:      true,
	}, assertFixtureHandler)

	allow := Allowlist{
		{Method: http.MethodGet, Path: "/v1/communities/{communityId}/hidden-allowlisted", Reason: "test fixture: intentionally undocumented"},
	}

	if err := AssertScopedRegistration(api.OpenAPI(), r, allow); err != nil {
		t.Fatalf("expected the allowlisted Hidden route to boot successfully, got: %v", err)
	}
}

// authz-membership: V8 marker-survives-groups (task 1.20). The marker
// must survive huma.NewGroup's prefix rewrite and still be readable
// from Paths at the post-prefix path.
func TestAssertScopedRegistration_MarkerSurvivesGroupPrefixing(t *testing.T) {
	api, _ := newInMemoryAPI()
	group := huma.NewGroup(api, "/v1/communities/{communityId}")
	huma.Register(group, huma.Operation{
		OperationID: "groupedOp",
		Method:      http.MethodGet,
		Path:        "/grouped",
		Metadata: map[string]any{
			MetadataKey: Marker{Kind: KindCommunity, Roles: []Role{RoleAdmin}},
		},
	}, assertFixtureHandler)

	pathItem, ok := api.OpenAPI().Paths["/v1/communities/{communityId}/grouped"]
	if !ok {
		t.Fatalf("expected the prefixed path to be documented in Paths")
	}
	if pathItem.Get == nil {
		t.Fatalf("expected a GET operation at the prefixed path")
	}
	marker, ok := pathItem.Get.Metadata[MetadataKey].(Marker)
	if !ok {
		t.Fatalf("expected the marker to survive group prefixing, Metadata was: %#v", pathItem.Get.Metadata)
	}
	if marker.Kind != KindCommunity {
		t.Fatalf("expected the surviving marker's Kind to be KindCommunity, got %v", marker.Kind)
	}
}
