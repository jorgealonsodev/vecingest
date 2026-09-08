package dto

import (
	"time"

	"github.com/google/uuid"
)

// MeResponse is GET /v1/me's response shape at M0: id, email and
// is_superadmin only -- no role, no memberships (user-profile: GET
// /v1/me Response Shape). Those arrive at M1 with the office/community
// schema.
type MeResponse struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	IsSuperadmin bool      `json:"is_superadmin"`
}

type MeInput struct {
	Authorization string `header:"Authorization"`
}

type MeOutput struct {
	Body MeResponse
}

// SessionSummary is one entry of GET /v1/me/sessions -- device and
// activity data, and explicitly NEVER a token hash (user-profile:
// Remote Session Listing and Revocation Endpoints).
type SessionSummary struct {
	FamilyID   uuid.UUID  `json:"family_id" doc:"Also the :id path parameter for DELETE /v1/me/sessions/:id."`
	DeviceName string     `json:"device_name,omitempty"`
	Platform   string     `json:"platform"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type ListSessionsInput struct {
	Authorization string `header:"Authorization"`
}

type ListSessionsResponse struct {
	Sessions []SessionSummary `json:"sessions"`
}

type ListSessionsOutput struct {
	Body ListSessionsResponse
}

type RevokeSessionInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id"`
}

type RevokeSessionOutput struct{}

// LiveResponse is GET /v1/health/live's body: unconditional, no
// dependency detail (service-health: Liveness Endpoint).
type LiveResponse struct {
	OK bool `json:"ok"`
}

type LiveInput struct{}

type LiveOutput struct {
	Body LiveResponse
}

// ReadyResponse mirrors LiveResponse's shape; readiness exposes no
// dependency detail externally either (D-Q).
type ReadyResponse struct {
	OK bool `json:"ok"`
}

type ReadyInput struct{}

type ReadyOutput struct {
	Body ReadyResponse
}
