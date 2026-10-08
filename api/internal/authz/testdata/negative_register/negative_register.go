// Package negativeregister is a fixture proving rung 1's one-directional
// compile guarantee (design D-1, authz-membership: "Scoped handler
// signature does not compile against huma.Register"): this package MUST
// NOT compile. authz_negative_register_test.go (package authz) shells
// out to `go build` on this directory and asserts a non-zero exit.
//
// This directory lives under testdata/ so `go build ./...`, `go vet
// ./...` and the normal test suite skip it entirely (Go tooling ignores
// testdata by convention) — only the explicit `go build` invocation in
// the sibling test exercises it.
package negativeregister

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jorgealonsodev/vecingest/internal/authz"
)

type fixtureInput struct{}

type fixtureOutput struct{}

// scopedHandler has the three-argument scoped shape
// (context.Context, *I, authz.Membership) (*O, error) that MUST NOT be
// assignable to huma.Register's two-argument
// (context.Context, *I) (*O, error) handler parameter.
func scopedHandler(_ context.Context, _ *fixtureInput, _ authz.Membership) (*fixtureOutput, error) {
	return &fixtureOutput{}, nil
}

// register attempts the forbidden registration. This line is the one
// that must fail to compile.
func register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "negativeRegisterFixture",
		Method:      "GET",
		Path:        "/v1/communities/{communityId}/fixture",
	}, scopedHandler)
}
