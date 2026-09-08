package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jorgealonsodev/vecingest/internal/health"
	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
	"github.com/jorgealonsodev/vecingest/internal/http/dto"
)

// Live implements GET /v1/health/live (service-health: Liveness
// Endpoint): unconditional 200, no dependency checks, no detail.
func Live(_ context.Context, _ *dto.LiveInput) (*dto.LiveOutput, error) {
	return &dto.LiveOutput{Body: dto.LiveResponse{OK: true}}, nil
}

// ReadyHandler implements GET /v1/health/ready against registry
// (service-health: Readiness Endpoint; D-Q). It exposes no dependency
// detail externally: a failure is a bare 503, never which check failed
// or why.
type ReadyHandler struct {
	Registry *health.Registry
}

func (h ReadyHandler) Ready(ctx context.Context, _ *dto.ReadyInput) (*dto.ReadyOutput, error) {
	if err := h.Registry.Ready(ctx); err != nil {
		return nil, apperr.New(503, apperr.CodeInternal, "not ready", nil)
	}
	return &dto.ReadyOutput{Body: dto.ReadyResponse{OK: true}}, nil
}

// RegisterHealth wires GET /v1/health/live and GET /v1/health/ready
// into api.
func RegisterHealth(api huma.API, registry *health.Registry) {
	huma.Register(api, huma.Operation{
		OperationID: "healthLive",
		Method:      "GET",
		Path:        "/v1/health/live",
		Summary:     "Liveness probe",
		Tags:        []string{"health"},
	}, Live)

	h := ReadyHandler{Registry: registry}
	huma.Register(api, huma.Operation{
		OperationID: "healthReady",
		Method:      "GET",
		Path:        "/v1/health/ready",
		Summary:     "Readiness probe",
		Tags:        []string{"health"},
	}, h.Ready)
}
