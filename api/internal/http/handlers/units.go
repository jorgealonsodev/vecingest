package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/jorgealonsodev/vecingest/internal/authz"
	"github.com/jorgealonsodev/vecingest/internal/authz/scoped"
	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/audit"
	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
	"github.com/jorgealonsodev/vecingest/internal/http/dto"
)

// unitManageRoles is unit-management's write role set: admin and
// admin_staff, mirroring communityWriteRoles in communities.go -- an
// owner/tenant never reaches unit creation or unit-member management
// (unit-management: Unit Creation Scoped To Community, "Owner cannot
// create a unit").
var unitManageRoles = []authz.Role{authz.RoleAdmin, authz.RoleAdminStaff}

// unitMemberReadRoles is every role a resolved unit-community Membership
// can hold. unit-management: Unit Member Management Scoped To Community
// only requires SOME membership tied to the unit's community for the
// read path, never a role restriction -- mirroring communityReadRoles's
// identical reasoning in communities.go.
var unitMemberReadRoles = []authz.Role{authz.RoleAdmin, authz.RoleAdminStaff, authz.RoleOwner, authz.RoleTenant}

// coefficientTolerance is §5.2's "100 ± 0.01" tolerance band.
var coefficientTolerance = decimal.RequireFromString("0.01")

// coefficientTarget is the expected participation-coefficient sum.
var coefficientTarget = decimal.NewFromInt(100)

// CreateUnit implements POST /v1/communities/:id/units (unit-management:
// Unit Creation Scoped To Community; Unit Uniqueness Per Community;
// Participation Coefficient Sum Is A Warning, Not A Block; Unit Member
// Roles And Deferred board_role; Consent And Notification Fields
// Captured Per Member). Registered via scoped.Community with
// unitManageRoles=[admin, admin_staff]: an owner/tenant caller never
// reaches this handler (403).
//
// Members is created in the SAME transaction as the unit: this
// milestone has no invitation flow yet (Phase 6/WU-4), so a member
// entry MUST identify an existing account by email -- exactly
// office-management's "Office Staff Addition Restricted To Existing
// Accounts" pattern (dto.UnitMemberCreateRequest's doc comment).
func (d *Deps) CreateUnit(ctx context.Context, in *dto.CreateUnitInput, membership authz.Membership) (*dto.CreateUnitOutput, error) {
	coefficient, err := parseOptionalNumeric(in.Body.ParticipationCoefficient)
	if err != nil {
		return nil, apperr.New(400, apperr.CodeValidation, "invalid participation_coefficient", nil)
	}

	tx, err := d.DB.Write.Begin(ctx)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	unit, err := q.InsertUnit(ctx, db.InsertUnitParams{
		ID:                       uuid.New(),
		CommunityID:              membership.CommunityID(),
		Block:                    optionalText(in.Body.Block),
		Floor:                    optionalText(in.Body.Floor),
		Door:                     optionalText(in.Body.Door),
		Type:                     in.Body.Type,
		ParticipationCoefficient: coefficient,
		CadastralRef:             optionalText(in.Body.CadastralRef),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, apperr.New(409, apperr.CodeConflict, "a unit with this block/floor/door already exists in this community", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	members := make([]dto.UnitMemberResponse, 0, len(in.Body.Members))
	for _, memberReq := range in.Body.Members {
		user, err := q.GetUserByEmail(ctx, memberReq.Email)
		if err != nil {
			if isNoRows(err) {
				return nil, apperr.New(404, apperr.CodeNotFound, "no account exists for that email", nil)
			}
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}

		tenure := memberReq.Tenure
		if tenure == "" {
			tenure = "full_owner"
		}
		var consentAt pgtype.Timestamptz
		if memberReq.ElectronicNotificationsConsent {
			consentAt = pgtype.Timestamptz{Time: d.clock().Now(), Valid: true}
		}

		member, err := q.InsertUnitMember(ctx, db.InsertUnitMemberParams{
			ID:                               uuid.New(),
			UnitID:                           unit.ID,
			CommunityID:                      unit.CommunityID,
			UserID:                           user.ID,
			Role:                             memberReq.Role,
			Tenure:                           tenure,
			NotificationAddress:              optionalText(memberReq.NotificationAddress),
			ElectronicNotificationsConsentAt: consentAt,
			ConsentTextVersion:               optionalText(memberReq.ConsentTextVersion),
		})
		if err != nil {
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		members = append(members, unitMemberResponse(member))
	}

	sum, err := q.SumParticipationCoefficientByCommunityID(ctx, unit.CommunityID)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	callerID := membership.UserID()
	after, _ := json.Marshal(map[string]any{"unit_id": unit.ID, "community_id": unit.CommunityID, "member_count": len(members)})
	if _, err := audit.Append(ctx, tx, d.clock(), audit.Entry{
		UserID: &callerID, CommunityID: &unit.CommunityID, Action: "unit.create", Entity: "unit", EntityID: &unit.ID, After: after,
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.CreateUnitOutput{Body: dto.UnitCreateResponse{
		UnitResponse: unitResponse(unit),
		Members:      members,
		Warnings:     coefficientWarnings(sum),
	}}, nil
}

// coefficientWarnings implements unit-management: Participation
// Coefficient Sum Is A Warning, Not A Block -- a response-level warning,
// NEVER a write-time constraint, whenever a community's units sum
// outside 100 ± 0.01 (§5.2). sum arrives as pgtype.Numeric (this
// nullable-numeric codebase's convention, per communities.go's
// numericString) and is compared via shopspring/decimal, never float64
// (rules.apply.guidelines: coefficients are money-adjacent).
func coefficientWarnings(sum pgtype.Numeric) []string {
	s := numericString(sum)
	if s == "" {
		s = "0"
	}
	value, err := decimal.NewFromString(s)
	if err != nil {
		return nil
	}
	if value.Sub(coefficientTarget).Abs().GreaterThan(coefficientTolerance) {
		return []string{fmt.Sprintf("community participation coefficients sum to %s, expected 100 ± 0.01", value.String())}
	}
	return nil
}

func unitResponse(u db.Unit) dto.UnitResponse {
	return dto.UnitResponse{
		ID:                       u.ID,
		CommunityID:              u.CommunityID,
		Block:                    u.Block.String,
		Floor:                    u.Floor.String,
		Door:                     u.Door.String,
		Type:                     u.Type,
		ParticipationCoefficient: numericString(u.ParticipationCoefficient),
		CadastralRef:             u.CadastralRef.String,
		CreatedAt:                u.CreatedAt,
		UpdatedAt:                u.UpdatedAt,
	}
}

func unitMemberResponse(m db.UnitMember) dto.UnitMemberResponse {
	return dto.UnitMemberResponse{
		ID:                               m.ID,
		UnitID:                           m.UnitID,
		CommunityID:                      m.CommunityID,
		UserID:                           m.UserID,
		Role:                             m.Role,
		Tenure:                           m.Tenure,
		NotificationAddress:              m.NotificationAddress.String,
		ElectronicNotificationsConsentAt: timestamptzPtr(m.ElectronicNotificationsConsentAt),
		ConsentTextVersion:               m.ConsentTextVersion.String,
		CreatedAt:                        m.CreatedAt,
		UpdatedAt:                        m.UpdatedAt,
	}
}

// RegisterUnits wires unit-management's unit-creation operation into
// api (unit-management). See unit_members.go's RegisterUnitMembers for
// the {unitId}-scoped member endpoints (design D-4's separate route
// shape).
func RegisterUnits(api huma.API, d *Deps) {
	scoped.Community(api, huma.Operation{
		OperationID: "createUnit",
		Method:      "POST",
		Path:        "/v1/communities/{id}/units",
		Summary:     "Create a unit in a community (admin/admin_staff only)",
		Tags:        []string{"units"},
	}, unitManageRoles, d.CreateUnit)
}
