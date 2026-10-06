// resolve.go implements scoped resource resolution before the handler
// runs. Membership resolvers use one indexed lookup per scope kind;
// incident resolution separately requires a visibility-aware detail
// query. Every resolver reads its path resource from the typed input and
// caller identity from authenticated context, never from a header or
// body (authz-membership: "Membership Resolved From Route Resource,
// Never From Header Or Body").
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

// ErrNoIncidentAccess means the incident is absent, deleted, or is not
// visible to the authenticated caller; scoped.Incident maps it to an
// opaque 404 without invoking the handler.
var ErrNoIncidentAccess = errors.New("authz: no visible incident for this caller")

// ErrNoAuthenticatedCaller is returned when an incident resolver has no
// authenticated identity in its context.
var ErrNoAuthenticatedCaller = errors.New("authz: no authenticated caller in context")

// ErrMFAEnrollmentRequired is returned when the caller's resolved role
// is admin or admin_staff and NO second factor exists on their account
// yet (auth-mfa-totp delta: Mandatory TOTP For Admin And Admin_staff
// Scope Access; design D-7). It is DISTINCT from ErrNoMembership: the
// caller genuinely has the membership, they just cannot use it until
// they enroll -- scoped.* renders this as its own 403 code
// (apperr.CodeMFAEnrollmentRequired), never the generic 404 a foreign
// resource gets.
var ErrMFAEnrollmentRequired = errors.New("authz: TOTP enrollment required for this role")

// ErrMFAAuthenticationRequired is returned when the caller's resolved
// role is admin or admin_staff, a second factor DOES exist on their
// account, and the session in hand simply never used it -- it
// authenticated on a password alone.
//
// It is deliberately distinct from ErrMFAEnrollmentRequired because the
// two demand different things of the client: enroll a factor (a flow
// that starts at /v1/me/mfa/enroll) versus log in again carrying a code
// (a flow that starts at /v1/auth/login). Collapsing them would send a
// caller who already has an authenticator app to an enrollment screen
// that refuses them with a 409.
var ErrMFAAuthenticationRequired = errors.New("authz: second-factor authentication required for this role")

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
	// GetInvitationCommunityID is the invitation resolver's own lookup
	// (design D-4: the {invitationId} route shape resolves community
	// membership via the invitation's own community_id). It returns the
	// community id regardless of the invitation's status: a revoked,
	// accepted or blocked invitation is still tied to a real community
	// for cross-tenant isolation purposes (invitations spec:
	// "Cross-Tenant Isolation Proven By Test").
	GetInvitationCommunityID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
	// GetIncidentCommunityID resolves the route incident's owning community
	// while excluding deleted incidents and communities.
	GetIncidentCommunityID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
	// GetVisibleIncidentByID is the visibility proof for an incident grant;
	// it binds incident, community and authenticated caller together.
	GetVisibleIncidentByID(ctx context.Context, arg db.GetVisibleIncidentByIDParams) (db.Incident, error)
	// IsUserMFAEnabled tells the mandatory-TOTP gate's two failure modes
	// apart (auth-mfa-totp delta): no factor on the account means the
	// caller must ENROLL, a factor that this session simply never used
	// means they must LOG IN AGAIN with a code. It no longer decides
	// whether the gate opens -- the session's own fact does. It always
	// returns exactly one row (true/false), never pgx.ErrNoRows -- a
	// caller who never enrolled resolves cleanly to false.
	IsUserMFAEnabled(ctx context.Context, userID uuid.UUID) (bool, error)
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

// requireMFAForAdminRoles implements the mandatory-TOTP gate (design
// D-7; auth-mfa-totp delta: Mandatory TOTP For Admin And Admin_staff
// Scope Access) at the ONE point every community/office resolution
// passes through, regardless of which route shape got there
// ({communityId}, {officeId}, {unitId} or {invitationId} all end up
// here). owner/tenant (and any role neither admin nor admin_staff) are
// untouched -- TOTP stays optional for them.
//
// What it enforces is SECOND-FACTOR AUTHENTICATION OF THIS SESSION, not
// enrollment on the account. The distinction is the whole fix for
// R1-mandatory-totp-gate-is-only-an-enrollment-flag (review lineages
// review-0e1833930adf141a and review-e72754dc7521b57a): the previous
// implementation read IsUserMFAEnabled, a durable per-account flag, so
// a session that had proved nothing but a password satisfied it as long
// as the victim had ever enrolled -- and where the victim had not, that
// same session could enroll a fresh factor through the ungated
// /v1/me/mfa/enroll + /v1/me/mfa/verify pair and pass. Reading the
// session's own fact closes both: an attacker may still enroll, but the
// session they hold was minted with mfa=false and no endpoint can
// promote it. Only a fresh login through the TOTP challenge produces an
// elevated session.
//
// The account query survives for ONE purpose: telling the two failure
// modes apart. An admin with no factor at all must be sent to enroll
// (the bootstrap path, which is why the enroll endpoints stay reachable
// from a password-only session); an admin who HAS a factor must be sent
// back to log in with a code. Those are different screens, so they are
// different errors.
func requireMFAForAdminRoles(ctx context.Context, role Role, userID uuid.UUID) error {
	if role != RoleAdmin && role != RoleAdminStaff {
		return nil
	}
	if MFAAuthenticatedFromContext(ctx) {
		return nil
	}
	enrolled, err := queries.IsUserMFAEnabled(ctx, userID)
	if err != nil {
		return err
	}
	if !enrolled {
		return ErrMFAEnrollmentRequired
	}
	return ErrMFAAuthenticationRequired
}

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
		if merr := requireMFAForAdminRoles(ctx, Role(role), userID); merr != nil {
			return Membership{}, merr
		}
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
		if merr := requireMFAForAdminRoles(ctx, Role(role), userID); merr != nil {
			return Membership{}, merr
		}
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
	if merr := requireMFAForAdminRoles(ctx, Role(row.Role), userID); merr != nil {
		return Membership{}, merr
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

// ResolveCommunityViaInvitation resolves the caller's membership for the
// community owning invitationID (design D-4: the {invitationId} route
// shape resolves via invitations.community_id, one join, then the
// community resolver's predicate above, exactly mirroring
// ResolveCommunityViaUnit). An invitation that does not exist resolves
// to ErrNoMembership -- exactly like a foreign community -- so a caller
// learns nothing about whether the invitation id itself is valid (D-4:
// "Foreign resource → 404").
func ResolveCommunityViaInvitation(ctx context.Context, invitationID, userID uuid.UUID) (Membership, error) {
	if queries == nil {
		return Membership{}, errNotConfigured
	}

	communityID, err := queries.GetInvitationCommunityID(ctx, invitationID)
	if err != nil {
		if isNoRows(err) {
			return Membership{}, ErrNoMembership
		}
		return Membership{}, err
	}

	return ResolveCommunity(ctx, communityID, userID)
}

// ResolveIncidentAccess derives caller identity only from ctx, resolves
// the route incident's owning community, and requires the existing
// visibility-aware detail query to return that exact resource before it
// mints an IncidentAccess. It deliberately does not resolve membership:
// the incident's creator grant survives membership expiry.
func ResolveIncidentAccess(ctx context.Context, incidentID uuid.UUID) (IncidentAccess, error) {
	callerID, ok := UserIDFromContext(ctx)
	if !ok || callerID == uuid.Nil {
		return IncidentAccess{}, ErrNoAuthenticatedCaller
	}
	if incidentID == uuid.Nil {
		return IncidentAccess{}, ErrNoIncidentAccess
	}
	if queries == nil {
		return IncidentAccess{}, errNotConfigured
	}

	communityID, err := queries.GetIncidentCommunityID(ctx, incidentID)
	if err != nil {
		if isNoRows(err) {
			return IncidentAccess{}, ErrNoIncidentAccess
		}
		return IncidentAccess{}, err
	}
	if communityID == uuid.Nil {
		return IncidentAccess{}, errors.New("authz: incident lookup returned an empty community id")
	}

	visible, err := queries.GetVisibleIncidentByID(ctx, db.GetVisibleIncidentByIDParams{
		ID: incidentID, CommunityID: communityID, UserID: callerID,
	})
	if err != nil {
		if isNoRows(err) {
			return IncidentAccess{}, ErrNoIncidentAccess
		}
		return IncidentAccess{}, err
	}
	if visible.ID != incidentID || visible.CommunityID != communityID {
		return IncidentAccess{}, errors.New("authz: visibility lookup returned a different incident resource")
	}

	return IncidentAccess{incidentID: incidentID, communityID: communityID, callerID: callerID, valid: true}, nil
}

// ResolveSelf resolves the caller's full membership set (design D-4:
// "no path resource ⇒ scoped.Self ⇒ the caller's full membership set"),
// for GET /v1/communities and the /v1/offices/me routes.
//
// It runs the SAME mandatory-TOTP gate ResolveCommunity and ResolveOffice
// run, over every admin/admin_staff membership it is about to hand out
// (R1-self-scope-skips-mandatory-totp-gate, review lineage
// review-c4efc3f92d076299). It previously ran no gate at all, on the
// assumption that a route with no path resource grants nothing --
// which is false: addOfficeMember is registered through scoped.Self and
// inserts an office_members row with role admin_staff, granting
// office-wide reach into every community of that office through the
// community resolver's own office leg.
//
// The gate belongs HERE, on the whole route class, rather than on the
// subset of Self routes someone judges privileged. Two reasons:
//
//  1. The privilege IS the membership. This function's entire output is
//     the admin membership set; a handler that receives it can already
//     act on it, so deciding per route means deciding again in every
//     handler -- the manual, per-handler authorization check the scoped.*
//     mechanism exists to replace, and the one that let this through.
//  2. It makes the default safe. A Self route added tomorrow inherits
//     the gate without anyone remembering to classify it. "Self means
//     harmless" is exactly the assumption that produced this hole, and
//     a per-route rule would preserve it as the default.
//
// The deliberate cost: an admin who has not enrolled loses GET
// /v1/communities and the /v1/offices/me routes until they do. The
// bootstrap path survives intact because it is not Self-registered --
// GET /v1/me, POST /v1/me/mfa/enroll and POST /v1/me/mfa/verify are
// plain huma.Register operations (handlers.RegisterMe/RegisterMFA), so
// an un-enrolled admin can still read their profile, enroll a factor,
// and log in again with a code.
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
		if merr := requireMFAForAdminRoles(ctx, Role(r.Role), userID); merr != nil {
			return nil, merr
		}
		memberships = append(memberships, Membership{userID: userID, scope: scope{kind: KindOffice, id: r.OfficeID}, role: Role(r.Role), valid: true})
	}
	for _, r := range unitRows {
		// A no-op today (the unit_members.role CHECK permits only
		// owner/tenant), kept for the same reason the gate lives in this
		// function at all: if that role set ever widens, the safe
		// behaviour must be the one already written down.
		if merr := requireMFAForAdminRoles(ctx, Role(r.Role), userID); merr != nil {
			return nil, merr
		}
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
