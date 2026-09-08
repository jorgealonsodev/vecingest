package main

import (
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/jorgealonsodev/vecingest/internal/health"
	httpapi "github.com/jorgealonsodev/vecingest/internal/http/api"
	"github.com/jorgealonsodev/vecingest/internal/http/handlers"
)

// api-contract-generation: Generation Chain — "a Go input struct gaining
// a new required field propagates into openapi.yaml" (task 5.35). This
// regresses the MECHANISM: the DTO's json tags/required-ness are read
// directly by huma's reflection-based schema builder, so any change to
// dto.LoginRequest is reflected the next time this test (or `make gen`)
// runs, with no manual schema-editing step to forget.
func TestGeneration_RequiredFieldsPropagateFromGoStructs(t *testing.T) {
	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("Vecingest API (test)", "0.0.0"))
	httpapi.Register(api, &handlers.Deps{}, health.NewRegistry())

	doc := api.OpenAPI()
	op := doc.Paths["/v1/auth/login"].Post
	if op == nil {
		t.Fatalf("expected POST /v1/auth/login to be registered")
	}

	// The request-body schema is a $ref; the actual required-field list
	// (and additionalProperties) lives on the resolved component schema
	// huma registered for the Go struct itself.
	schema, ok := doc.Components.Schemas.Map()["LoginRequest"]
	if !ok || schema == nil {
		t.Fatalf("expected a LoginRequest component schema to be registered")
	}
	_ = op

	required := map[string]bool{}
	for _, name := range schema.Required {
		required[name] = true
	}
	for _, field := range []string{"email", "password", "platform"} {
		if !required[field] {
			t.Fatalf("expected %q to be a required field in the generated schema, required=%v", field, schema.Required)
		}
	}

	// device_name is optional (omitempty): it must NOT be required.
	if required["device_name"] {
		t.Fatalf("expected device_name to remain optional, required=%v", schema.Required)
	}

	// The default AllowAdditionalPropertiesByDefault=false registry
	// setting is what implements "DisallowUnknownFields" for every DTO
	// (task 5.10) without per-struct opt-in.
	if addl, ok := schema.AdditionalProperties.(bool); !ok || addl {
		t.Fatalf("expected additionalProperties:false (unknown fields rejected), got %v", schema.AdditionalProperties)
	}
}
