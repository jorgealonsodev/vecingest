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
	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
	"github.com/jorgealonsodev/vecingest/internal/http/dto"
)

// ListUnitMembers implements GET /v1/units/:unitId/members (unit-
// management: Unit Member Management Scoped To Community). Registered
// via scoped.Unit with unitMemberReadRoles (every role): a caller with
// no membership tied to the unit's community never reaches this
// handler (403/404, via scoped.Unit's resolveErrorResponse -- design
// D-4's ResolveCommunityViaUnit).
func (d *Deps) ListUnitMembers(ctx context.Context, in *dto.ListUnitMembersInput, membership authz.Membership) (*dto.ListUnitMembersOutput, error) {
	q := db.New(d.DB.Read)
	rows, err := q.ListUnitMembersByUnitID(ctx, db.ListUnitMembersByUnitIDParams{
		UnitID:      in.ScopeUnitID(),
		CommunityID: membership.CommunityID(),
	})
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	members := make([]dto.UnitMemberResponse, 0, len(rows))
	for _, r := range rows {
		members = append(members, unitMemberResponse(r))
	}
	return &dto.ListUnitMembersOutput{Body: dto.ListUnitMembersResponse{Members: members}}, nil
}

// UpdateUnitMember implements PATCH /v1/units/:unitId/members/:memberId
// (unit-management: Unit Member Management Scoped To Community; Consent
// And Notification Fields Captured Per Member, "update" half).
// Registered via scoped.Unit with unitManageRoles=[admin, admin_staff]:
// stricter than the spec's bare minimum (any membership tied to the
// community), matching community-management's read/write role split --
// see the doc comment on unitManageRoles. GetUnitMemberByID binds
// unit_id AND community_id together with the path memberId, so a
// memberId from a different unit or a different community can never be
// reached (defense in depth beyond the route's own scoping).
func (d *Deps) UpdateUnitMember(ctx context.Context, in *dto.UpdateUnitMemberInput, membership authz.Membership) (*dto.UpdateUnitMemberOutput, error) {
	memberID, err := uuid.Parse(in.MemberID)
	if err != nil {
		return nil, apperr.New(404, apperr.CodeNotFound, "not found", nil)
	}

	tx, err := d.DB.Write.Begin(ctx)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	current, err := q.GetUnitMemberByID(ctx, db.GetUnitMemberByIDParams{
		ID: memberID, UnitID: in.ScopeUnitID(), CommunityID: membership.CommunityID(),
	})
	if err != nil {
		if isNoRows(err) {
			return nil, apperr.New(404, apperr.CodeNotFound, "not found", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	before, _ := json.Marshal(unitMemberResponse(current))

	params := db.UpdateUnitMemberParams{
		ID:                               current.ID,
		UnitID:                           current.UnitID,
		CommunityID:                      current.CommunityID,
		Role:                             current.Role,
		Tenure:                           current.Tenure,
		NotificationAddress:              current.NotificationAddress,
		ElectronicNotificationsConsentAt: current.ElectronicNotificationsConsentAt,
		ConsentTextVersion:               current.ConsentTextVersion,
	}
	if in.Body.Role != nil {
		params.Role = *in.Body.Role
	}
	if in.Body.Tenure != nil {
		params.Tenure = *in.Body.Tenure
	}
	if in.Body.NotificationAddress != nil {
		params.NotificationAddress = optionalText(*in.Body.NotificationAddress)
	}
	if in.Body.ConsentTextVersion != nil {
		params.ConsentTextVersion = optionalText(*in.Body.ConsentTextVersion)
	}
	if in.Body.ElectronicNotificationsConsent != nil {
		if *in.Body.ElectronicNotificationsConsent {
			params.ElectronicNotificationsConsentAt = pgtype.Timestamptz{Time: d.clock().Now(), Valid: true}
		} else {
			params.ElectronicNotificationsConsentAt = pgtype.Timestamptz{}
		}
	}

	updated, err := q.UpdateUnitMember(ctx, params)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	callerID := membership.UserID()
	after, _ := json.Marshal(unitMemberResponse(updated))
	if _, err := audit.Append(ctx, tx, d.clock(), audit.Entry{
		UserID: &callerID, CommunityID: &updated.CommunityID, Action: "unit_member.update", Entity: "unit_member", EntityID: &updated.ID, Before: before, After: after,
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.UpdateUnitMemberOutput{Body: unitMemberResponse(updated)}, nil
}

// DeleteUnitMember implements DELETE /v1/units/:unitId/members/:memberId
// (unit-management: Unit Member Management Scoped To Community).
// Registered via scoped.Unit with unitManageRoles. Soft-deletes
// (deleted_at = now()), mirroring internal/db/queries/sessions.sql's
// revoke pattern -- never a hard DELETE.
func (d *Deps) DeleteUnitMember(ctx context.Context, in *dto.DeleteUnitMemberInput, membership authz.Membership) (*dto.DeleteUnitMemberOutput, error) {
	memberID, err := uuid.Parse(in.MemberID)
	if err != nil {
		return nil, apperr.New(404, apperr.CodeNotFound, "not found", nil)
	}

	tx, err := d.DB.Write.Begin(ctx)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	current, err := q.GetUnitMemberByID(ctx, db.GetUnitMemberByIDParams{
		ID: memberID, UnitID: in.ScopeUnitID(), CommunityID: membership.CommunityID(),
	})
	if err != nil {
		if isNoRows(err) {
			return nil, apperr.New(404, apperr.CodeNotFound, "not found", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	before, _ := json.Marshal(unitMemberResponse(current))

	rowsAffected, err := q.DeleteUnitMember(ctx, db.DeleteUnitMemberParams{
		ID: current.ID, UnitID: current.UnitID, CommunityID: current.CommunityID,
	})
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if rowsAffected == 0 {
		return nil, apperr.New(404, apperr.CodeNotFound, "not found", nil)
	}

	callerID := membership.UserID()
	if _, err := audit.Append(ctx, tx, d.clock(), audit.Entry{
		UserID: &callerID, CommunityID: &current.CommunityID, Action: "unit_member.delete", Entity: "unit_member", EntityID: &current.ID, Before: before,
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.DeleteUnitMemberOutput{}, nil
}

// RegisterUnitMembers wires the {unitId}-scoped unit-member operations
// (design D-4's separate route shape from RegisterUnits's
// {communityId}-scoped unit creation).
func RegisterUnitMembers(api huma.API, d *Deps) {
	scoped.Unit(api, huma.Operation{
		OperationID: "listUnitMembers",
		Method:      "GET",
		Path:        "/v1/units/{unitId}/members",
		Summary:     "List a unit's members (any membership tied to the unit's community)",
		Tags:        []string{"units"},
	}, unitMemberReadRoles, d.ListUnitMembers)

	scoped.Unit(api, huma.Operation{
		OperationID: "updateUnitMember",
		Method:      "PATCH",
		Path:        "/v1/units/{unitId}/members/{memberId}",
		Summary:     "Update a unit member (admin/admin_staff only)",
		Tags:        []string{"units"},
	}, unitManageRoles, d.UpdateUnitMember)

	scoped.Unit(api, huma.Operation{
		OperationID: "deleteUnitMember",
		Method:      "DELETE",
		Path:        "/v1/units/{unitId}/members/{memberId}",
		Summary:     "Remove a unit member (admin/admin_staff only)",
		Tags:        []string{"units"},
	}, unitManageRoles, d.DeleteUnitMember)
}
