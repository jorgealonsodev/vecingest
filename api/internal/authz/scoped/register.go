// Package scoped provides the D-1 typed registration constructors
// (Community, Office, Self) every tenant-scoped huma operation MUST
// register through. Each constructor stamps the D-2 registration
// marker onto the operation so the boot assertion (authz.
// AssertScopedRegistration) can find it. Membership resolution (D-4)
// and the role check (D-2 "Role Authorization Within A Resolved Scope")
// are wired in by tasks 1.13/1.17.
package scoped

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jorgealonsodev/vecingest/internal/authz"
	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
)

// stampMarker records the D-2 registration marker on op, so A1/A2's
// Paths walk can find it and so V8's PrefixModifier shallow-copy
// carries it through huma.NewGroup prefixing unchanged.
func stampMarker(op *huma.Operation, kind authz.Kind, roles []authz.Role) {
	if op.Metadata == nil {
		op.Metadata = map[string]any{}
	}
	op.Metadata[authz.MetadataKey] = authz.Marker{Kind: kind, Roles: roles}
}

// Community registers a community-scoped operation. PI is the
// pointer-receiver constraint that forces *I to declare its tenant id
// via authz.CommunityScoped, because huma parses path parameters into
// the typed input *I and a single generic function cannot express three
// different input constraints (design D-1).
func Community[I any, O any, PI interface {
	*I
	authz.CommunityScoped
}](api huma.API, op huma.Operation, roles []authz.Role, handler func(context.Context, PI, authz.Membership) (*O, error)) {
	stampMarker(&op, authz.KindCommunity, roles)

	huma.Register(api, op, func(ctx context.Context, in *I) (*O, error) {
		pi := PI(in)
		userID, ok := authz.UserIDFromContext(ctx)
		if !ok {
			return nil, apperr.New(401, apperr.CodeUnauthorized, "missing authenticated caller", nil)
		}
		membership, err := authz.ResolveCommunity(ctx, pi.ScopeCommunityID(), userID)
		if err != nil {
			return nil, resolveErrorResponse(err)
		}
		if !authz.RoleAllowed(membership.Role(), roles) {
			return nil, forbidden()
		}
		return handler(ctx, pi, membership)
	})
}

// Office registers an office-scoped operation, mirroring Community for
// authz.OfficeScoped inputs.
func Office[I any, O any, PI interface {
	*I
	authz.OfficeScoped
}](api huma.API, op huma.Operation, roles []authz.Role, handler func(context.Context, PI, authz.Membership) (*O, error)) {
	stampMarker(&op, authz.KindOffice, roles)

	huma.Register(api, op, func(ctx context.Context, in *I) (*O, error) {
		pi := PI(in)
		userID, ok := authz.UserIDFromContext(ctx)
		if !ok {
			return nil, apperr.New(401, apperr.CodeUnauthorized, "missing authenticated caller", nil)
		}
		membership, err := authz.ResolveOffice(ctx, pi.ScopeOfficeID(), userID)
		if err != nil {
			return nil, resolveErrorResponse(err)
		}
		if !authz.RoleAllowed(membership.Role(), roles) {
			return nil, forbidden()
		}
		return handler(ctx, pi, membership)
	})
}

// Unit registers a unit-scoped operation (design D-4: the {unitId}
// route shape). It stamps the SAME KindCommunity marker Community does
// -- both ultimately resolve to a KindCommunity Membership, community
// membership just gets there via the unit's own community_id
// (authz.ResolveCommunityViaUnit) rather than a route-level community
// id, so A2's per-kind resolver/role check applies identically.
func Unit[I any, O any, PI interface {
	*I
	authz.UnitScoped
}](api huma.API, op huma.Operation, roles []authz.Role, handler func(context.Context, PI, authz.Membership) (*O, error)) {
	stampMarker(&op, authz.KindCommunity, roles)

	huma.Register(api, op, func(ctx context.Context, in *I) (*O, error) {
		pi := PI(in)
		userID, ok := authz.UserIDFromContext(ctx)
		if !ok {
			return nil, apperr.New(401, apperr.CodeUnauthorized, "missing authenticated caller", nil)
		}
		membership, err := authz.ResolveCommunityViaUnit(ctx, pi.ScopeUnitID(), userID)
		if err != nil {
			return nil, resolveErrorResponse(err)
		}
		if !authz.RoleAllowed(membership.Role(), roles) {
			return nil, forbidden()
		}
		return handler(ctx, pi, membership)
	})
}

// Invitation registers an invitation-scoped operation (design D-4: the
// {invitationId} route shape, used by resend/revoke). It stamps the
// SAME KindCommunity marker Community and Unit do -- all three
// ultimately resolve to a KindCommunity Membership, community
// membership just gets there via the invitation's own community_id
// (authz.ResolveCommunityViaInvitation) rather than a route-level
// community id, so A2's per-kind resolver/role check applies
// identically.
func Invitation[I any, O any, PI interface {
	*I
	authz.InvitationScoped
}](api huma.API, op huma.Operation, roles []authz.Role, handler func(context.Context, PI, authz.Membership) (*O, error)) {
	stampMarker(&op, authz.KindCommunity, roles)

	huma.Register(api, op, func(ctx context.Context, in *I) (*O, error) {
		pi := PI(in)
		userID, ok := authz.UserIDFromContext(ctx)
		if !ok {
			return nil, apperr.New(401, apperr.CodeUnauthorized, "missing authenticated caller", nil)
		}
		membership, err := authz.ResolveCommunityViaInvitation(ctx, pi.ScopeInvitationID(), userID)
		if err != nil {
			return nil, resolveErrorResponse(err)
		}
		if !authz.RoleAllowed(membership.Role(), roles) {
			return nil, forbidden()
		}
		return handler(ctx, pi, membership)
	})
}

// Self registers an operation scoped to the caller's own membership set
// and no path resource (design D-1): GET /v1/communities, GET /v1/me.
func Self[I any, O any](api huma.API, op huma.Operation, handler func(context.Context, *I, authz.Memberships) (*O, error)) {
	stampMarker(&op, authz.KindSelf, nil)

	huma.Register(api, op, func(ctx context.Context, in *I) (*O, error) {
		userID, ok := authz.UserIDFromContext(ctx)
		if !ok {
			return nil, apperr.New(401, apperr.CodeUnauthorized, "missing authenticated caller", nil)
		}
		memberships, err := authz.ResolveSelf(ctx, userID)
		if err != nil {
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		return handler(ctx, in, memberships)
	})
}

// resolveErrorResponse maps a resolver error to the D-4 policy: a
// foreign resource (no membership row) is 404 -- never confirming a
// resource's existence to a caller with no tie to it -- anything else
// is an internal error.
func resolveErrorResponse(err error) error {
	if errors.Is(err, authz.ErrNoMembership) {
		return apperr.New(404, apperr.CodeNotFound, "not found", nil)
	}
	if errors.Is(err, authz.ErrMFARequired) {
		// Distinguishable from the generic forbidden() (design D-7;
		// auth-mfa-totp delta: Mandatory TOTP For Admin And Admin_staff
		// Scope Access) -- the SAME code superadmin login already uses
		// for its own mandatory-TOTP branch.
		return apperr.New(403, apperr.CodeMFAEnrollmentRequired, "TOTP enrollment required for this role", nil)
	}
	return apperr.New(500, apperr.CodeInternal, "internal error", nil)
}

func forbidden() error {
	return apperr.New(403, apperr.CodeUnauthorized, "forbidden", nil)
}
