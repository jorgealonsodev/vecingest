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

-- GET /v1/me's profile listing (user-profile: GET /v1/me Response
-- Shape): the caller's community memberships WITH the community name.
-- DISTINCT collapses several units held in one community under the same
-- role into the single entry the response describes; owner and tenant
-- in the same community stay two entries. Like
-- ListOfficeMembershipSummariesByUserID, a description of the caller's
-- own rows, never an authorization decision.
-- name: ListCommunityMembershipSummariesByUserID :many
SELECT DISTINCT um.community_id, c.name AS community_name, um.role
FROM unit_members um
JOIN communities c ON c.id = um.community_id AND c.deleted_at IS NULL
WHERE um.user_id = $1 AND um.deleted_at IS NULL
ORDER BY c.name ASC, um.community_id ASC, um.role ASC;

-- name: ListUnitMembersByCommunityID :many
SELECT * FROM unit_members WHERE community_id = $1 AND deleted_at IS NULL ORDER BY created_at ASC;

-- Incident creation requires current resident membership before either
-- common or unit scope is allowed. Return each active role separately so a
-- stale owner row cannot mask an active tenant when tenant creation is off.
-- name: GetActiveIncidentCommunityRoles :one
WITH active_memberships AS (
    SELECT um.role
    FROM unit_members um
    JOIN units u ON u.id = um.unit_id
        AND u.community_id = um.community_id
        AND u.deleted_at IS NULL
    WHERE um.community_id = sqlc.arg(community_id)
        AND um.user_id = sqlc.arg(user_id)
        AND um.role IN ('owner', 'tenant')
        AND um.deleted_at IS NULL
        AND (um.valid_from IS NULL OR um.valid_from <= now())
        AND (um.valid_to IS NULL OR um.valid_to > now())
)
SELECT EXISTS (SELECT 1 FROM active_memberships WHERE role = 'owner') AS has_active_owner,
    EXISTS (SELECT 1 FROM active_memberships WHERE role = 'tenant') AS has_active_tenant;

-- Incident creation must also prove membership in the exact requested unit;
-- community-level membership alone is not enough for unit scope.
-- name: HasActiveUnitMembership :one
SELECT EXISTS (
    SELECT 1
    FROM unit_members um
    JOIN units u ON u.id = um.unit_id
        AND u.community_id = um.community_id
        AND u.deleted_at IS NULL
    WHERE um.community_id = $1
        AND um.unit_id = $2
        AND um.user_id = $3
        AND um.deleted_at IS NULL
        AND (um.valid_from IS NULL OR um.valid_from <= now())
        AND (um.valid_to IS NULL OR um.valid_to > now())
);

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
