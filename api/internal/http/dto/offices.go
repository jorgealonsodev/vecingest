package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateOfficeRequest is POST /v1/admin/offices's body (office-management:
// Office Creation Restricted To Superadmin; First-Admin Bootstrap
// Without Invitation). AdminName/AdminEmail seed the office's first
// admin user directly -- created with no usable password until they
// complete M0's existing forgot-password flow, and with no invitation
// row of any kind.
type CreateOfficeRequest struct {
	Name             string `json:"name" minLength:"1" maxLength:"255"`
	CIF              string `json:"cif" minLength:"1" maxLength:"32"`
	Email            string `json:"email,omitempty" format:"email"`
	Phone            string `json:"phone,omitempty" maxLength:"32"`
	Address          string `json:"address,omitempty" maxLength:"255"`
	CollegiateNumber string `json:"collegiate_number,omitempty" maxLength:"64"`
	AdminName        string `json:"admin_name" minLength:"1" maxLength:"255"`
	AdminEmail       string `json:"admin_email" format:"email"`
}

type CreateOfficeInput struct {
	Authorization string `header:"Authorization"`
	Body          CreateOfficeRequest
}

// OfficeResponse is the office resource shape shared by every office
// endpoint (office-management).
type OfficeResponse struct {
	ID               uuid.UUID `json:"id"`
	Name             string    `json:"name"`
	CIF              string    `json:"cif"`
	Email            string    `json:"email,omitempty"`
	Phone            string    `json:"phone,omitempty"`
	Address          string    `json:"address,omitempty"`
	CollegiateNumber string    `json:"collegiate_number,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type CreateOfficeOutput struct {
	Body OfficeResponse
}

// GetMyOfficesInput carries no path/body parameters: GET /v1/offices/me
// is resolved entirely from the caller's own membership set
// (design D-1 scoped.Self).
type GetMyOfficesInput struct{}

// ListOfficesResponse is GET /v1/offices/me's body (office-management:
// GET /v1/offices/me Returns Caller's Offices).
type ListOfficesResponse struct {
	Offices []OfficeResponse `json:"offices"`
}

type ListOfficesOutput struct {
	Body ListOfficesResponse
}

// ListOfficeMembersInput mirrors GetMyOfficesInput: no parameters, the
// caller's own office(s) resolve from their membership set.
type ListOfficeMembersInput struct{}

// OfficeMemberResponse is one office_members row (office-management).
type OfficeMemberResponse struct {
	ID        uuid.UUID `json:"id"`
	OfficeID  uuid.UUID `json:"office_id"`
	UserID    uuid.UUID `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type ListOfficeMembersResponse struct {
	Members []OfficeMemberResponse `json:"members"`
}

type ListOfficeMembersOutput struct {
	Body ListOfficeMembersResponse
}

// AddOfficeMemberRequest is POST /v1/offices/me/members's body
// (office-management: Office Staff Addition Restricted To Existing
// Accounts). It MUST resolve to an existing user account: no email is
// sent, and no user is ever created from this endpoint.
type AddOfficeMemberRequest struct {
	Email string `json:"email" format:"email"`
}

type AddOfficeMemberInput struct {
	Body AddOfficeMemberRequest
}

type AddOfficeMemberOutput struct {
	Body OfficeMemberResponse
}
