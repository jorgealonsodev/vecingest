-- unit_members: tenant column community_id, denormalised NOT NULL FK
-- (design D-5).
-- name: InsertUnitMember :one
INSERT INTO unit_members (id, unit_id, community_id, user_id, role, tenure, notification_address)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- Community resolver, unit leg (design D-4): a caller's unit_members
-- row for this community_id, never derived from a header.
-- name: ResolveCommunityRoleViaUnit :one
SELECT role FROM unit_members WHERE community_id = $1 AND user_id = $2 AND deleted_at IS NULL LIMIT 1;

-- Self resolver (design D-4): the caller's full unit-membership set,
-- for GET /v1/me.
-- name: ListUnitMembershipsByUserID :many
SELECT community_id, unit_id, role FROM unit_members WHERE user_id = $1 AND deleted_at IS NULL;

-- name: ListUnitMembersByCommunityID :many
SELECT * FROM unit_members WHERE community_id = $1 AND deleted_at IS NULL ORDER BY created_at ASC;
