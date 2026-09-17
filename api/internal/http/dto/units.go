package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateUnitRequest is POST /v1/communities/:id/units's body
// (unit-management: Unit Creation Scoped To Community). Members is
// optional: co-owners are recorded as independent unit_members(owner)
// rows created in the SAME transaction as the unit (unit-management:
// Unit Member Roles And Deferred board_role, "co-owners recorded
// independently") -- there is no separate M1 endpoint to add a member
// to an EXISTING unit, matching this milestone's scope. board_role is
// deliberately absent from this and every M1 request DTO: its
// assignment is deferred to the milestone that builds the "Junta
// directiva" tab (proposal Decision 2).
type CreateUnitRequest struct {
	Block string `json:"block,omitempty" maxLength:"64"`
	Floor string `json:"floor,omitempty" maxLength:"32"`
	Door  string `json:"door,omitempty" maxLength:"32"`
	Type  string `json:"type" enum:"flat,premises,garage,storage"`
	// ParticipationCoefficient is a decimal string ("12.3456"), never a
	// JSON number (rules.apply.guidelines: "Never float64").
	ParticipationCoefficient string                    `json:"participation_coefficient,omitempty"`
	CadastralRef             string                    `json:"cadastral_ref,omitempty" maxLength:"64"`
	Members                  []UnitMemberCreateRequest `json:"members,omitempty"`
}

// UnitMemberCreateRequest identifies an EXISTING account by email --
// exactly office-management's "Office Staff Addition Restricted To
// Existing Accounts" pattern, reused here because M1 has no invitation
// flow yet (Phase 6/WU-4): a unit member with no existing account is
// onboarded through an invitation once that milestone ships, never
// bootstrapped with an unusable password the way the FIRST office admin
// is (that bootstrap is a documented one-off exception, not a general
// account-creation mechanism).
type UnitMemberCreateRequest struct {
	Email  string `json:"email" format:"email"`
	Role   string `json:"role" enum:"owner,tenant"`
	Tenure string `json:"tenure,omitempty" enum:"full_owner,bare_owner,usufructuary"`

	NotificationAddress string `json:"notification_address,omitempty"`
	// ElectronicNotificationsConsent, when true, stamps
	// electronic_notifications_consent_at with the current time
	// (unit-management: Consent And Notification Fields Captured Per
	// Member). Omitted or false leaves the column null.
	ElectronicNotificationsConsent bool   `json:"electronic_notifications_consent,omitempty"`
	ConsentTextVersion             string `json:"consent_text_version,omitempty"`
}

type CreateUnitInput struct {
	CommunityID string `path:"id" format:"uuid"`
	Body        CreateUnitRequest
}

// ScopeCommunityID satisfies authz.CommunityScoped: scoped.Community
// resolves and role-checks the caller's membership for this exact
// community BEFORE CreateUnit ever runs, so the handler sets the new
// unit's community_id from the route/resolved membership, never a
// request body field (unit-management: "MUST set the new unit's
// community_id from the route, never from the request body").
func (i *CreateUnitInput) ScopeCommunityID() uuid.UUID {
	id, _ := uuid.Parse(i.CommunityID)
	return id
}

// UnitResponse is the unit resource shape (unit-management).
type UnitResponse struct {
	ID                       uuid.UUID `json:"id"`
	CommunityID              uuid.UUID `json:"community_id"`
	Block                    string    `json:"block,omitempty"`
	Floor                    string    `json:"floor,omitempty"`
	Door                     string    `json:"door,omitempty"`
	Type                     string    `json:"type"`
	ParticipationCoefficient string    `json:"participation_coefficient,omitempty"`
	CadastralRef             string    `json:"cadastral_ref,omitempty"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// UnitMemberResponse is one unit_members row (unit-management). It
// carries no board_role field: M1 never sets it, and never exposes it
// for read either, so a client cannot infer board assignment through
// this milestone's API at all.
type UnitMemberResponse struct {
	ID          uuid.UUID `json:"id"`
	UnitID      uuid.UUID `json:"unit_id"`
	CommunityID uuid.UUID `json:"community_id"`
	UserID      uuid.UUID `json:"user_id"`
	Role        string    `json:"role"`
	Tenure      string    `json:"tenure"`

	NotificationAddress              string     `json:"notification_address,omitempty"`
	ElectronicNotificationsConsentAt *time.Time `json:"electronic_notifications_consent_at,omitempty"`
	ConsentTextVersion               string     `json:"consent_text_version,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UnitCreateResponse is POST /v1/communities/:id/units's body. Warnings
// carries the unit-management: Participation Coefficient Sum Is A
// Warning, Not A Block response-level warning -- never a write-time
// constraint, always empty when the community's units sum to 100 ± 0.01.
type UnitCreateResponse struct {
	UnitResponse
	Members  []UnitMemberResponse `json:"members,omitempty"`
	Warnings []string             `json:"warnings,omitempty"`
}

type CreateUnitOutput struct {
	Body UnitCreateResponse
}

// ListUnitMembersInput is GET /v1/units/:unitId/members's input
// (unit-management: Unit Member Management Scoped To Community).
type ListUnitMembersInput struct {
	UnitID string `path:"unitId" format:"uuid"`
}

// ScopeUnitID satisfies authz.UnitScoped. An unparsable id resolves to
// the zero UUID, which authz.ResolveCommunityViaUnit's
// GetUnitCommunityID lookup treats as any other foreign/unknown unit --
// ErrNoMembership -- so a malformed id surfaces as 404, matching
// GetCommunityInput's identical documented behaviour.
func (i *ListUnitMembersInput) ScopeUnitID() uuid.UUID {
	id, _ := uuid.Parse(i.UnitID)
	return id
}

type ListUnitMembersResponse struct {
	Members []UnitMemberResponse `json:"members"`
}

type ListUnitMembersOutput struct {
	Body ListUnitMembersResponse
}

// UpdateUnitMemberRequest is PATCH /v1/units/:unitId/members/:memberId's
// body. Every field is optional (partial update); board_role is absent
// (unit-management: "no M1 operation accepts a board_role value for
// write").
type UpdateUnitMemberRequest struct {
	Role   *string `json:"role,omitempty" enum:"owner,tenant"`
	Tenure *string `json:"tenure,omitempty" enum:"full_owner,bare_owner,usufructuary"`

	NotificationAddress *string `json:"notification_address,omitempty"`
	// ElectronicNotificationsConsent, when explicitly set, either stamps
	// (true) or clears (false) electronic_notifications_consent_at.
	// nil leaves the current value unchanged (unit-management: Consent
	// And Notification Fields Captured Per Member, "update" half).
	ElectronicNotificationsConsent *bool   `json:"electronic_notifications_consent,omitempty"`
	ConsentTextVersion             *string `json:"consent_text_version,omitempty"`
}

type UpdateUnitMemberInput struct {
	UnitID   string `path:"unitId" format:"uuid"`
	MemberID string `path:"memberId" format:"uuid"`
	Body     UpdateUnitMemberRequest
}

// ScopeUnitID satisfies authz.UnitScoped, identically to
// ListUnitMembersInput's.
func (i *UpdateUnitMemberInput) ScopeUnitID() uuid.UUID {
	id, _ := uuid.Parse(i.UnitID)
	return id
}

type UpdateUnitMemberOutput struct {
	Body UnitMemberResponse
}

// DeleteUnitMemberInput is DELETE /v1/units/:unitId/members/:memberId's
// input.
type DeleteUnitMemberInput struct {
	UnitID   string `path:"unitId" format:"uuid"`
	MemberID string `path:"memberId" format:"uuid"`
}

// ScopeUnitID satisfies authz.UnitScoped, identically to the other unit-
// member inputs.
func (i *DeleteUnitMemberInput) ScopeUnitID() uuid.UUID {
	id, _ := uuid.Parse(i.UnitID)
	return id
}

// DeleteUnitMemberOutput carries no Body: huma defaults an
// output with no Body field to 204 No Content.
type DeleteUnitMemberOutput struct{}
