-- units: tenant column community_id (design D-5).
-- name: InsertUnit :one
INSERT INTO units (id, community_id, block, floor, door, type, participation_coefficient, cadastral_ref)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetUnitByID :one
SELECT * FROM units WHERE id = $1 AND deleted_at IS NULL;

-- Unit resolver (design D-4): a {unitId} route resolves community
-- membership via the unit's own community_id, never a caller-supplied
-- value.
-- name: GetUnitCommunityID :one
SELECT community_id FROM units WHERE id = $1 AND deleted_at IS NULL;

-- name: ListUnitsByCommunityID :many
SELECT * FROM units WHERE community_id = $1 AND deleted_at IS NULL ORDER BY block, floor, door;

-- CountUnitsByCommunityID backs community-management: Community Detail
-- Excludes Cross-Milestone Aggregates -- the community detail response's
-- unit count is a plain count of this tenant's own rows, never a
-- reserve-fund/quorum/balance aggregate from a later milestone.
-- name: CountUnitsByCommunityID :one
SELECT count(*) FROM units WHERE community_id = $1 AND deleted_at IS NULL;

-- SumParticipationCoefficientByCommunityID backs unit-management:
-- Participation Coefficient Sum Is A Warning, Not A Block (§5.2) -- the
-- sum is read-only, never a write-time constraint, so the caller
-- decides what to do with a sum outside 100 ± 0.01.
-- name: SumParticipationCoefficientByCommunityID :one
SELECT COALESCE(SUM(participation_coefficient), 0)::numeric AS sum
FROM units WHERE community_id = $1 AND deleted_at IS NULL;
