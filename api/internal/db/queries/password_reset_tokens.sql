-- name: InsertPasswordResetToken :one
INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, requested_ip)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetPasswordResetTokenByHash :one
SELECT * FROM password_reset_tokens WHERE token_hash = $1;

-- name: ConsumePasswordResetToken :exec
UPDATE password_reset_tokens SET used_at = now(), updated_at = now()
WHERE id = $1 AND used_at IS NULL;
