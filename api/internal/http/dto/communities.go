package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateCommunityRequest is POST /v1/communities's body
// (community-management: Community Creation Restricted To Admin, Scoped
// To Office). OfficeID is the value scoped.Office resolves and validates
// against the caller's own office_members row (design D-1/D-4) -- the
// handler stores the RESOLVED membership's office id, never this raw
// field, so a caller cannot create a community under an office they do
// not belong to even though the field is client-supplied.
type CreateCommunityRequest struct {
	OfficeID          uuid.UUID `json:"office_id"`
	ParentCommunityID string    `json:"parent_community_id,omitempty" format:"uuid"`
	Name              string    `json:"name" minLength:"1" maxLength:"255"`
	CIF               string    `json:"cif,omitempty" maxLength:"32"`
	Address           string    `json:"address,omitempty" maxLength:"255"`
	City              string    `json:"city,omitempty" maxLength:"128"`
	Province          string    `json:"province,omitempty" maxLength:"128"`
	PostalCode        string    `json:"postal_code,omitempty" maxLength:"16"`
	// AnnualBudget/ReserveFund are decimal strings ("123.45"), never a
	// JSON number: rules.apply.guidelines ban binary floating-point types
	// for money. The type name is spelled out in that guideline rather than
	// here, because api/.semgrep's no-float-money-go guard is a lexical
	// scan and would flag this comment as a violation.
	AnnualBudget      string `json:"annual_budget,omitempty"`
	ReserveFund       string `json:"reserve_fund,omitempty"`
	SecretaryIsOffice bool   `json:"secretary_is_office,omitempty"`
}

type CreateCommunityInput struct {
	Body CreateCommunityRequest
}

// ScopeOfficeID satisfies authz.OfficeScoped: scoped.Office resolves and
// role-checks the caller's membership for this exact office id before
// CreateCommunity ever runs (design D-1).
func (i *CreateCommunityInput) ScopeOfficeID() uuid.UUID { return i.Body.OfficeID }

// CommunityResponse is the community resource shape shared by create,
// list and update responses (community-management). It carries no
// unit/office-member counts -- those are detail-only (see
// CommunityDetailResponse) so a list response never pays their query
// cost.
type CommunityResponse struct {
	ID                    uuid.UUID  `json:"id"`
	OfficeID              uuid.UUID  `json:"office_id"`
	ParentCommunityID     *uuid.UUID `json:"parent_community_id,omitempty"`
	Name                  string     `json:"name"`
	CIF                   string     `json:"cif,omitempty"`
	Address               string     `json:"address,omitempty"`
	City                  string     `json:"city,omitempty"`
	Province              string     `json:"province,omitempty"`
	PostalCode            string     `json:"postal_code,omitempty"`
	AnnualBudget          string     `json:"annual_budget,omitempty"`
	ReserveFund           string     `json:"reserve_fund,omitempty"`
	SecretaryIsOffice     bool       `json:"secretary_is_office"`
	LastOrdinaryMeetingAt *time.Time `json:"last_ordinary_meeting_at,omitempty"`
	DpaSignedAt           *time.Time `json:"dpa_signed_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type CreateCommunityOutput struct {
	Body CommunityResponse
}

// ListCommunitiesInput carries no parameters: GET /v1/communities is
// resolved entirely from the caller's own membership set (design D-1
// scoped.Self / D-4 "no path resource").
type ListCommunitiesInput struct{}

type ListCommunitiesResponse struct {
	Communities []CommunityResponse `json:"communities"`
}

type ListCommunitiesOutput struct {
	Body ListCommunitiesResponse
}

// GetCommunityInput is GET /v1/communities/{id}'s input (community-
// management: Community Read And List Scoped By Membership).
type GetCommunityInput struct {
	ID string `path:"id" format:"uuid"`
}

// ScopeCommunityID satisfies authz.CommunityScoped. An unparsable ID
// resolves to the zero UUID, which authz.ResolveCommunity treats as any
// other foreign resource -- ErrNoMembership -- so a malformed id
// surfaces as 404, never a different error shape (design D-4: "Foreign
// resource → 404").
func (i *GetCommunityInput) ScopeCommunityID() uuid.UUID {
	id, _ := uuid.Parse(i.ID)
	return id
}

// CommunityDetailResponse is GET /v1/communities/{id}'s body
// (community-management: Community Detail Excludes Cross-Milestone
// Aggregates). It embeds every CommunityResponse field plus the two
// M1-only aggregates the requirement explicitly allows (unit count,
// office-member count) and MUST NEVER gain a reserve-fund-compliance,
// meeting-quorum, or account-balance field -- those depend on M5/M7
// data that does not exist at M1.
type CommunityDetailResponse struct {
	CommunityResponse
	UnitCount         int64 `json:"unit_count"`
	OfficeMemberCount int64 `json:"office_member_count"`
}

type GetCommunityOutput struct {
	Body CommunityDetailResponse
}

// UpdateCommunityRequest is PATCH /v1/communities/{id}'s body
// (community-management: Community Update Restricted To Office Roles;
// Legal And Descriptive Fields Persisted Per §7.3). Every field is
// optional and a nil pointer leaves the current value unchanged --
// this is a partial update, not a full replace.
type UpdateCommunityRequest struct {
	Name              *string `json:"name,omitempty" minLength:"1" maxLength:"255"`
	CIF               *string `json:"cif,omitempty" maxLength:"32"`
	Address           *string `json:"address,omitempty" maxLength:"255"`
	City              *string `json:"city,omitempty" maxLength:"128"`
	Province          *string `json:"province,omitempty" maxLength:"128"`
	PostalCode        *string `json:"postal_code,omitempty" maxLength:"16"`
	ParentCommunityID *string `json:"parent_community_id,omitempty" format:"uuid"`
	AnnualBudget      *string `json:"annual_budget,omitempty"`
	ReserveFund       *string `json:"reserve_fund,omitempty"`
	SecretaryIsOffice *bool   `json:"secretary_is_office,omitempty"`
}

type UpdateCommunityInput struct {
	ID   string `path:"id" format:"uuid"`
	Body UpdateCommunityRequest
}

// ScopeCommunityID satisfies authz.CommunityScoped, identically to
// GetCommunityInput's.
func (i *UpdateCommunityInput) ScopeCommunityID() uuid.UUID {
	id, _ := uuid.Parse(i.ID)
	return id
}

type UpdateCommunityOutput struct {
	Body CommunityResponse
}
