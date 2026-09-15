-- +goose Up
SET ROLE vecingest_owner;

-- invitations -- tenant column: community_id (design D-5/D-6). status is
-- an explicit column, maintained transactionally, rather than derived
-- purely from timestamps: it is the only way to represent `blocked`,
-- which no timestamp derives (recorded proposal Decision 3 / design D-6
-- §7.3 deviation). expires_at stays authoritative for expiry: `expired`
-- is derived at read time by the invitations.expire sweep job (Phase 6),
-- never stored.
--
-- token_hash and short_code_hash are both generated together at
-- creation (design D-6: "Tokens: crypto/rand, SHA-256 token_hash and
-- short_code_hash (both unique)") and are therefore NOT NULL, unlike
-- the nullable short_code_hash §7.3 originally sketched -- the created
-- flow never produces one without the other.
CREATE TABLE invitations (
    id uuid PRIMARY KEY,
    community_id uuid NOT NULL REFERENCES communities (id),
    unit_id uuid REFERENCES units (id),
    email text,
    role text NOT NULL CHECK (role IN ('owner', 'tenant')),
    token_hash bytea NOT NULL UNIQUE,
    short_code_hash bytea NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'revoked', 'blocked')),
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    sent_count integer NOT NULL DEFAULT 1,
    failed_attempts integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
GRANT UPDATE, DELETE ON invitations TO app_rw;
CREATE INDEX invitations_community_id_status_idx ON invitations (community_id, status);

RESET ROLE;

-- +goose Down
DROP TABLE IF EXISTS invitations;
