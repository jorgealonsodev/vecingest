-- +goose Up
SET ROLE vecingest_owner;

-- mfa_at records WHEN, and therefore WHETHER, this session was
-- second-factor authenticated (auth-mfa-totp: Mandatory TOTP For Admin
-- And Admin_staff Scope Access; review lineage review-0e1833930adf141a,
-- R1-mandatory-totp-gate-is-only-an-enrollment-flag).
--
-- The gate it backs used to read user_mfa.enabled_at, a DURABLE
-- PER-ACCOUNT flag, which cannot express a per-session fact and left two
-- bypasses open: a stolen password alone produced a session that
-- satisfied the gate whenever the victim had ever enrolled, and where
-- the victim had not, that same password-only session could enroll a
-- fresh factor through /v1/me/mfa/enroll + /verify and pass. Carrying
-- the fact on the session closes both at once: an attacker may still
-- enroll, but the session they already hold keeps mfa_at NULL, so it
-- buys them nothing, while the legitimate admin keeps the bootstrap
-- path they need in order to enroll at all.
--
-- NULLABLE is the load-bearing property, not a default-avoidance
-- preference. This stack is already deployed (docs/pendientes-
-- despliegue.md): every session row that exists when this migration
-- runs was issued with no TOTP challenge, and NULL is exactly what such
-- a session is -- not second-factor authenticated. A DEFAULT now()
-- would silently grant every live session the elevation this column
-- exists to withhold.
--
-- No grant is needed: sessions already carries GRANT UPDATE, DELETE ON
-- sessions TO app_rw from 00001, and column privileges follow the
-- table's.
ALTER TABLE sessions ADD COLUMN mfa_at timestamptz;

RESET ROLE;

-- +goose Down
SET ROLE vecingest_owner;

ALTER TABLE sessions DROP COLUMN IF EXISTS mfa_at;

RESET ROLE;
