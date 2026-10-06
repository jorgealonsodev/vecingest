package dto

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

// IncidentUnitID preserves explicit JSON null as a non-empty invalid value:
// a plain string decoder otherwise makes `unit_id: null` indistinguishable
// from omission, which would let common scope silently accept the forbidden key.
type IncidentUnitID string

const incidentUnitIDNull = "\x00"

func (id *IncidentUnitID) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		*id = IncidentUnitID(incidentUnitIDNull)
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	*id = IncidentUnitID(value)
	return nil
}

// CreateIncidentRequest is POST /v1/communities/{id}/incidents. The path
// community is the tenant scope; creator, status, priority and attachments
// are deliberately not client-controlled fields. Unknown fields are rejected
// by huma's strict request schema.
type CreateIncidentRequest struct {
	Title        string         `json:"title" minLength:"1" maxLength:"200"`
	Description  string         `json:"description" minLength:"1" maxLength:"5000"`
	Category     string         `json:"category" enum:"elevator,plumbing,electricity,cleaning,locksmith,gardening,works,mandatory_works,noise,other"`
	Scope        string         `json:"scope" enum:"common,unit"`
	UnitID       IncidentUnitID `json:"unit_id,omitempty" format:"uuid" minLength:"1" maxLength:"36" doc:"Required for unit scope and forbidden for common scope."`
	LocationText string         `json:"location_text,omitempty" maxLength:"255"`
}

type CreateIncidentInput struct {
	CommunityID string `path:"id" format:"uuid"`
	Body        CreateIncidentRequest
}

type ListIncidentsInput struct {
	CommunityID string `path:"id" format:"uuid"`
	Status      string `query:"status" enum:"open,assigned,in_progress,resolved,closed,rejected" doc:"Filter by incident status."`
	Category    string `query:"category" enum:"elevator,plumbing,electricity,cleaning,locksmith,gardening,works,mandatory_works,noise,other" doc:"Filter by public incident category; noise maps to the stored noise_and_coexistence category."`
	UnitID      string `query:"unit_id" format:"uuid" doc:"Filter to incidents for this non-deleted unit in the community."`
	Limit       int    `query:"limit" minimum:"1" maximum:"100" default:"20" doc:"Page size (default 20, maximum 100)."`
	Cursor      string `query:"cursor" doc:"Opaque keyset cursor from the previous page; restart pagination if it is stale."`
}

func (i *ListIncidentsInput) ScopeCommunityID() uuid.UUID {
	id, _ := uuid.Parse(i.CommunityID)
	return id
}

func (i *ListIncidentsInput) Resolve(ctx huma.Context) []error {
	allowed := map[string]bool{"status": true, "category": true, "unit_id": true, "limit": true, "cursor": true}
	var issues []error
	requestURL := ctx.URL()
	for name, values := range requestURL.Query() {
		if !allowed[name] || len(values) != 1 || values[0] == "" {
			issues = append(issues, &huma.ErrorDetail{
				Location: "query." + name, Message: "unknown or invalid query parameter",
			})
		}
	}
	return issues
}

type GetIncidentInput struct {
	IncidentID string `path:"id" format:"uuid"`
}

func (i *GetIncidentInput) ScopeIncidentID() uuid.UUID {
	id, _ := uuid.Parse(i.IncidentID)
	return id
}

func (i *GetIncidentInput) Resolve(ctx huma.Context) []error {
	requestURL := ctx.URL()
	if len(requestURL.Query()) == 0 {
		return nil
	}
	return []error{&huma.ErrorDetail{Location: "query", Message: "incident detail does not accept query parameters"}}
}

// ScopeCommunityID satisfies authz.CommunityScoped; the caller's membership
// is resolved for this route community before the create handler runs.
func (i *CreateIncidentInput) ScopeCommunityID() uuid.UUID {
	id, _ := uuid.Parse(i.CommunityID)
	return id
}

// IncidentResponse is an explicit, safe projection of the incident's public
// fields. Internal company data, budgets, notes, rejection details and soft-
// deletion metadata are never serialized from the database row.
type IncidentResponse struct {
	ID            uuid.UUID  `json:"id"`
	CommunityID   uuid.UUID  `json:"community_id"`
	UnitID        *uuid.UUID `json:"unit_id,omitempty"`
	CreatedBy     uuid.UUID  `json:"created_by"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	Category      string     `json:"category" enum:"elevator,plumbing,electricity,cleaning,locksmith,gardening,works,mandatory_works,noise,other"`
	Priority      string     `json:"priority" enum:"low,normal,high,urgent"`
	Status        string     `json:"status" enum:"open,assigned,in_progress,resolved,closed,rejected"`
	Scope         string     `json:"scope" enum:"common,unit"`
	LocationText  string     `json:"location_text,omitempty"`
	AffectedCount int32      `json:"affected_count"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type CreateIncidentOutput struct {
	Body IncidentResponse
}

type ListIncidentsResponse struct {
	Items      []IncidentResponse `json:"items"`
	NextCursor string             `json:"next_cursor"`
}

type ListIncidentsOutput struct {
	Body ListIncidentsResponse
}

type GetIncidentOutput struct {
	Body IncidentResponse
}
