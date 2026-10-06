package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
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
	category := incidentPublicCategory(incident.Category)
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

const (
	defaultIncidentPageSize = 20
	maxIncidentPageSize     = 100
	maxIncidentCursorLength = 1024
)

var incidentListRoles = []authz.Role{
	authz.RoleAdmin, authz.RoleAdminStaff, authz.RoleOwner, authz.RoleTenant,
}

type incidentCursor struct {
	Version    int    `json:"v"`
	Community  string `json:"community"`
	Caller     string `json:"caller"`
	Status     string `json:"status"`
	Category   string `json:"category"`
	UnitID     string `json:"unit_id"`
	CreatedAt  string `json:"created_at"`
	IncidentID string `json:"id"`
}

type incidentCursorPivot struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// ListIncidents applies the existing tenant/caller visibility SQL for every
// page. The cursor is pagination state only: its anchor must itself remain
// visible and match the page filters, and the next page always reruns SQL.
func (d *Deps) ListIncidents(ctx context.Context, in *dto.ListIncidentsInput, membership authz.Membership) (*dto.ListIncidentsOutput, error) {
	communityID, callerID := membership.CommunityID(), membership.UserID()
	status, category, unitFilter := "", "", ""
	statusArg, categoryArg, unitArg := pgtype.Text{}, pgtype.Text{}, pgtype.UUID{}

	if in.Status != "" {
		status = in.Status
		if !validIncidentStatus(status) {
			return nil, incidentValidationError()
		}
		statusArg = pgtype.Text{String: status, Valid: true}
	}
	if in.Category != "" {
		category = in.Category
		stored, ok := incidentStorageCategory(category)
		if !ok {
			return nil, incidentValidationError()
		}
		categoryArg = pgtype.Text{String: stored, Valid: true}
	}
	if in.UnitID != "" {
		id, err := uuid.Parse(in.UnitID)
		if err != nil {
			return nil, incidentValidationError()
		}
		unitFilter = id.String()
		unitArg = pgtype.UUID{Bytes: id, Valid: true}
		unit, err := db.New(d.DB.Read).GetUnitByID(ctx, id)
		if err != nil {
			if isNoRows(err) {
				return nil, incidentValidationError()
			}
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		if unit.CommunityID != communityID {
			return nil, incidentValidationError()
		}
	}

	limit := in.Limit
	if limit == 0 {
		limit = defaultIncidentPageSize
	}
	if limit < 1 || limit > maxIncidentPageSize {
		return nil, incidentValidationError()
	}

	q := db.New(d.DB.Read)
	var pivot *incidentCursorPivot
	if in.Cursor != "" {
		decoded, err := decodeIncidentCursor(in.Cursor, communityID, callerID, status, category, unitFilter)
		if err != nil {
			return nil, incidentValidationError()
		}
		anchor, err := q.GetVisibleIncidentByID(ctx, db.GetVisibleIncidentByIDParams{
			ID: decoded.ID, CommunityID: communityID, UserID: callerID,
		})
		if err != nil {
			if isNoRows(err) {
				return nil, incidentValidationError()
			}
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		if !anchor.CreatedAt.Equal(decoded.CreatedAt) ||
			(status != "" && anchor.Status != status) ||
			(category != "" && incidentPublicCategory(anchor.Category) != category) ||
			(unitFilter != "" && (!anchor.UnitID.Valid || uuid.UUID(anchor.UnitID.Bytes).String() != unitFilter)) {
			return nil, incidentValidationError()
		}
		pivot = &decoded
	}

	query := db.ListVisibleIncidentsParams{
		CommunityID: communityID, UserID: callerID, Status: statusArg,
		Category: categoryArg, UnitID: unitArg, PageSize: int32(limit + 1),
	}
	if pivot != nil {
		query.CursorCreatedAt = pgtype.Timestamptz{Time: pivot.CreatedAt, Valid: true}
		query.CursorID = pgtype.UUID{Bytes: pivot.ID, Valid: true}
	}
	rows, err := q.ListVisibleIncidents(ctx, query)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	items := make([]dto.IncidentResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, incidentResponse(row))
	}
	next := ""
	if more {
		last := rows[len(rows)-1]
		next = encodeIncidentCursor(incidentCursor{
			Version: 1, Community: communityID.String(), Caller: callerID.String(),
			Status: status, Category: category, UnitID: unitFilter,
			CreatedAt: last.CreatedAt.UTC().Format(time.RFC3339Nano), IncidentID: last.ID.String(),
		})
	}
	return &dto.ListIncidentsOutput{Body: dto.ListIncidentsResponse{Items: items, NextCursor: next}}, nil
}

// GetIncident repeats the visibility SQL after scoped.Incident has minted the
// exact resource grant, so the handler only projects the safe C1 DTO.
func (d *Deps) GetIncident(ctx context.Context, _ *dto.GetIncidentInput, access authz.IncidentAccess) (*dto.GetIncidentOutput, error) {
	incident, err := db.New(d.DB.Read).GetVisibleIncidentByID(ctx, db.GetVisibleIncidentByIDParams{
		ID: access.IncidentID(), CommunityID: access.CommunityID(), UserID: access.CallerID(),
	})
	if err != nil {
		if isNoRows(err) {
			return nil, apperr.New(404, apperr.CodeNotFound, "not found", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	return &dto.GetIncidentOutput{Body: incidentResponse(incident)}, nil
}

func validIncidentStatus(status string) bool {
	switch status {
	case "open", "assigned", "in_progress", "resolved", "closed", "rejected":
		return true
	default:
		return false
	}
}

func incidentPublicCategory(category string) string {
	if category == "noise_and_coexistence" {
		return "noise"
	}
	return category
}

func encodeIncidentCursor(cursor incidentCursor) string {
	payload, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeIncidentCursor(encoded string, communityID, callerID uuid.UUID, status, category, unitID string) (incidentCursorPivot, error) {
	if encoded == "" || len(encoded) > maxIncidentCursorLength {
		return incidentCursorPivot{}, errors.New("invalid incident cursor length")
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || len(payload) > 768 {
		return incidentCursorPivot{}, errors.New("invalid incident cursor encoding")
	}
	if err := validateIncidentCursorJSON(payload); err != nil {
		return incidentCursorPivot{}, err
	}
	var cursor incidentCursor
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return incidentCursorPivot{}, err
	}
	if cursor.Version != 1 || cursor.Community != communityID.String() || cursor.Caller != callerID.String() ||
		cursor.Status != status || cursor.Category != category || cursor.UnitID != unitID {
		return incidentCursorPivot{}, errors.New("incident cursor context mismatch")
	}
	id, err := uuid.Parse(cursor.IncidentID)
	if err != nil || id.String() != cursor.IncidentID {
		return incidentCursorPivot{}, errors.New("invalid incident cursor id")
	}
	if !strings.HasSuffix(cursor.CreatedAt, "Z") {
		return incidentCursorPivot{}, errors.New("incident cursor timestamp must be UTC")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, cursor.CreatedAt)
	if err != nil || createdAt.UTC().Format(time.RFC3339Nano) != cursor.CreatedAt {
		return incidentCursorPivot{}, errors.New("invalid incident cursor timestamp")
	}
	return incidentCursorPivot{CreatedAt: createdAt, ID: id}, nil
}

func validateIncidentCursorJSON(payload []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("incident cursor must be an object")
	}
	allowed := map[string]bool{"v": true, "community": true, "caller": true, "status": true, "category": true, "unit_id": true, "created_at": true, "id": true}
	seen := make(map[string]bool, len(allowed))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok || !allowed[key] || seen[key] {
			return errors.New("invalid incident cursor field")
		}
		seen[key] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return err
		}
		if key == "v" {
			var version int
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, &version) != nil {
				return errors.New("invalid incident cursor version type")
			}
		} else {
			var text string
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, &text) != nil {
				return errors.New("invalid incident cursor field type")
			}
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if len(seen) != len(allowed) {
		return errors.New("incident cursor fields are incomplete")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("trailing incident cursor data")
	}
	return nil
}

// RegisterIncidents registers creation and list on the community resource,
// plus detail through the independent typed incident-access scope.
func RegisterIncidents(api huma.API, d *Deps) {
	scoped.Community(api, huma.Operation{
		OperationID: "listIncidents", Method: "GET", Path: "/v1/communities/{id}/incidents",
		Summary:     "List visible community incidents",
		Description: "Filters: status (open, assigned, in_progress, resolved, closed, rejected), category (documented public categories; noise maps to noise_and_coexistence), unit_id (a non-deleted unit in this community), limit (default 20, range 1–100), and cursor. Results use stable descending (created_at, id) keyset pagination. Each page rechecks visibility; stale cursors require restarting pagination.",
	}, incidentListRoles, d.ListIncidents)
	scoped.Community(api, huma.Operation{
		OperationID:   "createIncident",
		Method:        "POST",
		Path:          "/v1/communities/{id}/incidents",
		Summary:       "Create an incident",
		Description:   "Owners and tenants need active membership in the exact target unit for unit incidents. Tenants also require tenants_can_create_incidents (default true). Admin and admin_staff may target any active unit in their community. Common incidents omit unit_id. New incidents start open with normal priority; creator is the authenticated caller.",
		DefaultStatus: http.StatusCreated,
	}, incidentCreateRoles, d.CreateIncident)
	scoped.Incident(api, huma.Operation{
		OperationID: "getIncident", Method: "GET", Path: "/v1/incidents/{id}",
		Summary:     "Get a visible incident",
		Description: "Returns a safe incident projection only when the authenticated caller can currently view it; absent, deleted, foreign, and invisible incidents all return 404.",
	}, d.GetIncident)
}
