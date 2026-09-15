package handlers

import (
	"context"
	"encoding/json"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jorgealonsodev/vecingest/internal/authz"
	"github.com/jorgealonsodev/vecingest/internal/authz/scoped"
	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/audit"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/password"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
	"github.com/jorgealonsodev/vecingest/internal/http/dto"
)

// CreateOffice implements POST /v1/admin/offices (office-management:
// Office Creation Restricted To Superadmin; First-Admin Bootstrap
// Without Invitation). It is superadmin-authenticated but NOT
// tenant-scoped -- offices ARE the tenant root (design D-5) -- so it
// registers via plain huma.Register under the same Bearer-auth group
// every other authenticated M0 handler uses, and checks the caller's
// own is_superadmin claim directly, exactly as Me/SuperadminLogin check
// their own claims rather than depending on a route-level decision.
//
// The bootstrap admin's password_hash is a real Argon2id hash of a
// value nobody -- including this handler -- ever learns (the column is
// NOT NULL, so a literal absent password is not representable): login
// therefore fails identically to "no password set" until the admin
// completes forgot-password, and no invitations row of any kind is
// created for them.
func (d *Deps) CreateOffice(ctx context.Context, in *dto.CreateOfficeInput) (*dto.CreateOfficeOutput, error) {
	claims, err := d.authenticate(ctx, in.Authorization)
	if err != nil {
		return nil, unauthorized(err)
	}
	if !claims.SA {
		return nil, forbidden()
	}

	unusable, _, err := token.GenerateOpaqueToken()
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	hash, err := password.Hash(unusable)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	tx, err := d.DB.Write.Begin(ctx)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	office, err := q.InsertOffice(ctx, db.InsertOfficeParams{
		ID:               uuid.New(),
		Name:             in.Body.Name,
		Cif:              in.Body.CIF,
		Email:            optionalText(in.Body.Email),
		Phone:            optionalText(in.Body.Phone),
		Address:          optionalText(in.Body.Address),
		CollegiateNumber: optionalText(in.Body.CollegiateNumber),
	})
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	admin, err := q.InsertUser(ctx, db.InsertUserParams{
		ID:           uuid.New(),
		Email:        in.Body.AdminEmail,
		PasswordHash: hash,
		Name:         in.Body.AdminName,
		Locale:       "es",
	})
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	if _, err := q.InsertOfficeMember(ctx, db.InsertOfficeMemberParams{
		ID:       uuid.New(),
		OfficeID: office.ID,
		UserID:   admin.ID,
		Role:     string(authz.RoleAdmin),
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	callerID, _ := uuid.Parse(claims.Subject)
	after, _ := json.Marshal(map[string]any{"office_id": office.ID, "admin_user_id": admin.ID})
	if _, err := audit.Append(ctx, tx, d.clock(), audit.Entry{
		UserID: &callerID, Action: "office.create", Entity: "office", EntityID: &office.ID, After: after,
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.CreateOfficeOutput{Body: officeResponse(office)}, nil
}

// AddOfficeMember implements POST /v1/offices/me/members
// (office-management: Office Staff Addition Restricted To Existing
// Accounts). Registered via scoped.Self, not scoped.Office: "my own
// office" carries no path resource to resolve an office id from ahead
// of time (D-1's Office constructor requires the input to declare its
// target office id), so the caller's admin membership -- found by
// scanning their own resolved Memberships, exactly as D-4 describes for
// a no-path-resource route -- IS the target office.
func (d *Deps) AddOfficeMember(ctx context.Context, in *dto.AddOfficeMemberInput, memberships authz.Memberships) (*dto.AddOfficeMemberOutput, error) {
	admin, ok := callerOfficeAdmin(memberships)
	if !ok {
		return nil, forbidden()
	}

	q := db.New(d.DB.Write)
	user, err := q.GetUserByEmail(ctx, in.Body.Email)
	if err != nil {
		if isNoRows(err) {
			return nil, apperr.New(404, apperr.CodeNotFound, "no account exists for that email", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	tx, err := d.DB.Write.Begin(ctx)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	txq := db.New(tx)

	member, err := txq.InsertOfficeMember(ctx, db.InsertOfficeMemberParams{
		ID:       uuid.New(),
		OfficeID: admin.OfficeID(),
		UserID:   user.ID,
		Role:     string(authz.RoleAdminStaff),
	})
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	callerID := admin.UserID()
	after, _ := json.Marshal(map[string]any{"office_id": admin.OfficeID(), "user_id": user.ID, "role": member.Role})
	if _, err := audit.Append(ctx, tx, d.clock(), audit.Entry{
		UserID: &callerID, Action: "office_member.add", Entity: "office_member", EntityID: &member.ID, After: after,
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.AddOfficeMemberOutput{Body: officeMemberResponse(member)}, nil
}

// GetMyOffices implements GET /v1/offices/me (office-management: GET
// /v1/offices/me Returns Caller's Offices), listing only the office(s)
// the caller's own resolved Memberships name.
func (d *Deps) GetMyOffices(ctx context.Context, _ *dto.GetMyOfficesInput, memberships authz.Memberships) (*dto.ListOfficesOutput, error) {
	q := db.New(d.DB.Read)
	offices := make([]dto.OfficeResponse, 0, len(memberships))
	for _, m := range memberships {
		if m.Kind() != authz.KindOffice {
			continue
		}
		office, err := q.GetOfficeByID(ctx, m.OfficeID())
		if err != nil {
			if isNoRows(err) {
				continue
			}
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		offices = append(offices, officeResponse(office))
	}
	return &dto.ListOfficesOutput{Body: dto.ListOfficesResponse{Offices: offices}}, nil
}

// ListMyOfficeMembers implements GET /v1/offices/me/members
// (office-management: "GET /v1/offices/me/members MUST list only
// members of the caller's own office(s)"), aggregating every office the
// caller's own resolved Memberships name.
func (d *Deps) ListMyOfficeMembers(ctx context.Context, _ *dto.ListOfficeMembersInput, memberships authz.Memberships) (*dto.ListOfficeMembersOutput, error) {
	q := db.New(d.DB.Read)
	var members []dto.OfficeMemberResponse
	for _, m := range memberships {
		if m.Kind() != authz.KindOffice {
			continue
		}
		rows, err := q.ListOfficeMembers(ctx, m.OfficeID())
		if err != nil {
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		for _, r := range rows {
			members = append(members, officeMemberResponse(r))
		}
	}
	return &dto.ListOfficeMembersOutput{Body: dto.ListOfficeMembersResponse{Members: members}}, nil
}

// callerOfficeAdmin scans memberships for the caller's admin membership
// in an office (office-management: "MUST require admin membership in
// the caller's office"); admin_staff or no office membership at all
// returns ok=false.
func callerOfficeAdmin(memberships authz.Memberships) (authz.Membership, bool) {
	for _, m := range memberships {
		if m.Kind() == authz.KindOffice && m.Role() == authz.RoleAdmin {
			return m, true
		}
	}
	return authz.Membership{}, false
}

func optionalText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func officeResponse(o db.Office) dto.OfficeResponse {
	return dto.OfficeResponse{
		ID:               o.ID,
		Name:             o.Name,
		CIF:              o.Cif,
		Email:            o.Email.String,
		Phone:            o.Phone.String,
		Address:          o.Address.String,
		CollegiateNumber: o.CollegiateNumber.String,
		CreatedAt:        o.CreatedAt,
	}
}

func officeMemberResponse(m db.OfficeMember) dto.OfficeMemberResponse {
	return dto.OfficeMemberResponse{
		ID:        m.ID,
		OfficeID:  m.OfficeID,
		UserID:    m.UserID,
		Role:      m.Role,
		CreatedAt: m.CreatedAt,
	}
}

func forbidden() error {
	return apperr.New(403, apperr.CodeUnauthorized, "forbidden", nil)
}

// RegisterOffices wires every M1 office-management operation into api
// (office-management). CreateOffice is not tenant-scoped (plain
// huma.Register, per D-3: it must never appear in the scoped-route
// allowlist); the /me routes resolve entirely from the caller's own
// Memberships via scoped.Self.
func RegisterOffices(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "createOffice",
		Method:      "POST",
		Path:        "/v1/admin/offices",
		Summary:     "Create an office and its first admin user (superadmin only)",
		Tags:        []string{"offices"},
	}, d.CreateOffice)

	scoped.Self(api, huma.Operation{
		OperationID: "addOfficeMember",
		Method:      "POST",
		Path:        "/v1/offices/me/members",
		Summary:     "Add an existing user as admin_staff to the caller's office",
		Tags:        []string{"offices"},
	}, d.AddOfficeMember)

	scoped.Self(api, huma.Operation{
		OperationID: "getMyOffices",
		Method:      "GET",
		Path:        "/v1/offices/me",
		Summary:     "List the caller's own office(s)",
		Tags:        []string{"offices"},
	}, d.GetMyOffices)

	scoped.Self(api, huma.Operation{
		OperationID: "listMyOfficeMembers",
		Method:      "GET",
		Path:        "/v1/offices/me/members",
		Summary:     "List members of the caller's own office(s)",
		Tags:        []string{"offices"},
	}, d.ListMyOfficeMembers)
}
