package dto

import (
	"net/http"
	"time"

	"github.com/google/uuid"
)

// CreateInvitationRequest is POST /v1/communities/:id/invitations's body
// (invitations: Invitation Creation With Hashed, One-Time-Visible
// Secrets). UnitID is required: accept-invitation's own request carries
// no unit or community id, so the invitation row itself MUST already
// know which unit the resulting unit_member row belongs to. Email is
// also required, despite the underlying invitations.email column being
// nullable: accept-invitation's request has no email field either (per
// invitations spec: "MUST NOT reveal whether the invited email already
// had an account"), so the invitation's own email is the only source of
// truth accept can use to decide "create a new user" vs. "link an
// existing one" (see handlers/invitations.go's doc comment for the full
// reasoning -- this is a documented deviation from the migration's
// nullable column, at the application layer only).
type CreateInvitationRequest struct {
	UnitID string `json:"unit_id" format:"uuid"`
	Email  string `json:"email" format:"email"`
	Role   string `json:"role" enum:"owner,tenant"`
}

type CreateInvitationInput struct {
	CommunityID string `path:"id" format:"uuid"`
	Body        CreateInvitationRequest
}

// ScopeCommunityID satisfies authz.CommunityScoped: scoped.Community
// resolves and role-checks the caller's membership for this exact
// community BEFORE CreateInvitation ever runs.
func (i *CreateInvitationInput) ScopeCommunityID() uuid.UUID {
	id, _ := uuid.Parse(i.CommunityID)
	return id
}

// InvitationResponse is the invitation resource shape shared by create,
// list responses (invitations). It NEVER carries the plaintext token or
// short code, nor either hash column (invitations spec: "Stored
// invitation contains only hashes" -- and neither hash ever needs to
// reach a client at all). Status is the READ-TIME DERIVED value (design
// D-6: a pending invitation past its expires_at reads as "expired"),
// never the raw stored column value.
type InvitationResponse struct {
	ID          uuid.UUID  `json:"id"`
	CommunityID uuid.UUID  `json:"community_id"`
	UnitID      *uuid.UUID `json:"unit_id,omitempty"`
	Email       string     `json:"email,omitempty"`
	Role        string     `json:"role"`
	Status      string     `json:"status"`
	ExpiresAt   time.Time  `json:"expires_at"`
	AcceptedAt  *time.Time `json:"accepted_at,omitempty"`
	SentCount   int32      `json:"sent_count"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// CreateInvitationResponse embeds InvitationResponse plus the two
// plaintext secrets, present ONLY in the creation response (invitations
// spec: "the response contains the plaintext short code, and no later
// read of that invitation returns the plaintext again").
type CreateInvitationResponse struct {
	InvitationResponse
	ShortCode string `json:"short_code"`
	Token     string `json:"token"`
}

type CreateInvitationOutput struct {
	Body CreateInvitationResponse
}

// ListInvitationsInput is GET /v1/communities/:id/invitations's input.
type ListInvitationsInput struct {
	CommunityID string `path:"id" format:"uuid"`
}

func (i *ListInvitationsInput) ScopeCommunityID() uuid.UUID {
	id, _ := uuid.Parse(i.CommunityID)
	return id
}

type ListInvitationsResponse struct {
	Invitations []InvitationResponse `json:"invitations"`
}

type ListInvitationsOutput struct {
	Body ListInvitationsResponse
}

// ResendInvitationInput is POST /v1/invitations/:id/resend's input
// (invitations: Resend And Revoke). Resolved via
// authz.ResolveCommunityViaInvitation (design D-4), never a caller-
// supplied community id.
type ResendInvitationInput struct {
	InvitationID string `path:"id" format:"uuid"`
}

func (i *ResendInvitationInput) ScopeInvitationID() uuid.UUID {
	id, _ := uuid.Parse(i.InvitationID)
	return id
}

// ResendInvitationResponse carries the NEWLY ISSUED short code as well as
// the send counter. Resend re-issues the code (see ResendInvitation for why
// it cannot redeliver the old one), which means the caller who triggered the
// resend is also the last holder of the plaintext for the paper/voice
// delivery path design D-6 documents -- so it is returned here exactly as
// creation returns it, one time, and no later read of the invitation returns
// it again.
type ResendInvitationResponse struct {
	SentCount int32  `json:"sent_count"`
	ShortCode string `json:"short_code"`
}

type ResendInvitationOutput struct {
	Body ResendInvitationResponse
}

// RevokeInvitationInput is DELETE /v1/invitations/:id's input.
type RevokeInvitationInput struct {
	InvitationID string `path:"id" format:"uuid"`
}

func (i *RevokeInvitationInput) ScopeInvitationID() uuid.UUID {
	id, _ := uuid.Parse(i.InvitationID)
	return id
}

type RevokeInvitationResponse struct {
	Status string `json:"status"`
}

type RevokeInvitationOutput struct {
	Body RevokeInvitationResponse
}

// PreviewInvitationRequest is POST /v1/invitations/preview's body
// (invitations: Preview Endpoint Is POST, Never GET With A Query
// Credential). Exactly one of Token/ShortCode MUST be present -- NEVER
// a query parameter, since a credential in a query string leaks into
// access logs, proxies and Referer.
type PreviewInvitationRequest struct {
	Token     string `json:"token,omitempty"`
	ShortCode string `json:"short_code,omitempty"`
}

// PreviewInvitationInput carries the mandatory device-fingerprint
// headers the enumeration lockout keys on (design D-6: "key
// invite:{ip}:{deviceHash}, where deviceHash is a SHA-256 of the
// mandatory X-Platform + X-App-Version headers").
type PreviewInvitationInput struct {
	XPlatform   string `header:"X-Platform"`
	XAppVersion string `header:"X-App-Version"`
	Body        PreviewInvitationRequest
}

// PreviewInvitationResponse never includes the invited email (design
// D-6: "It returns community, unit, role and expires_at, never the
// invited email").
type PreviewInvitationResponse struct {
	CommunityID   uuid.UUID  `json:"community_id"`
	CommunityName string     `json:"community_name"`
	UnitID        *uuid.UUID `json:"unit_id,omitempty"`
	Role          string     `json:"role"`
	ExpiresAt     time.Time  `json:"expires_at"`
}

type PreviewInvitationOutput struct {
	Body PreviewInvitationResponse
}

// AcceptInvitationRequest is POST /v1/auth/accept-invitation's body
// (invitations: Accept Creates Or Links An Account Without Revealing
// Prior Existence). Platform/DeviceName mirror LoginRequest's own
// fields: a successful accept issues a session exactly like a fresh
// login (design's sequence diagram: "201 session").
type AcceptInvitationRequest struct {
	Token     string `json:"token,omitempty"`
	ShortCode string `json:"short_code,omitempty"`
	Name      string `json:"name" minLength:"1" maxLength:"255"`
	Password  string `json:"password" minLength:"12"`
	Phone     string `json:"phone,omitempty" maxLength:"32"`
	Consent   bool   `json:"consent"`

	// TOTPCode is REQUIRED when, and only when, the invited address
	// already belongs to an account with an ACTIVE second factor. Accept
	// mints a full session, so it runs the same challenge
	// POST /v1/auth/login does and returns the same 403 AUTH_MFA_REQUIRED
	// when the code is missing -- otherwise the two session-minting routes
	// disagree and the one that does not ask becomes the way in
	// (review lineage review-f855997b550a986d).
	TOTPCode string `json:"totp_code,omitempty" doc:"Required only when the invited address already has an account with an active TOTP factor; a 403 AUTH_MFA_REQUIRED response means the account needs one."`

	DeviceName string `json:"device_name,omitempty" maxLength:"255"`
	Platform   string `json:"platform" enum:"ios,android,web"`
}

type AcceptInvitationInput struct {
	XPlatform   string `header:"X-Platform"`
	XAppVersion string `header:"X-App-Version"`
	Body        AcceptInvitationRequest
}

// AcceptInvitationOutput mirrors LoginOutput exactly (invitations spec:
// "both responses have the same shape" -- true independent of the
// account-linked/account-created branch precisely because both branches
// converge on the SAME issueSession call).
type AcceptInvitationOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      LoginResponse
}
