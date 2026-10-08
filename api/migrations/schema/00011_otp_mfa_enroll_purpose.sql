-- +goose Up
SET ROLE vecingest_owner;

-- mfa_enroll is the purpose of the email code that confirms a TOTP
-- enrollment (auth-mfa-totp: Email-Confirmed Enrollment). Without it,
-- a password alone was enough to bind the attacker's own authenticator
-- to an account that had no factor yet, and with no disable or
-- recovery path that binding locked the legitimate owner out for good.
-- Proving control of the stored mailbox at enrollment time is the
-- cheapest check that closes that path.
--
-- otp_challenges already had every column this needs (code_hash,
-- channel 'email' since 00005, expires_at, attempts, verified_at); only
-- the purpose check was too narrow.
ALTER TABLE otp_challenges DROP CONSTRAINT otp_challenges_purpose_check;
ALTER TABLE otp_challenges ADD CONSTRAINT otp_challenges_purpose_check
    CHECK (purpose IN ('vote', 'sign_minutes', 'sensitive_action', 'mfa_enroll'));

RESET ROLE;

-- +goose Down
SET ROLE vecingest_owner;

-- Rows carrying the purpose being removed would fail the restored
-- check; they are short-lived enrollment codes with no value once the
-- feature that reads them is gone.
DELETE FROM otp_challenges WHERE purpose = 'mfa_enroll';

ALTER TABLE otp_challenges DROP CONSTRAINT otp_challenges_purpose_check;
ALTER TABLE otp_challenges ADD CONSTRAINT otp_challenges_purpose_check
    CHECK (purpose IN ('vote', 'sign_minutes', 'sensitive_action'));

RESET ROLE;
