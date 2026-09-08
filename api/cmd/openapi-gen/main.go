// Command openapi-gen emits api/openapi/openapi.yaml from the exact same
// huma operation registrations api/internal/http/api.Register uses to
// serve the API (api-contract-generation: Generation Chain). It is
// deliberately not part of the shipped image (design File Changes
// table): it never binds a listener or opens a database connection, it
// only needs the Go structs to exist.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/jorgealonsodev/vecingest/internal/health"
	httpapi "github.com/jorgealonsodev/vecingest/internal/http/api"
	"github.com/jorgealonsodev/vecingest/internal/http/handlers"
)

const outPath = "openapi/openapi.yaml"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	r := chi.NewRouter()
	config := huma.DefaultConfig("Vecingest API", "0.1.0")
	api := humachi.New(r, config)

	// Zero-value Deps: registration only reads struct field TYPES (via
	// reflection on the generic handler signatures) to build the
	// OpenAPI schema; it never invokes a handler, so no real secret,
	// database handle or clock is needed here.
	deps := &handlers.Deps{}
	registry := health.NewRegistry()

	httpapi.Register(api, deps, registry)

	yaml, err := api.OpenAPI().YAML()
	if err != nil {
		return fmt.Errorf("openapi-gen: render YAML: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("openapi-gen: create output dir: %w", err)
	}
	// 0o600: this process is the only writer and the only reader that
	// matters (CI/build tooling running as the same user); gosec G306
	// wants 0600-or-less regardless of the file's actual sensitivity.
	if err := os.WriteFile(outPath, yaml, 0o600); err != nil {
		return fmt.Errorf("openapi-gen: write %s: %w", outPath, err)
	}
	fmt.Printf("openapi-gen: wrote %s (%d bytes)\n", outPath, len(yaml))
	return nil
}
