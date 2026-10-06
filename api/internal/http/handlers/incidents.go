package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jorgealonsodev/vecingest/internal/authz"
	"github.com/jorgealonsodev/vecingest/internal/authz/scoped"
	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
	"github.com/jorgealonsodev/vecingest/internal/http/dto"
)

var incidentCreateRoles = []authz.Role{
	authz.RoleAdmin, authz.RoleAdminStaff, authz.RoleOwner, authz.RoleTenant,
}

// CreateIncident implements the create-only incident slice. Community scope
// and caller identity come from scoped.Community's resolved membership, not
// from a client-supplied tenant or creator id.
func (d *Deps) CreateIncident(ctx context.Context, in *dto.CreateIncidentInput, membership authz.Membership) (*dto.CreateIncidentOutput, error) {
	body := in.Body
	title := strings.TrimSpace(body.Title)
	description := strings.TrimSpace(body.Description)
	location := strings.TrimSpace(body.LocationText)
	if utf8.RuneCountInString(title) < 1 || utf8.RuneCountInString(title) > 200 ||
		utf8.RuneCountInString(description) < 1 || utf8.RuneCountInString(description) > 5000 ||
		utf8.RuneCountInString(location) > 255 {
		return nil, incidentValidationError()
	}
	category, ok := incidentStorageCategory(body.Category)
	if !ok || (body.Scope != "common" && body.Scope != "unit") {
		return nil, incidentValidationError()
	}

	var unitID pgtype.UUID
	var targetID uuid.UUID
	if body.Scope == "common" {
		if body.UnitID != "" {
			return nil, incidentValidationError()
		}
	} else {
		parsed, err := uuid.Parse(string(body.UnitID))
		if err != nil {
			return nil, incidentValidationError()
		}
		targetID = parsed
		unitID = pgtype.UUID{Bytes: parsed, Valid: true}
	}

	tx, err := d.DB.Write.Begin(ctx)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	community, err := q.GetCommunityByID(ctx, membership.CommunityID())
	if err != nil {
		if isNoRows(err) {
			return nil, apperr.New(404, apperr.CodeNotFound, "not found", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	tenantsMayCreate, err := tenantIncidentCreationAllowed(community.Settings)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	// A supplied unit is a resource reference: validate its active state and
	// tenant before applying role/settings refusals, so foreign units stay 404.
	if body.Scope == "unit" {
		unit, err := q.GetUnitByID(ctx, targetID)
		if err != nil {
			if isNoRows(err) {
				return nil, apperr.New(404, apperr.CodeNotFound, "unit not found", nil)
			}
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		if unit.CommunityID != membership.CommunityID() {
			return nil, apperr.New(404, apperr.CodeNotFound, "unit not found", nil)
		}
	}

	switch membership.Role() {
	case authz.RoleAdmin, authz.RoleAdminStaff:
	case authz.RoleOwner, authz.RoleTenant:
		// The scoped community resolver establishes route membership, but its
		// legacy query intentionally ignores unit validity. Re-prove current
		// community membership locally before allowing any incident scope, and
		// derive tenant-setting policy from current role evidence so a stale
		// owner row cannot mask an active tenant row. A valid active owner grant
		// takes priority over the tenant-only setting restriction.
		activeRoles, err := q.GetActiveIncidentCommunityRoles(ctx, db.GetActiveIncidentCommunityRolesParams{
			CommunityID: membership.CommunityID(), UserID: membership.UserID(),
		})
		if err != nil {
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		if !activeRoles.HasActiveOwner && !activeRoles.HasActiveTenant {
			return nil, apperr.New(403, apperr.CodeUnauthorized, "forbidden", nil)
		}
		if activeRoles.HasActiveTenant && !activeRoles.HasActiveOwner && !tenantsMayCreate {
			return nil, apperr.New(403, apperr.CodeUnauthorized, "incident creation is disabled for tenants", nil)
		}
		if body.Scope == "unit" {
			active, err := q.HasActiveUnitMembership(ctx, db.HasActiveUnitMembershipParams{
				CommunityID: membership.CommunityID(), UnitID: targetID, UserID: membership.UserID(),
			})
			if err != nil {
				return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
			}
			if !active {
				return nil, apperr.New(403, apperr.CodeUnauthorized, "forbidden", nil)
			}
		}
	default:
		return nil, apperr.New(403, apperr.CodeUnauthorized, "forbidden", nil)
	}

	incident, err := q.InsertIncident(ctx, db.InsertIncidentParams{
		ID: uuid.New(), CommunityID: membership.CommunityID(), UnitID: unitID,
		CreatedBy: membership.UserID(), Title: title, Description: description,
		Category: category, Scope: body.Scope, LocationText: optionalText(location),
	})
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.CreateIncidentOutput{Body: incidentResponse(incident)}, nil
}

func tenantIncidentCreationAllowed(settings json.RawMessage) (bool, error) {
	if len(bytes.TrimSpace(settings)) == 0 {
		return true, nil
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(settings, &values); err != nil || values == nil {
		return false, errInvalidIncidentSettings
	}
	raw, exists := values["tenants_can_create_incidents"]
	if !exists {
		return true, nil
	}
	value := string(bytes.TrimSpace(raw))
	if value != "true" && value != "false" {
		return false, errInvalidIncidentSettings
	}
	var allowed bool
	if err := json.Unmarshal(raw, &allowed); err != nil {
		return false, errInvalidIncidentSettings
	}
	return allowed, nil
}

var errInvalidIncidentSettings = errors.New("invalid tenants_can_create_incidents setting")

func incidentStorageCategory(category string) (string, bool) {
	if category == "noise" {
		// The existing database enum stores this PRD category under its
		// earlier label; keep the public transport vocabulary stable.
		return "noise_and_coexistence", true
	}
	switch category {
	case "elevator", "plumbing", "electricity", "cleaning", "locksmith", "gardening", "works", "mandatory_works", "other":
		return category, true
	default:
		return "", false
	}
}

func incidentValidationError() error {
	return apperr.New(400, apperr.CodeValidation, "invalid incident request", nil)
}

func incidentResponse(incident db.Incident) dto.IncidentResponse {
	var unitID *uuid.UUID
	if incident.UnitID.Valid {
		id := uuid.UUID(incident.UnitID.Bytes)
		unitID = &id
	}
	category := incident.Category
	if category == "noise_and_coexistence" {
		category = "noise"
	}
	location := ""
	if incident.LocationText.Valid {
		location = incident.LocationText.String
	}
	return dto.IncidentResponse{
		ID: incident.ID, CommunityID: incident.CommunityID, UnitID: unitID,
		CreatedBy: incident.CreatedBy, Title: incident.Title, Description: incident.Description,
		Category: category, Priority: incident.Priority, Status: incident.Status,
		Scope: incident.Scope, LocationText: location, AffectedCount: incident.AffectedCount,
		CreatedAt: incident.CreatedAt, UpdatedAt: incident.UpdatedAt,
	}
}

// RegisterIncidents registers only POST creation in C1; list/detail are a
// separate C2 slice. All community member roles reach the handler for the
// exact unit-level checks and tenant setting policy above.
func RegisterIncidents(api huma.API, d *Deps) {
	scoped.Community(api, huma.Operation{
		OperationID:   "createIncident",
		Method:        "POST",
		Path:          "/v1/communities/{id}/incidents",
		Summary:       "Create an incident",
		Description:   "Owners and tenants need active membership in the exact target unit for unit incidents. Tenants also require tenants_can_create_incidents (default true). Admin and admin_staff may target any active unit in their community. Common incidents omit unit_id. New incidents start open with normal priority; creator is the authenticated caller.",
		DefaultStatus: http.StatusCreated,
	}, incidentCreateRoles, d.CreateIncident)
}
