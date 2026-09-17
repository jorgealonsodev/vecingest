-- unit_members: tenant column community_id, denormalised NOT NULL FK
-- (design D-5). electronic_notifications_consent_at/consent_text_version
-- are settable at creation (unit-management: Consent And Notification
-- Fields Captured Per Member, "created without consent" scenario --
-- null when the caller passes no consent).
-- name: InsertUnitMember :one
INSERT INTO unit_members (
    id, unit_id, community_id, user_id, role, tenure, notification_address,
    electronic_notifications_consent_at, consent_text_version
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
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

-- name: ListUnitMembersByUnitID :many
SELECT * FROM unit_members WHERE unit_id = $1 AND community_id = $2 AND deleted_at IS NULL ORDER BY created_at ASC;

-- unit-management: Unit Member Management Scoped To Community --
-- unit_id AND community_id are both bound parameters (never just the
-- path-supplied memberId alone), so a memberId belonging to a
-- different unit or a different community can never be read, updated
-- or deleted through this unit's route.
-- name: GetUnitMemberByID :one
SELECT * FROM unit_members WHERE id = $1 AND unit_id = $2 AND community_id = $3 AND deleted_at IS NULL;

-- name: UpdateUnitMember :one
UPDATE unit_members SET
    role = $4,
    tenure = $5,
    notification_address = $6,
    electronic_notifications_consent_at = $7,
    consent_text_version = $8,
    updated_at = now()
WHERE id = $1 AND unit_id = $2 AND community_id = $3 AND deleted_at IS NULL
RETURNING *;

-- name: DeleteUnitMember :execrows
UPDATE unit_members SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND unit_id = $2 AND community_id = $3 AND deleted_at IS NULL;
