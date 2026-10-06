package handlers

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
	"github.com/jorgealonsodev/vecingest/internal/http/dto"
)

// Me implements GET /v1/me (user-profile: GET /v1/me Response Shape):
// id, email, is_superadmin and the caller's memberships, each entry
// discriminated by scope.
//
// It deliberately stays a plain huma.Register operation, not
// scoped.Self, and lists the caller's membership rows through
// listOwnMemberships rather than authz.ResolveSelf. ResolveSelf runs the
// mandatory-TOTP gate over the whole Self route class, and its own doc
// names GET /v1/me as part of the bootstrap path that must survive it:
// an admin with no second factor yet has to be able to read their
// profile before enrolling one. Listing one's own memberships confers no
// privilege -- every route that ACTS on a membership still resolves it
// through authz, gate included.
//
// Membership state is read fresh on every request, never cached (design
// interfaces table: "Membership lookups are deliberately not cached"),
// so it is independent of session state: revoking a session changes
// neither what this lists nor how (user-profile: Remote Session Listing
// and Revocation Endpoints).
func (d *Deps) Me(ctx context.Context, in *dto.MeInput) (*dto.MeOutput, error) {
	claims, err := d.authenticate(ctx, in.Authorization)
	if err != nil {
		return nil, unauthorized(err)
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, apperr.New(401, apperr.CodeUnauthorized, "invalid token subject", nil)
	}

	q := db.New(d.DB.Read)
	user, err := q.GetUserByID(ctx, userID)
	if err != nil {
		if isNoRows(err) {
			return nil, apperr.New(401, apperr.CodeUnauthorized, "user not found", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	memberships, err := listOwnMemberships(ctx, q, user.ID)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.MeOutput{Body: dto.MeResponse{
		ID:           user.ID,
		Email:        user.Email,
		IsSuperadmin: user.IsSuperadmin,
		Memberships:  memberships,
	}}, nil
}

// listOwnMemberships returns userID's office entries followed by their
// community entries, each leg ordered by name. The result is never nil,
// so a caller with no rows renders as an empty JSON array.
func listOwnMemberships(ctx context.Context, q *db.Queries, userID uuid.UUID) ([]dto.MembershipEntry, error) {
	officeRows, err := q.ListOfficeMembershipSummariesByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	communityRows, err := q.ListCommunityMembershipSummariesByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := make([]dto.MembershipEntry, 0, len(officeRows)+len(communityRows))
	for _, r := range officeRows {
		out = append(out, dto.MembershipEntry{
			Scope: dto.MembershipScopeOffice, ID: r.OfficeID, Name: r.OfficeName, Role: r.Role,
		})
	}
	for _, r := range communityRows {
		out = append(out, dto.MembershipEntry{
			Scope: dto.MembershipScopeCommunity, ID: r.CommunityID, Name: r.CommunityName, Role: r.Role,
		})
	}
	return out, nil
}

// ListSessions implements GET /v1/me/sessions (user-profile: Remote
// Session Listing and Revocation Endpoints). It lists LIVE sessions in
// the caller's own family only -- device/activity data, never a token
// hash.
func (d *Deps) ListSessions(ctx context.Context, in *dto.ListSessionsInput) (*dto.ListSessionsOutput, error) {
	claims, err := d.authenticate(ctx, in.Authorization)
	if err != nil {
		return nil, unauthorized(err)
	}
	familyID, err := uuid.Parse(claims.SID)
	if err != nil {
		return nil, apperr.New(401, apperr.CodeUnauthorized, "invalid session claim", nil)
	}

	q := db.New(d.DB.Read)
	rows, err := q.ListLiveSessionsByFamilyID(ctx, familyID)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	summaries := make([]dto.SessionSummary, 0, len(rows))
	for _, r := range rows {
		s := dto.SessionSummary{
			FamilyID:  r.FamilyID,
			Platform:  r.Platform,
			CreatedAt: r.CreatedAt,
		}
		if r.DeviceName.Valid {
			s.DeviceName = r.DeviceName.String
		}
		if r.LastUsedAt.Valid {
			t := r.LastUsedAt.Time
			s.LastUsedAt = &t
		}
		summaries = append(summaries, s)
	}

	return &dto.ListSessionsOutput{Body: dto.ListSessionsResponse{Sessions: summaries}}, nil
}

// RevokeSession implements DELETE /v1/me/sessions/:id (user-profile:
// Remote Session Listing and Revocation Endpoints; auth-session-tokens:
// Session Tracking and Remote Revocation). :id is the family_id; a
// foreign family (one the caller does not own) returns 404, never 403
// -- the spec's explicit choice, so the endpoint never confirms or
// denies that a given family id exists to a caller who doesn't own it.
func (d *Deps) RevokeSession(ctx context.Context, in *dto.RevokeSessionInput) (*dto.RevokeSessionOutput, error) {
	claims, err := d.authenticate(ctx, in.Authorization)
	if err != nil {
		return nil, unauthorized(err)
	}
	callerUserID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, apperr.New(401, apperr.CodeUnauthorized, "invalid token subject", nil)
	}
	targetFamilyID, err := uuid.Parse(in.ID)
	if err != nil {
		return nil, apperr.New(404, apperr.CodeNotFound, "session not found", nil)
	}

	q := db.New(d.DB.Write)
	rows, err := q.ListLiveSessionsByFamilyID(ctx, targetFamilyID)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if len(rows) == 0 || rows[0].UserID != callerUserID {
		return nil, apperr.New(404, apperr.CodeNotFound, "session not found", nil)
	}

	if err := q.RevokeSessionFamily(ctx, targetFamilyID); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if d.RevocationCache != nil {
		d.RevocationCache.MarkRevoked(targetFamilyID)
	}

	return &dto.RevokeSessionOutput{}, nil
}

func unauthorized(err error) error {
	if errors.Is(err, ErrMissingBearer) {
		return apperr.New(401, apperr.CodeUnauthorized, "missing or invalid Authorization header", nil)
	}
	return apperr.New(401, apperr.CodeUnauthorized, "invalid or expired access token", nil)
}

// RegisterMe wires GET /v1/me, GET /v1/me/sessions and DELETE
// /v1/me/sessions/:id into api.
func RegisterMe(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "getMe",
		Method:      "GET",
		Path:        "/v1/me",
		Summary:     "Get the caller's own profile",
		Tags:        []string{"me"},
	}, d.Me)

	huma.Register(api, huma.Operation{
		OperationID: "listMySessions",
		Method:      "GET",
		Path:        "/v1/me/sessions",
		Summary:     "List the caller's own live sessions",
		Tags:        []string{"me"},
	}, d.ListSessions)

	huma.Register(api, huma.Operation{
		OperationID: "revokeMySession",
		Method:      "DELETE",
		Path:        "/v1/me/sessions/{id}",
		Summary:     "Revoke one of the caller's own sessions",
		Tags:        []string{"me"},
	}, d.RevokeSession)
}
