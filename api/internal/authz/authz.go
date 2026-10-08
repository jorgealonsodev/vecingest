// Package authz is the tenant-isolation enforcement layer PRD §7.7 names
// (design D-1/D-2): an unconstructible Membership type, typed scoped
// registration constructors, route-resource-only membership resolution,
// and a fail-closed boot assertion.
package authz

import "github.com/google/uuid"

// Kind is the scope a resolved Membership (or a scoped.* marker) carries.
type Kind int

const (
	// KindCommunity scopes to one communities.id.
	KindCommunity Kind = iota
	// KindOffice scopes to one offices.id.
	KindOffice
	// KindSelf carries no path resource: the caller's own full
	// membership set (scoped.Self).
	KindSelf
)

// String renders Kind for error messages (A1/A2 failure text).
func (k Kind) String() string {
	switch k {
	case KindCommunity:
		return "community"
	case KindOffice:
		return "office"
	case KindSelf:
		return "self"
	default:
		return "unknown"
	}
}

// Role is one of the four M1 membership roles (PRD §3).
type Role string

const (
	RoleAdmin      Role = "admin"
	RoleAdminStaff Role = "admin_staff"
	RoleOwner      Role = "owner"
	RoleTenant     Role = "tenant"
)

// scope is the resolved (kind, id) pair a Membership carries. Never
// exported: a caller can only ever hold Membership, never build a scope
// value itself.
type scope struct {
	kind Kind
	id   uuid.UUID
}

// Membership is unconstructible outside this package: every field is
// unexported, there is no exported constructor, and the zero value
// fails every accessor by panicking (authz-membership: "Zero-value
// membership rejected", "Membership only exists via resolution"). Only
// resolve.go's resolvers ever build a valid one.
type Membership struct {
	userID uuid.UUID
	scope  scope
	role   Role
	valid  bool
}

// Memberships is the caller's full membership set, passed to a
// scoped.Self handler. Like Membership, it carries no exported
// constructor: an outsider can build an empty or all-zero-value slice,
// but every element still fails its own accessors on access, so no
// valid membership can be fabricated from outside authz.
type Memberships []Membership

// CommunityScoped is the type constraint scoped.Community's input type
// parameter must satisfy: the input itself declares which community it
// targets, because huma parses path parameters into the typed input
// *I, and a single generic function cannot express three different
// input constraints.
type CommunityScoped interface {
	ScopeCommunityID() uuid.UUID
}

// OfficeScoped is scoped.Office's equivalent constraint.
type OfficeScoped interface {
	ScopeOfficeID() uuid.UUID
}

// UnitScoped is scoped.Unit's constraint (design D-4: the {unitId} route
// shape). The input declares which unit it targets; the community
// membership is resolved indirectly, via that unit's own community_id
// (authz.ResolveCommunityViaUnit), never a client-supplied community id.
type UnitScoped interface {
	ScopeUnitID() uuid.UUID
}

// InvitationScoped is scoped.Invitation's constraint (design D-4: the
// {invitationId} route shape, used by resend/revoke). The input
// declares which invitation it targets; the community membership is
// resolved indirectly, via that invitation's own community_id
// (authz.ResolveCommunityViaInvitation), never a client-supplied
// community id.
type InvitationScoped interface {
	ScopeInvitationID() uuid.UUID
}

// UserID returns the resolved user id. It panics on an invalid
// (zero-value or otherwise unresolved) Membership: unreachable by
// construction, since only this package's resolvers ever set valid.
func (m Membership) UserID() uuid.UUID {
	if !m.valid {
		panic("authz: Membership is invalid (zero value or not resolved)")
	}
	return m.userID
}

// Role returns the resolved role. Panics on an invalid Membership.
func (m Membership) Role() Role {
	if !m.valid {
		panic("authz: Membership is invalid (zero value or not resolved)")
	}
	return m.role
}

// CommunityID returns the resolved community id. Panics on an invalid
// Membership, or on a valid Membership whose scope is not
// KindCommunity -- calling the wrong accessor for the scope kind is
// also a programmer error this type refuses to serve.
func (m Membership) CommunityID() uuid.UUID {
	if !m.valid {
		panic("authz: Membership is invalid (zero value or not resolved)")
	}
	if m.scope.kind != KindCommunity {
		panic("authz: Membership is not community-scoped")
	}
	return m.scope.id
}

// OfficeID returns the resolved office id. Panics on an invalid
// Membership, or on a valid Membership whose scope is not KindOffice.
func (m Membership) OfficeID() uuid.UUID {
	if !m.valid {
		panic("authz: Membership is invalid (zero value or not resolved)")
	}
	if m.scope.kind != KindOffice {
		panic("authz: Membership is not office-scoped")
	}
	return m.scope.id
}

// Kind returns the resolved scope kind, letting a scoped.Self handler
// discriminate a caller's mixed office/community Memberships (e.g.
// office-management's GET /v1/offices/me) without guessing which
// per-kind accessor is safe to call. Panics on an invalid Membership,
// same as every other accessor.
func (m Membership) Kind() Kind {
	if !m.valid {
		panic("authz: Membership is invalid (zero value or not resolved)")
	}
	return m.scope.kind
}

// MetadataKey is the huma Operation.Metadata key scoped.* stamps and
// AssertScopedRegistration reads back (design D-2/V5: Operation.Metadata
// is yaml:"-" and therefore never reaches the published openapi.yaml).
const MetadataKey = "authz.scope"

// Marker is the registration-time value scoped.* stores at
// op.Metadata[MetadataKey]. AssertScopedRegistration's A2 check reads
// Kind to find this operation's resolver and Roles to confirm a
// non-empty allowed-role set for Community/Office operations.
type Marker struct {
	Kind  Kind
	Roles []Role
}
