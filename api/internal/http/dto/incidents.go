package dto

import (
	"bytes"
	"encoding/json"
	"time"

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
	Category      string     `json:"category"`
	Priority      string     `json:"priority"`
	Status        string     `json:"status"`
	Scope         string     `json:"scope"`
	LocationText  string     `json:"location_text,omitempty"`
	AffectedCount int32      `json:"affected_count"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type CreateIncidentOutput struct {
	Body IncidentResponse
}
