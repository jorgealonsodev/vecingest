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

-- name: InvalidateOpenOTPChallenges :exec
-- Closes every still-open challenge for user_id+purpose by expiring it
-- at `now` (the caller's clock). Issuing a new code calls this first, so a
-- code from a superseded enrollment can never confirm the new one.
UPDATE otp_challenges SET expires_at = sqlc.arg(now), updated_at = now()
WHERE user_id = sqlc.arg(user_id) AND purpose = sqlc.arg(purpose)
  AND verified_at IS NULL AND expires_at > sqlc.arg(now);

-- name: ConsumeOTPChallengeAttempt :one
-- Spends one attempt on the latest open challenge for user_id+purpose
-- and returns it, or no row when there is none, it has expired, or its
-- attempts are exhausted. It is a single autocommitted statement on
-- purpose: the increment must survive the rejection that follows it, and
-- Postgres re-evaluates `attempts < max` against the newest row version
-- when two requests race for the same challenge.
UPDATE otp_challenges o SET attempts = o.attempts + 1, updated_at = now()
WHERE o.id = (
    SELECT c.id FROM otp_challenges c
    WHERE c.user_id = sqlc.arg(user_id) AND c.purpose = sqlc.arg(purpose)
      AND c.verified_at IS NULL
    ORDER BY c.created_at DESC
    LIMIT 1
)
  AND o.verified_at IS NULL
  AND o.expires_at > sqlc.arg(now)
  AND o.attempts < sqlc.arg(max_attempts)::integer
RETURNING *;

-- name: LockOpenOTPChallenge :one
-- Re-reads one challenge FOR UPDATE inside the transaction that will
-- mark it verified, so two concurrent correct submissions cannot both
-- consume it: the second waits on the lock and then finds verified_at set.
SELECT * FROM otp_challenges
WHERE id = $1 AND verified_at IS NULL
FOR UPDATE;

-- name: CountOTPChallengesIssuedSince :one
-- How many challenges of purpose were issued to user_id within the last
-- window_seconds. created_at is the database's own clock, so the window
-- is measured against now() rather than the caller's clock. It caps
-- issuance, not use: superseded and expired challenges count too.
--
-- window_seconds is a whole number of seconds, multiplied into an
-- interval rather than passed to make_interval(secs => ...), whose secs
-- argument only accepts double precision. That cast made sqlc emit a
-- binary floating-point parameter under internal/db, which api/.semgrep's
-- no-float-money-go guard forbids there. Every caller's window is a whole
-- number of seconds, so integer arithmetic is exact and the guard stays
-- strict instead of being suppressed.
SELECT count(*) FROM otp_challenges
WHERE user_id = sqlc.arg(user_id) AND purpose = sqlc.arg(purpose)
  AND created_at > now() - (sqlc.arg(window_seconds)::int * interval '1 second');
