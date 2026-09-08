package health_test

import (
	"context"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/health"
	"github.com/jorgealonsodev/vecingest/internal/testhelpers"
)

// service-health: Readiness Endpoint — both scenarios, against a real
// Postgres 17 container: 200-equivalent (nil error) while up, and a
// non-2xx-equivalent (non-nil error) once the dependency is stopped.
func TestPostgresCheck_RealContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}

	handles, _ := testhelpers.AppRWHandles(t)
	registry := health.NewRegistry(health.PostgresCheck{DB: handles.Write})

	if err := registry.Ready(context.Background()); err != nil {
		t.Fatalf("expected ready while postgres is up, got: %v", err)
	}

	handles.Close()

	if err := registry.Ready(context.Background()); err == nil {
		t.Fatalf("expected readiness to fail once the database connection is closed")
	}
}
