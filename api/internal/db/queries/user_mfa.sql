-- name: UpsertUserMFA :one
INSERT INTO user_mfa (user_id, totp_secret_encrypted)
VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE SET totp_secret_encrypted = EXCLUDED.totp_secret_encrypted, updated_at = now()
RETURNING *;

-- name: GetUserMFA :one
SELECT * FROM user_mfa WHERE user_id = $1;

-- name: ConfirmUserMFAEnrollment :exec
UPDATE user_mfa SET enabled_at = now(), updated_at = now() WHERE user_id = $1;

-- name: UpdateUserMFALastTOTPStep :execrows
UPDATE user_mfa SET last_totp_step = $2, updated_at = now()
WHERE user_id = $1 AND (last_totp_step IS NULL OR last_totp_step < $2);

-- name: SetUserMFARecoveryCodes :exec
UPDATE user_mfa SET recovery_codes_hashed = $2, updated_at = now() WHERE user_id = $1;

-- name: ConsumeUserMFARecoveryCode :execrows
UPDATE user_mfa SET recovery_codes_hashed = array_remove(recovery_codes_hashed, $2), updated_at = now()
WHERE user_id = $1 AND $2 = ANY(recovery_codes_hashed);

-- name: IsUserMFAEnabled :one
-- auth-mfa-totp delta: Mandatory TOTP For Admin And Admin_staff Scope
-- Access. Always returns exactly one row (true/false), never
-- pgx.ErrNoRows, so a caller who never enrolled resolves cleanly to
-- false instead of a "no rows" error the resolver would have to special
-- -case.
SELECT EXISTS (
  SELECT 1 FROM user_mfa WHERE user_id = $1 AND enabled_at IS NOT NULL
);
