-- +goose Up
SET ROLE vecingest_owner;

-- `otp_challenges.channel` was created in 00001 as CHECK (channel IN ('sms',
-- 'totp')), which contradicts a decision taken later and recorded in
-- PRD_go.md: there is NO SMS provider. Line 423 states it ("Sin proveedor de
-- SMS"), and line 1644 records the reasoning and the accepted residual risk --
-- the OTP travels on the same channel as password recovery, so whoever
-- controls a mailbox can impersonate that owner's vote, which is why TOTP is
-- recommended to anyone voting remotely.
--
-- Left as it was, the first INSERT of an email OTP would fail the check
-- constraint at runtime, in M7, inside a vote. Nothing writes to this table
-- yet, so correcting it now costs nothing.
--
-- 'sms' is dropped rather than kept alongside 'email': an accepted value that
-- no code path can produce is an invitation to write code that produces it.
-- If SMS ever comes back (PRD 11 names the trigger: the first challenge to a
-- voter's identity, or a real compromised-mailbox case), it returns as its own
-- migration, together with the provider.
ALTER TABLE otp_challenges DROP CONSTRAINT otp_challenges_channel_check;
ALTER TABLE otp_challenges ADD CONSTRAINT otp_challenges_channel_check
    CHECK (channel IN ('email', 'totp'));

RESET ROLE;

-- +goose Down
SET ROLE vecingest_owner;

ALTER TABLE otp_challenges DROP CONSTRAINT otp_challenges_channel_check;
ALTER TABLE otp_challenges ADD CONSTRAINT otp_challenges_channel_check
    CHECK (channel IN ('sms', 'totp'));

RESET ROLE;
