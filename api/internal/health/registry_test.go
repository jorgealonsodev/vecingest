package health_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jorgealonsodev/vecingest/internal/health"
)

type fakeCheck struct {
	name string
	err  error
}

func (f fakeCheck) Name() string { return f.name }
func (f fakeCheck) Check(ctx context.Context) error {
	return f.err
}

// service-health: Readiness Endpoint — "Registered M0 dependency
// reachable" scenario.
func TestRegistry_ReadyWhenEveryCheckPasses(t *testing.T) {
	r := health.NewRegistry(fakeCheck{name: "postgres"})
	if err := r.Ready(context.Background()); err != nil {
		t.Fatalf("expected ready, got error: %v", err)
	}
}

// service-health: Readiness Endpoint — "Database unreachable" scenario.
func TestRegistry_NotReadyWhenACheckFails(t *testing.T) {
	r := health.NewRegistry(fakeCheck{name: "postgres", err: errors.New("connection refused")})
	if err := r.Ready(context.Background()); err == nil {
		t.Fatalf("expected an error when a registered check fails")
	}
}

func TestRegistry_M0HoldsExactlyOnePostgresCheck(t *testing.T) {
	r := health.NewRegistry(fakeCheck{name: "postgres"})
	names := r.CheckNames()
	if len(names) != 1 || names[0] != "postgres" {
		t.Fatalf("expected the M0 registry to hold exactly [postgres], got %v", names)
	}
}

func TestRegistry_CheckTimesOut(t *testing.T) {
	r := health.NewRegistry(slowCheck{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := r.Ready(ctx); err == nil {
		t.Fatalf("expected a slow check to fail readiness once its context deadline passes")
	}
}

type slowCheck struct{}

func (slowCheck) Name() string { return "slow" }
func (slowCheck) Check(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Second):
		return nil
	}
}
