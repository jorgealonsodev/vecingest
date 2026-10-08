-- invitations: tenant column community_id (design D-5).
-- name: InsertInvitation :one
INSERT INTO invitations (id, community_id, unit_id, email, role, token_hash, short_code_hash, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetInvitationByID :one
SELECT * FROM invitations WHERE id = $1;

-- Invitation resolver (design D-4): an {invitationId} route resolves
-- community membership via the invitation's own owning community_id.
-- name: GetInvitationCommunityID :one
SELECT community_id FROM invitations WHERE id = $1;

-- name: ListInvitationsByCommunityID :many
SELECT * FROM invitations WHERE community_id = $1 ORDER BY created_at DESC;
