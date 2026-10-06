-- incidents: tenant column community_id (design D-5).
-- name: InsertIncident :one
INSERT INTO incidents (
    id, community_id, unit_id, created_by, title, description, category, scope, location_text
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- A private unit incident is visible only to its creator, an active
-- member of that exact unit, or an admin/admin_staff of the community's
-- owning office. Board roles are intentionally not an authorization
-- predicate: being president alone does not grant private-unit access.
-- Common incidents require community membership unless the caller is a
-- scoped office administrator. Caller and community ids are explicit
-- bound inputs, never inferred from an incident id alone.
-- name: ListVisibleIncidents :many
SELECT i.*
FROM incidents i
JOIN communities c ON c.id = i.community_id AND c.deleted_at IS NULL
WHERE i.community_id = sqlc.arg(community_id)
    AND i.deleted_at IS NULL
    AND (
        EXISTS (
            SELECT 1 FROM office_members om
            WHERE om.office_id = c.office_id
                AND om.user_id = sqlc.arg(user_id)
                AND om.role IN ('admin', 'admin_staff')
                AND om.deleted_at IS NULL
        )
        OR (
            i.scope = 'common'
            AND EXISTS (
                SELECT 1
                FROM unit_members um
                JOIN units u ON u.id = um.unit_id
                    AND u.community_id = um.community_id
                    AND u.deleted_at IS NULL
                WHERE um.community_id = i.community_id
                    AND um.user_id = sqlc.arg(user_id)
                    AND um.deleted_at IS NULL
                    AND (um.valid_from IS NULL OR um.valid_from <= now())
                    AND (um.valid_to IS NULL OR um.valid_to > now())
            )
        )
        OR (
            i.scope = 'unit'
            AND (
                i.created_by = sqlc.arg(user_id)
                OR EXISTS (
                    SELECT 1
                    FROM unit_members um
                    JOIN units u ON u.id = um.unit_id
                        AND u.community_id = um.community_id
                        AND u.deleted_at IS NULL
                    WHERE um.community_id = i.community_id
                        AND um.unit_id = i.unit_id
                        AND um.user_id = sqlc.arg(user_id)
                        AND um.deleted_at IS NULL
                        AND (um.valid_from IS NULL OR um.valid_from <= now())
                        AND (um.valid_to IS NULL OR um.valid_to > now())
                )
            )
        )
    )
    AND (sqlc.narg('status')::text IS NULL OR i.status = sqlc.narg('status')::text)
    AND (sqlc.narg('category')::text IS NULL OR i.category = sqlc.narg('category')::text)
    AND (sqlc.narg('unit_id')::uuid IS NULL OR i.unit_id = sqlc.narg('unit_id')::uuid)
    AND (
        sqlc.narg('cursor_created_at')::timestamptz IS NULL
        OR (i.created_at, i.id) < (
            sqlc.narg('cursor_created_at')::timestamptz,
            sqlc.narg('cursor_id')::uuid
        )
    )
ORDER BY i.created_at DESC, i.id DESC
LIMIT sqlc.arg(page_size)::int;

-- Detail lookup repeats the complete visibility predicate and binds both
-- community and caller, so foreign/private ids resolve as no row.
-- name: GetVisibleIncidentByID :one
SELECT i.*
FROM incidents i
JOIN communities c ON c.id = i.community_id AND c.deleted_at IS NULL
WHERE i.id = sqlc.arg(id)
    AND i.community_id = sqlc.arg(community_id)
    AND i.deleted_at IS NULL
    AND (
        EXISTS (
            SELECT 1 FROM office_members om
            WHERE om.office_id = c.office_id
                AND om.user_id = sqlc.arg(user_id)
                AND om.role IN ('admin', 'admin_staff')
                AND om.deleted_at IS NULL
        )
        OR (
            i.scope = 'common'
            AND EXISTS (
                SELECT 1
                FROM unit_members um
                JOIN units u ON u.id = um.unit_id
                    AND u.community_id = um.community_id
                    AND u.deleted_at IS NULL
                WHERE um.community_id = i.community_id
                    AND um.user_id = sqlc.arg(user_id)
                    AND um.deleted_at IS NULL
                    AND (um.valid_from IS NULL OR um.valid_from <= now())
                    AND (um.valid_to IS NULL OR um.valid_to > now())
            )
        )
        OR (
            i.scope = 'unit'
            AND (
                i.created_by = sqlc.arg(user_id)
                OR EXISTS (
                    SELECT 1
                    FROM unit_members um
                    JOIN units u ON u.id = um.unit_id
                        AND u.community_id = um.community_id
                        AND u.deleted_at IS NULL
                    WHERE um.community_id = i.community_id
                        AND um.unit_id = i.unit_id
                        AND um.user_id = sqlc.arg(user_id)
                        AND um.deleted_at IS NULL
                        AND (um.valid_from IS NULL OR um.valid_from <= now())
                        AND (um.valid_to IS NULL OR um.valid_to > now())
                )
            )
        )
    );
