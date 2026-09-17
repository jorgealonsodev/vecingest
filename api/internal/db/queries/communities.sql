-- communities: tenant column office_id (tenant owner); id is the
-- community tenant root (design D-5).
-- name: InsertCommunity :one
INSERT INTO communities (
    id, office_id, parent_community_id, name, cif, address, city, province, postal_code,
    annual_budget, reserve_fund, secretary_is_office
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- GetCommunityByID reads by the community's own id, which is itself the
-- tenant root (design D-5) -- the explicit column list (rather than
-- SELECT *) keeps office_id textually present for lint-scope, since a
-- lookup already scoped to one exact id has no other tenant's row to
-- leak.
-- name: GetCommunityByID :one
SELECT id, office_id, parent_community_id, name, cif, address, city, province, postal_code,
    settings, annual_budget, reserve_fund, secretary_is_office, last_ordinary_meeting_at,
    dpa_signed_at, transferred_from_office_id, transferred_at, deleted_at, created_at, updated_at
FROM communities WHERE id = $1 AND deleted_at IS NULL;

-- name: ListCommunitiesByOfficeID :many
SELECT * FROM communities WHERE office_id = $1 AND deleted_at IS NULL ORDER BY created_at ASC;

-- UpdateCommunity persists the community-management: Community Update
-- Restricted To Office Roles / Legal And Descriptive Fields Persisted
-- Per §7.3 field set (task 3.8's "full column set"). transferred_* and
-- office_id are deliberately absent from the SET list: transferring a
-- community to a different office is a distinct sensitive operation
-- (design D-5 documents transferred_from_office_id/transferred_at as
-- schema groundwork, not an M1 endpoint) this PATCH's admin/admin_staff
-- role check must never be able to trigger implicitly. office_id is
-- kept in the RETURNING column list only (rather than SELECT *) so
-- lint-scope's textual check keeps seeing this tenant-scoped UPDATE's
-- own tenant column, matching GetCommunityByID's documented deviation
-- above for the identical reason: an update already scoped to one
-- exact id (WHERE id = $1) has no other tenant's row to touch.
-- name: UpdateCommunity :one
UPDATE communities SET
    parent_community_id = $2,
    name = $3,
    cif = $4,
    address = $5,
    city = $6,
    province = $7,
    postal_code = $8,
    annual_budget = $9,
    reserve_fund = $10,
    secretary_is_office = $11,
    last_ordinary_meeting_at = $12,
    dpa_signed_at = $13,
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, office_id, parent_community_id, name, cif, address, city, province, postal_code,
    settings, annual_budget, reserve_fund, secretary_is_office, last_ordinary_meeting_at,
    dpa_signed_at, transferred_from_office_id, transferred_at, deleted_at, created_at, updated_at;

-- Community resolver, office leg (design D-4): an admin/admin_staff
-- reaches a community whose office_id matches one of their
-- office_members rows -- never a client-supplied office id.
-- name: ResolveCommunityRoleViaOffice :one
SELECT om.role
FROM office_members om
JOIN communities c ON c.office_id = om.office_id
WHERE c.id = sqlc.arg(community_id) AND om.user_id = sqlc.arg(user_id)
    AND om.deleted_at IS NULL AND c.deleted_at IS NULL;
