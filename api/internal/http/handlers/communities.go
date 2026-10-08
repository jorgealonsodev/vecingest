package handlers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jorgealonsodev/vecingest/internal/authz"
	"github.com/jorgealonsodev/vecingest/internal/authz/scoped"
	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/audit"
	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
	"github.com/jorgealonsodev/vecingest/internal/http/dto"
)

// communityReadRoles is every role a resolved community Membership can
// hold (PRD §3). GetCommunity passes this set to scoped.Community so
// role checking is a no-op beyond the resolution itself: any caller
// authz.ResolveCommunity already tied to the community (via office or
// unit membership) may read it -- read access is not restricted to a
// subset of roles, only creation and update are (community-management:
// Community Read And List Scoped By Membership).
var communityReadRoles = []authz.Role{authz.RoleAdmin, authz.RoleAdminStaff, authz.RoleOwner, authz.RoleTenant}

// communityWriteRoles is the role set community-management: Community
// Update Restricted To Office Roles names: admin and admin_staff, never
// owner or tenant.
var communityWriteRoles = []authz.Role{authz.RoleAdmin, authz.RoleAdminStaff}

// CreateCommunity implements POST /v1/communities (community-management:
// Community Creation Restricted To Admin, Scoped To Office). Registered
// via scoped.Office with roles=[admin]: scoped.Office resolves and role-
// checks the caller's membership for in.Body.OfficeID BEFORE this
// handler ever runs, so membership.OfficeID() is the validated value --
// the raw request field is never trusted directly for the insert
// (design D-4: "never a client-supplied office id").
func (d *Deps) CreateCommunity(ctx context.Context, in *dto.CreateCommunityInput, membership authz.Membership) (*dto.CreateCommunityOutput, error) {
	parentID, err := parseOptionalUUID(in.Body.ParentCommunityID)
	if err != nil {
		return nil, apperr.New(400, apperr.CodeValidation, "invalid parent_community_id", nil)
	}
	annualBudget, err := parseOptionalNumeric(in.Body.AnnualBudget)
	if err != nil {
		return nil, apperr.New(400, apperr.CodeValidation, "invalid annual_budget", nil)
	}
	reserveFund, err := parseOptionalNumeric(in.Body.ReserveFund)
	if err != nil {
		return nil, apperr.New(400, apperr.CodeValidation, "invalid reserve_fund", nil)
	}

	tx, err := d.DB.Write.Begin(ctx)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	community, err := q.InsertCommunity(ctx, db.InsertCommunityParams{
		ID:                uuid.New(),
		OfficeID:          membership.OfficeID(),
		ParentCommunityID: parentID,
		Name:              in.Body.Name,
		Cif:               optionalText(in.Body.CIF),
		Address:           optionalText(in.Body.Address),
		City:              optionalText(in.Body.City),
		Province:          optionalText(in.Body.Province),
		PostalCode:        optionalText(in.Body.PostalCode),
		AnnualBudget:      annualBudget,
		ReserveFund:       reserveFund,
		SecretaryIsOffice: in.Body.SecretaryIsOffice,
	})
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	callerID := membership.UserID()
	after, _ := json.Marshal(map[string]any{"community_id": community.ID, "office_id": community.OfficeID, "name": community.Name})
	if _, err := audit.Append(ctx, tx, d.clock(), audit.Entry{
		UserID: &callerID, CommunityID: &community.ID, Action: "community.create", Entity: "community", EntityID: &community.ID, After: after,
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.CreateCommunityOutput{Body: communityResponse(community)}, nil
}

// ListMyCommunities implements GET /v1/communities (community-
// management: Community Read And List Scoped By Membership), returning
// every community reachable through the caller's own office membership
// (admin/admin_staff, via ListCommunitiesByOfficeID) or unit membership
// (owner/tenant, via the community id the membership already carries) --
// never a caller-supplied filter.
func (d *Deps) ListMyCommunities(ctx context.Context, _ *dto.ListCommunitiesInput, memberships authz.Memberships) (*dto.ListCommunitiesOutput, error) {
	q := db.New(d.DB.Read)
	seen := make(map[uuid.UUID]bool)
	var out []dto.CommunityResponse

	for _, m := range memberships {
		switch m.Kind() {
		case authz.KindOffice:
			rows, err := q.ListCommunitiesByOfficeID(ctx, m.OfficeID())
			if err != nil {
				return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
			}
			for _, c := range rows {
				if seen[c.ID] {
					continue
				}
				seen[c.ID] = true
				out = append(out, communityResponse(c))
			}
		case authz.KindCommunity:
			if seen[m.CommunityID()] {
				continue
			}
			c, err := q.GetCommunityByID(ctx, m.CommunityID())
			if err != nil {
				if isNoRows(err) {
					continue
				}
				return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
			}
			seen[c.ID] = true
			out = append(out, communityResponse(c))
		case authz.KindSelf:
			// Self never appears inside a resolved Memberships set --
			// ResolveSelf only ever produces KindOffice/KindCommunity
			// entries. Nothing to do.
		}
	}

	return &dto.ListCommunitiesOutput{Body: dto.ListCommunitiesResponse{Communities: out}}, nil
}

// GetCommunity implements GET /v1/communities/{id} (community-
// management: Community Read And List Scoped By Membership; Community
// Detail Excludes Cross-Milestone Aggregates). Registered via
// scoped.Community with communityReadRoles: a caller with no membership
// tied to the path community never reaches this handler (404, via
// scoped.Community's resolveErrorResponse).
func (d *Deps) GetCommunity(ctx context.Context, _ *dto.GetCommunityInput, membership authz.Membership) (*dto.GetCommunityOutput, error) {
	q := db.New(d.DB.Read)
	community, err := q.GetCommunityByID(ctx, membership.CommunityID())
	if err != nil {
		if isNoRows(err) {
			return nil, apperr.New(404, apperr.CodeNotFound, "not found", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	unitCount, err := q.CountUnitsByCommunityID(ctx, community.ID)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	officeMemberCount, err := q.CountOfficeMembersByOfficeID(ctx, community.OfficeID)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.GetCommunityOutput{Body: dto.CommunityDetailResponse{
		CommunityResponse: communityResponse(community),
		UnitCount:         unitCount,
		OfficeMemberCount: officeMemberCount,
	}}, nil
}

// UpdateCommunity implements PATCH /v1/communities/{id} (community-
// management: Community Update Restricted To Office Roles; Legal And
// Descriptive Fields Persisted Per §7.3). Registered via scoped.Community
// with communityWriteRoles=[admin, admin_staff]: an owner/tenant caller
// never reaches this handler (403, via scoped.Community's role check).
// Every request field is optional (partial update) -- a nil field
// leaves the current stored value unchanged.
//
// last_ordinary_meeting_at, dpa_signed_at and the transferred_* pair are
// schema columns the design's full column set names (D-5) but this PATCH
// deliberately does not expose as settable request fields: no Phase 3
// scenario requires them, and transferred_from_office_id/transferred_at
// in particular is a distinct sensitive operation (moving a tenant to a
// different office) this admin/admin_staff-scoped-to-their-OWN-office
// check must never be able to trigger implicitly. UpdateCommunity still
// carries them in its SET list (task 3.8's "full column set") so they
// round-trip unchanged rather than being silently reset.
func (d *Deps) UpdateCommunity(ctx context.Context, in *dto.UpdateCommunityInput, membership authz.Membership) (*dto.UpdateCommunityOutput, error) {
	tx, err := d.DB.Write.Begin(ctx)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	current, err := q.GetCommunityByID(ctx, membership.CommunityID())
	if err != nil {
		if isNoRows(err) {
			return nil, apperr.New(404, apperr.CodeNotFound, "not found", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	before, _ := json.Marshal(communityResponse(current))

	params := db.UpdateCommunityParams{
		ID:                    current.ID,
		ParentCommunityID:     current.ParentCommunityID,
		Name:                  current.Name,
		Cif:                   current.Cif,
		Address:               current.Address,
		City:                  current.City,
		Province:              current.Province,
		PostalCode:            current.PostalCode,
		AnnualBudget:          current.AnnualBudget,
		ReserveFund:           current.ReserveFund,
		SecretaryIsOffice:     current.SecretaryIsOffice,
		LastOrdinaryMeetingAt: current.LastOrdinaryMeetingAt,
		DpaSignedAt:           current.DpaSignedAt,
	}
	if in.Body.Name != nil {
		params.Name = *in.Body.Name
	}
	if in.Body.CIF != nil {
		params.Cif = optionalText(*in.Body.CIF)
	}
	if in.Body.Address != nil {
		params.Address = optionalText(*in.Body.Address)
	}
	if in.Body.City != nil {
		params.City = optionalText(*in.Body.City)
	}
	if in.Body.Province != nil {
		params.Province = optionalText(*in.Body.Province)
	}
	if in.Body.PostalCode != nil {
		params.PostalCode = optionalText(*in.Body.PostalCode)
	}
	if in.Body.SecretaryIsOffice != nil {
		params.SecretaryIsOffice = *in.Body.SecretaryIsOffice
	}
	if in.Body.ParentCommunityID != nil {
		parentID, perr := parseOptionalUUID(*in.Body.ParentCommunityID)
		if perr != nil {
			return nil, apperr.New(400, apperr.CodeValidation, "invalid parent_community_id", nil)
		}
		params.ParentCommunityID = parentID
	}
	if in.Body.AnnualBudget != nil {
		annualBudget, nerr := parseOptionalNumeric(*in.Body.AnnualBudget)
		if nerr != nil {
			return nil, apperr.New(400, apperr.CodeValidation, "invalid annual_budget", nil)
		}
		params.AnnualBudget = annualBudget
	}
	if in.Body.ReserveFund != nil {
		reserveFund, nerr := parseOptionalNumeric(*in.Body.ReserveFund)
		if nerr != nil {
			return nil, apperr.New(400, apperr.CodeValidation, "invalid reserve_fund", nil)
		}
		params.ReserveFund = reserveFund
	}

	updated, err := q.UpdateCommunity(ctx, params)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	callerID := membership.UserID()
	after, _ := json.Marshal(communityResponse(updated))
	if _, err := audit.Append(ctx, tx, d.clock(), audit.Entry{
		UserID: &callerID, CommunityID: &updated.ID, Action: "community.update", Entity: "community", EntityID: &updated.ID, Before: before, After: after,
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.UpdateCommunityOutput{Body: communityResponse(updated)}, nil
}

// communityResponse maps a db.Community row to its API shape, textually
// excluding every field community-management's "Community Detail
// Excludes Cross-Milestone Aggregates" requirement forbids: there is no
// reserve-fund-compliance, quorum, or balance field anywhere on
// dto.CommunityResponse to accidentally populate.
func communityResponse(c db.Community) dto.CommunityResponse {
	return dto.CommunityResponse{
		ID:                    c.ID,
		OfficeID:              c.OfficeID,
		ParentCommunityID:     uuidPtr(c.ParentCommunityID),
		Name:                  c.Name,
		CIF:                   c.Cif.String,
		Address:               c.Address.String,
		City:                  c.City.String,
		Province:              c.Province.String,
		PostalCode:            c.PostalCode.String,
		AnnualBudget:          numericString(c.AnnualBudget),
		ReserveFund:           numericString(c.ReserveFund),
		SecretaryIsOffice:     c.SecretaryIsOffice,
		LastOrdinaryMeetingAt: timestamptzPtr(c.LastOrdinaryMeetingAt),
		DpaSignedAt:           timestamptzPtr(c.DpaSignedAt),
		CreatedAt:             c.CreatedAt,
		UpdatedAt:             c.UpdatedAt,
	}
}

// parseOptionalUUID parses s as a UUID, returning an invalid (SQL NULL)
// pgtype.UUID for an empty string. A malformed non-empty string is
// reported as an error -- unlike the path-parameter parsing in
// dto.GetCommunityInput/UpdateCommunityInput, a body field's validity is
// this handler's own responsibility to check (design D-4's "resolve
// unparsable path ids to 404" policy does not apply to request bodies).
func parseOptionalUUID(s string) (pgtype.UUID, error) {
	if s == "" {
		return pgtype.UUID{}, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: [16]byte(id), Valid: true}, nil
}

// uuidPtr converts a pgtype.UUID into *uuid.UUID, nil when SQL NULL.
func uuidPtr(id pgtype.UUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}
	u := uuid.UUID(id.Bytes)
	return &u
}

// parseOptionalNumeric parses s (a decimal string, e.g. "123.45" --
// rules.apply.guidelines: "Never float64") into a pgtype.Numeric,
// returning an invalid (SQL NULL) value for an empty string.
func parseOptionalNumeric(s string) (pgtype.Numeric, error) {
	if s == "" {
		return pgtype.Numeric{}, nil
	}
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, err
	}
	return n, nil
}

// numericString renders a pgtype.Numeric back to its decimal-string API
// shape, "" for SQL NULL.
func numericString(n pgtype.Numeric) string {
	if !n.Valid {
		return ""
	}
	v, err := n.Value()
	if err != nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// timestamptzPtr converts a pgtype.Timestamptz into *time.Time, nil when
// SQL NULL.
func timestamptzPtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

// RegisterCommunities wires every M1 community-management operation
// into api. CreateCommunity registers via scoped.Office (D-4: creation
// is scoped to the caller's office, not an existing community);
// ListMyCommunities via scoped.Self (no path resource); GetCommunity and
// UpdateCommunity via scoped.Community (D-4: {communityId} route shape).
func RegisterCommunities(api huma.API, d *Deps) {
	scoped.Office(api, huma.Operation{
		OperationID: "createCommunity",
		Method:      "POST",
		Path:        "/v1/communities",
		Summary:     "Create a community in the caller's own office",
		Tags:        []string{"communities"},
	}, []authz.Role{authz.RoleAdmin}, d.CreateCommunity)

	scoped.Self(api, huma.Operation{
		OperationID: "listMyCommunities",
		Method:      "GET",
		Path:        "/v1/communities",
		Summary:     "List communities reachable through the caller's own membership",
		Tags:        []string{"communities"},
	}, d.ListMyCommunities)

	scoped.Community(api, huma.Operation{
		OperationID: "getCommunity",
		Method:      "GET",
		Path:        "/v1/communities/{id}",
		Summary:     "Get a community the caller has a membership tied to",
		Tags:        []string{"communities"},
	}, communityReadRoles, d.GetCommunity)

	scoped.Community(api, huma.Operation{
		OperationID: "updateCommunity",
		Method:      "PATCH",
		Path:        "/v1/communities/{id}",
		Summary:     "Update a community (admin/admin_staff only)",
		Tags:        []string{"communities"},
	}, communityWriteRoles, d.UpdateCommunity)
}
