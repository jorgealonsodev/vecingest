-- +goose Up
-- Every object below must be CREATEd while the session's current_user is
-- vecingest_owner: the bootstrap set's fail-closed default-privilege
-- baseline (D-B) only grants SELECT, INSERT to app_rw automatically for
-- objects created by that exact role. A superuser can SET ROLE to any
-- role regardless of membership, which is exactly what the schema-set
-- connection (derived from BOOTSTRAP_DATABASE_URL by migrate.SchemaDSN)
-- relies on since vecingest_owner itself is NOLOGIN.
SET ROLE vecingest_owner;

CREATE TABLE users (
    id uuid PRIMARY KEY,
    email text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    name text NOT NULL,
    phone text,
    phone_verified_at timestamptz,
    locale text NOT NULL DEFAULT 'es',
    avatar_key text,
    id_document_encrypted bytea,
    notification_prefs jsonb NOT NULL DEFAULT '{}'::jsonb,
    is_superadmin boolean NOT NULL DEFAULT false,
    last_login_at timestamptz,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
-- Mutable table (D-B mechanism 1): the fail-closed baseline only grants
-- SELECT, INSERT by default; every mutable M0 table adds this explicitly.
GRANT UPDATE, DELETE ON users TO app_rw;

CREATE TABLE sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users (id),
    refresh_token_hash bytea NOT NULL UNIQUE,
    family_id uuid NOT NULL,
    device_name text,
    platform text NOT NULL CHECK (platform IN ('ios', 'android', 'web')),
    ip inet,
    last_used_at timestamptz,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
GRANT UPDATE, DELETE ON sessions TO app_rw;
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_family_id_idx ON sessions (family_id);

CREATE TABLE password_reset_tokens (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users (id),
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    requested_ip inet,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
GRANT UPDATE, DELETE ON password_reset_tokens TO app_rw;
CREATE INDEX password_reset_tokens_user_id_idx ON password_reset_tokens (user_id);

CREATE TABLE user_mfa (
    user_id uuid PRIMARY KEY REFERENCES users (id),
    totp_secret_encrypted bytea NOT NULL,
    enabled_at timestamptz,
    last_totp_step bigint,
    recovery_codes_hashed text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
GRANT UPDATE, DELETE ON user_mfa TO app_rw;

CREATE TABLE otp_challenges (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users (id),
    purpose text NOT NULL CHECK (purpose IN ('phone_verify', 'vote', 'sign_minutes', 'sensitive_action')),
    code_hash bytea NOT NULL,
    channel text NOT NULL CHECK (channel IN ('sms', 'totp')),
    expires_at timestamptz NOT NULL,
    attempts integer NOT NULL DEFAULT 0,
    verified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
GRANT UPDATE, DELETE ON otp_challenges TO app_rw;
CREATE INDEX otp_challenges_user_id_idx ON otp_challenges (user_id);

RESET ROLE;

-- +goose Down
DROP TABLE IF EXISTS otp_challenges;
DROP TABLE IF EXISTS user_mfa;
DROP TABLE IF EXISTS password_reset_tokens;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
