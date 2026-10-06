-- name: InsertUser :one
INSERT INTO users (id, email, password_hash, name, phone, locale, is_superadmin)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1 AND deleted_at IS NULL;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: UpdateUserLastLogin :exec
UPDATE users SET last_login_at = $2, updated_at = now() WHERE id = $1;

-- name: InsertUserIgnoreConflict :execrows
-- platform-bootstrap: Idempotent Superadmin Bootstrap / seed Refuses To
-- Run Outside Non-Production -- both bootstrap-superadmin and seed need
-- an idempotent insert that never duplicates and never errors on a
-- second run. Zero affected rows means the email already existed; the
-- caller is expected to load the existing row (GetUserByEmail) and, for
-- bootstrap-superadmin only, ensure is_superadmin via EnsureSuperadmin.
INSERT INTO users (id, email, password_hash, name, phone, locale, is_superadmin)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (email) DO NOTHING;

-- name: EnsureSuperadmin :exec
-- platform-bootstrap: Idempotent Superadmin Bootstrap -- a second
-- bootstrap-superadmin run against an existing, non-superadmin user
-- (e.g. one first created by seed) promotes it, but NEVER touches its
-- password_hash.
UPDATE users SET is_superadmin = true, updated_at = now() WHERE id = $1 AND is_superadmin = false;

-- name: LockUserForMFAEnrollment :one
-- Row-locks the user at the start of every TOTP enrollment write
-- (enroll and its confirmation), so per-user enrollment state changes
-- one transaction at a time: the issuance cap is counted and enforced
-- without a race, and enroll and confirm take their later locks
-- (otp_challenges, then user_mfa) behind this one in the same order.
-- NO KEY UPDATE leaves foreign-key checks against users unblocked.
SELECT email FROM users WHERE id = $1 FOR NO KEY UPDATE;
