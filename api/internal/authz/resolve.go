// resolve.go implements the D-4 membership resolvers: one indexed
// lookup per scope kind, called by the scoped.* adapter before the
// handler runs. Resolution reads ONLY the route's own path resource
// (via the input's ScopeCommunityID()/ScopeOfficeID() method) and the
// already-authenticated caller's user id from context -- never a
// header or body (authz-membership: "Membership Resolved From Route
// Resource, Never From Header Or Body").
package authz

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jorgealonsodev/vecingest/internal/db"
)

// ErrNoMembership is returned when the caller has no membership row for
// the requested resource -- a foreign resource resolves to nothing,
// never partial data (design D-4: "Foreign resource → 404").
var ErrNoMembership = errors.New("authz: no membership for this resource")

// errNotConfigured is returned when a resolver runs before Configure
// was ever called (a boot-wiring bug, not a caller error).
var errNotConfigured = errors.New("authz: not configured (Configure was never called)")

// Querier is the narrow subset of sqlc-generated queries the resolvers
// call through, configured once at boot via Configure. Kept narrow
// (rather than the full generated db.Querier) so unit tests can satisfy
// it with a small fake instead of implementing dozens of unrelated
// methods; *db.Queries already satisfies this structurally.
type Querier interface {
	// GetOfficeMemberByOfficeAndUser is the office resolver (D-4):
	// office scope resolves by (office_id, user_id).
	GetOfficeMemberByOfficeAndUser(ctx context.Context, arg db.GetOfficeMemberByOfficeAndUserParams) (db.OfficeMember, error)
	// ResolveCommunityRoleViaOffice is the community resolver's
	// admin/office leg: an admin/admin_staff reaches a community whose
	// office_id matches one of their office_members rows -- never a
	// client-supplied office id (office-management: "Admin scope
	// derived from office_members").
	ResolveCommunityRoleViaOffice(ctx context.Context, arg db.ResolveCommunityRoleViaOfficeParams) (string, error)
	// ResolveCommunityRoleViaUnit is the community resolver's
	// owner/tenant leg.
	ResolveCommunityRoleViaUnit(ctx context.Context, arg db.ResolveCommunityRoleViaUnitParams) (string, error)
	// ListOfficeMembershipsByUserID and ListUnitMembershipsByUserID
	// back the Self resolver (GET /v1/me's full membership set).
	ListOfficeMembershipsByUserID(ctx context.Context, userID uuid.UUID) ([]db.ListOfficeMembershipsByUserIDRow, error)
	ListUnitMembershipsByUserID(ctx context.Context, userID uuid.UUID) ([]db.ListUnitMembershipsByUserIDRow, error)
	// GetUnitCommunityID is the unit resolver's own lookup (design D-4:
	// the {unitId} route shape resolves community membership via the
	// unit's own community_id, one join, then the community resolver's
	// predicate).
	GetUnitCommunityID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
}

var queries Querier

// Configure wires the sqlc Querier every resolver call goes through.
// MUST be called once during boot (composition root), before any
// scoped request is served.
func Configure(q Querier) { queries = q }

// isNoRows reports whether err is the sqlc/pgx "no matching row" error,
// distinguishing "caller has no membership" from a real infrastructure
// failure.
func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// ResolveCommunity resolves the caller's membership for communityID via
// office_members ∪ unit_members for that community_id (design D-4):
// the office leg first (admin/admin_staff, via the community's own
// office_id), then the unit leg (owner/tenant). A caller present in
// neither returns ErrNoMembership.
func ResolveCommunity(ctx context.Context, communityID, userID uuid.UUID) (Membership, error) {
	if queries == nil {
		return Membership{}, errNotConfigured
	}

	role, err := queries.ResolveCommunityRoleViaOffice(ctx, db.ResolveCommunityRoleViaOfficeParams{
		CommunityID: communityID,
		UserID:      userID,
	})
	if err == nil {
		return Membership{userID: userID, scope: scope{kind: KindCommunity, id: communityID}, role: Role(role), valid: true}, nil
	}
	if !isNoRows(err) {
		return Membership{}, err
	}

	role, err = queries.ResolveCommunityRoleViaUnit(ctx, db.ResolveCommunityRoleViaUnitParams{
		CommunityID: communityID,
		UserID:      userID,
	})
	if err == nil {
		return Membership{userID: userID, scope: scope{kind: KindCommunity, id: communityID}, role: Role(role), valid: true}, nil
	}
	if isNoRows(err) {
		return Membership{}, ErrNoMembership
	}
	return Membership{}, err
}

// ResolveOffice resolves the caller's membership for officeID via
// (office_id, user_id) (design D-4).
func ResolveOffice(ctx context.Context, officeID, userID uuid.UUID) (Membership, error) {
	if queries == nil {
		return Membership{}, errNotConfigured
	}

	row, err := queries.GetOfficeMemberByOfficeAndUser(ctx, db.GetOfficeMemberByOfficeAndUserParams{
		OfficeID: officeID,
		UserID:   userID,
	})
	if err != nil {
		if isNoRows(err) {
			return Membership{}, ErrNoMembership
		}
		return Membership{}, err
	}
	return Membership{userID: userID, scope: scope{kind: KindOffice, id: officeID}, role: Role(row.Role), valid: true}, nil
}

// ResolveCommunityViaUnit resolves the caller's membership for the
// community owning unitID (design D-4: the {unitId} route shape
// resolves via units.community_id, one join, then the community
// resolver's predicate above). A unit that does not exist, or is
// soft-deleted, resolves to ErrNoMembership -- exactly like a foreign
// community -- so a caller learns nothing about whether the unit id
// itself is valid (D-4: "Foreign resource → 404").
func ResolveCommunityViaUnit(ctx context.Context, unitID, userID uuid.UUID) (Membership, error) {
	if queries == nil {
		return Membership{}, errNotConfigured
	}

	communityID, err := queries.GetUnitCommunityID(ctx, unitID)
	if err != nil {
		if isNoRows(err) {
			return Membership{}, ErrNoMembership
		}
		return Membership{}, err
	}

	return ResolveCommunity(ctx, communityID, userID)
}

// ResolveSelf resolves the caller's full membership set (design D-4:
// "no path resource ⇒ scoped.Self ⇒ the caller's full membership set"),
// for GET /v1/me and GET /v1/communities.
func ResolveSelf(ctx context.Context, userID uuid.UUID) (Memberships, error) {
	if queries == nil {
		return nil, errNotConfigured
	}

	officeRows, err := queries.ListOfficeMembershipsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	unitRows, err := queries.ListUnitMembershipsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	memberships := make(Memberships, 0, len(officeRows)+len(unitRows))
	for _, r := range officeRows {
		memberships = append(memberships, Membership{userID: userID, scope: scope{kind: KindOffice, id: r.OfficeID}, role: Role(r.Role), valid: true})
	}
	for _, r := range unitRows {
		memberships = append(memberships, Membership{userID: userID, scope: scope{kind: KindCommunity, id: r.CommunityID}, role: Role(r.Role), valid: true})
	}
	return memberships, nil
}

// RoleAllowed reports whether role appears in allowed -- the D-2 "Role
// Authorization Within A Resolved Scope" check scoped.Community/
// scoped.Office run after a successful resolution.
func RoleAllowed(role Role, allowed []Role) bool {
	for _, r := range allowed {
		if r == role {
			return true
		}
	}
	return false
}
