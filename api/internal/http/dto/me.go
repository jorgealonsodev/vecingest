package dto

import (
	"time"

	"github.com/google/uuid"
)

// MeResponse is GET /v1/me's response shape: id, email, is_superadmin
// and the caller's memberships (user-profile: GET /v1/me Response
// Shape). memberships is always an array, empty for a caller with no
// membership rows -- never null, never omitted.
type MeResponse struct {
	ID           uuid.UUID         `json:"id"`
	Email        string            `json:"email"`
	IsSuperadmin bool              `json:"is_superadmin"`
	Memberships  []MembershipEntry `json:"memberships" nullable:"false"`
}

// Membership scopes M1 can return. A company scope joins the same field
// at M8 with no breaking change; M1 never emits it, since
// company_members does not exist yet.
const (
	MembershipScopeOffice    = "office"
	MembershipScopeCommunity = "community"
)

// MembershipEntry is one element of GET /v1/me's memberships array,
// discriminated by Scope. ID and Name are the office's or the
// community's own, according to Scope.
type MembershipEntry struct {
	Scope string    `json:"scope" enum:"office,community" doc:"Which resource the membership is tied to."`
	ID    uuid.UUID `json:"id" doc:"The office id (scope office) or the community id (scope community)."`
	Name  string    `json:"name" doc:"The office or community name."`
	Role  string    `json:"role" enum:"admin,admin_staff,owner,tenant"`
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
