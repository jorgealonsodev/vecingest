-- +goose Up
SET ROLE vecingest_owner;

-- Phone verification is dropped. Verifying that a number belongs to someone
-- requires sending a code to that number, and PRD_go.md line 423 records that
-- there is no SMS provider. Verifying a phone by email would not prove
-- anything about the phone, so the honest options were to drop it or to wait
-- for an SMS provider that is not coming in phase 1.
--
-- `users.phone` STAYS: a phone number is still useful contact data for an
-- administrator calling a neighbour. What goes is the claim that the platform
-- ever checked it.
--
-- Same reasoning as 00005 dropping 'sms' from the channel check: a state the
-- product cannot reach should not be representable, because a column named
-- `phone_verified_at` invites code that sets it.
ALTER TABLE users DROP COLUMN phone_verified_at;

ALTER TABLE otp_challenges DROP CONSTRAINT otp_challenges_purpose_check;
ALTER TABLE otp_challenges ADD CONSTRAINT otp_challenges_purpose_check
    CHECK (purpose IN ('vote', 'sign_minutes', 'sensitive_action'));

RESET ROLE;

-- +goose Down
SET ROLE vecingest_owner;

ALTER TABLE otp_challenges DROP CONSTRAINT otp_challenges_purpose_check;
ALTER TABLE otp_challenges ADD CONSTRAINT otp_challenges_purpose_check
    CHECK (purpose IN ('phone_verify', 'vote', 'sign_minutes', 'sensitive_action'));

-- The column comes back empty: the timestamps it held, if any, are gone. It
-- never held any, because nothing ever wrote to it.
ALTER TABLE users ADD COLUMN phone_verified_at timestamptz;

RESET ROLE;
