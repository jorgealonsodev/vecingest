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
