// Package health implements the readiness-check registry (design D-Q;
// service-health: Readiness Endpoint). At M0 exactly one check is
// registered -- postgres -- because object storage is out of M0 scope
// beyond configuration placeholders (proposal), and probing a
// dependency the process does not have would either fabricate success
// or take the whole compose stack down on boot (worker gates on
// api: service_healthy).
package health

import (
	"context"
	"fmt"
	"time"
)

// checkTimeout bounds a single ReadinessCheck.Check call (D-Q: "each
// with a name and a Check(ctx) error bounded by a 2-second timeout").
const checkTimeout = 2 * time.Second

// ReadinessCheck is one dependency the process actually has.
type ReadinessCheck interface {
	Name() string
	Check(ctx context.Context) error
}

// Registry walks every registered ReadinessCheck. Ready succeeds only
// when all of them succeed within checkTimeout each.
type Registry struct {
	checks []ReadinessCheck
}

// NewRegistry builds a Registry from the given checks, in the order
// callers wish them evaluated. At M0, cmd/vecingest wires exactly one:
// postgres.
func NewRegistry(checks ...ReadinessCheck) *Registry {
	return &Registry{checks: checks}
}

// CheckNames reports every registered check's name, in registration
// order -- useful for the M0 assertion that the registry holds exactly
// [postgres] and nothing else.
func (r *Registry) CheckNames() []string {
	names := make([]string, len(r.checks))
	for i, c := range r.checks {
		names[i] = c.Name()
	}
	return names
}

// Ready runs every registered check, each bounded by its own
// checkTimeout derived from ctx, and returns the first failure. It
// never returns dependency detail beyond the error itself; the HTTP
// handler (Phase 5) is what decides that /v1/health/ready exposes no
// detail externally (D-Q).
func (r *Registry) Ready(ctx context.Context) error {
	for _, c := range r.checks {
		checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		err := c.Check(checkCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("health: check %q failed: %w", c.Name(), err)
		}
	}
	return nil
}
