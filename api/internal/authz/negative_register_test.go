package authz_test

import (
	"os/exec"
	"strings"
	"testing"
)

// authz-membership: Scoped handler signature does not compile against
// huma.Register (task 1.8). testdata/negative_register is a fixture
// package go build/go vet already skip by convention -- this test is
// the only thing that ever compiles it, on purpose, to prove rung 1's
// one-directional guarantee (design D-1): a three-argument scoped
// handler (context.Context, *I, authz.Membership) (*O, error) is not
// assignable to huma.Register's two-argument handler parameter.
func TestNegativeRegister_ScopedHandlerFailsToCompile(t *testing.T) {
	cmd := exec.Command("go", "build", "./testdata/negative_register/...")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected the negative_register fixture to fail to compile, but `go build` succeeded:\n%s", out)
	}
	if !strings.Contains(string(out), "does not match func(context.Context, *I) (*O, error)") {
		t.Fatalf("expected the failure to name huma.Register's signature mismatch, got:\n%s", out)
	}
	if !strings.Contains(string(out), "scopedHandler") {
		t.Fatalf("expected the failure to name the offending handler, got:\n%s", out)
	}
}

// authz-membership: Input missing its scope interface fails to compile
// (task 1.10). badInput does not implement authz.CommunityScoped, so it
// must fail to compile as scoped.Community's type parameter.
func TestNegativeRegister_MissingScopeInterfaceFailsToCompile(t *testing.T) {
	cmd := exec.Command("go", "build", "./testdata/negative_register/...")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected the negative_register fixture to fail to compile, but `go build` succeeded:\n%s", out)
	}
	if !strings.Contains(string(out), "badInput") {
		t.Fatalf("expected the failure to name badInput, got:\n%s", out)
	}
	if !strings.Contains(string(out), "CommunityScoped") {
		t.Fatalf("expected the failure to name the missing CommunityScoped constraint, got:\n%s", out)
	}
}
