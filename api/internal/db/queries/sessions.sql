-- name: InsertSession :one
-- mfa_at is set at INSERT time and never updated afterwards: how a
-- session authenticated is decided once, when it is created (login) or
-- inherited from the row it rotates from (refresh). A session cannot be
-- promoted to second-factor authenticated later without a fresh login.
INSERT INTO sessions (id, user_id, refresh_token_hash, family_id, device_name, platform, ip, expires_at, mfa_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetSessionByRefreshTokenHash :one
SELECT * FROM sessions WHERE refresh_token_hash = $1;

-- name: ListLiveSessionsByFamilyID :many
SELECT * FROM sessions WHERE family_id = $1 AND revoked_at IS NULL ORDER BY created_at DESC;

-- name: RevokeSessionFamily :exec
UPDATE sessions SET revoked_at = now(), updated_at = now()
WHERE family_id = $1 AND revoked_at IS NULL;

-- name: RevokeSession :exec
UPDATE sessions SET revoked_at = now(), updated_at = now()
WHERE id = $1 AND revoked_at IS NULL;

-- name: TouchSessionLastUsed :exec
UPDATE sessions SET last_used_at = now(), updated_at = now() WHERE id = $1;
