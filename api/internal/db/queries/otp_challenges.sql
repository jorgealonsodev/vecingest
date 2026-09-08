-- name: InsertOTPChallenge :one
INSERT INTO otp_challenges (id, user_id, purpose, code_hash, channel, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetOTPChallenge :one
SELECT * FROM otp_challenges WHERE id = $1;

-- name: IncrementOTPChallengeAttempts :one
UPDATE otp_challenges SET attempts = attempts + 1, updated_at = now()
WHERE id = $1
RETURNING attempts;

-- name: MarkOTPChallengeVerified :exec
UPDATE otp_challenges SET verified_at = now(), updated_at = now() WHERE id = $1;
