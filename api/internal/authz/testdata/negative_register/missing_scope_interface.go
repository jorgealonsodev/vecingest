package negativeregister

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jorgealonsodev/vecingest/internal/authz"
	"github.com/jorgealonsodev/vecingest/internal/authz/scoped"
)

// badInput does NOT implement authz.CommunityScoped (no
// ScopeCommunityID method), so it MUST fail to compile as
// scoped.Community's type parameter (authz-membership: "Input missing
// its scope interface fails to compile").
type badInput struct{}

type badOutput struct{}

func badScopedHandler(_ context.Context, _ *badInput, _ authz.Membership) (*badOutput, error) {
	return &badOutput{}, nil
}

// registerMissingScopeInterface attempts the forbidden registration.
// This line is the one that must fail to compile.
func registerMissingScopeInterface(api huma.API) {
	scoped.Community[badInput, badOutput](api, huma.Operation{
		OperationID: "negativeRegisterMissingScopeInterface",
		Method:      "GET",
		Path:        "/v1/communities/{communityId}/fixture-missing-scope",
	}, []authz.Role{authz.RoleAdmin}, badScopedHandler)
}
