-- office_members: tenant column office_id (design D-5).
-- name: InsertOfficeMember :one
INSERT INTO office_members (id, office_id, user_id, role)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- office resolver (design D-4): office scope resolves by (office_id,
-- user_id), never from a header or body.
-- name: GetOfficeMemberByOfficeAndUser :one
SELECT * FROM office_members WHERE office_id = $1 AND user_id = $2 AND deleted_at IS NULL;

-- Self resolver (design D-4): the caller's full office-membership set,
-- for GET /v1/me. Looked up by user, not by office -- the documented
-- exception D-5 names for office_members_user_id_idx.
-- name: ListOfficeMembershipsByUserID :many
SELECT office_id, role FROM office_members WHERE user_id = $1 AND deleted_at IS NULL;

-- name: ListOfficeMembers :many
SELECT * FROM office_members WHERE office_id = $1 AND deleted_at IS NULL ORDER BY created_at ASC;

-- CountOfficeMembersByOfficeID backs community-management: Community
-- Detail Excludes Cross-Milestone Aggregates -- the community detail
-- response's office-member count is a plain count of the owning
-- office's own rows, never a reserve-fund/quorum/balance aggregate.
-- name: CountOfficeMembersByOfficeID :one
SELECT count(*) FROM office_members WHERE office_id = $1 AND deleted_at IS NULL;
