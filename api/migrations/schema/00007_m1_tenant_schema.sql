-- +goose Up
-- Every object below must be CREATEd while the session's current_user is
-- vecingest_owner: the bootstrap set's fail-closed default-privilege
-- baseline (D-B) only grants SELECT, INSERT to app_rw automatically for
-- objects created by that exact role.
SET ROLE vecingest_owner;

-- offices -- despachos de administración de fincas (§7.3). id is itself
-- the tenant root; office_id elsewhere is an FK to it (design D-5).
CREATE TABLE offices (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    cif text NOT NULL,
    email text,
    phone text,
    address text,
    logo_key text,
    collegiate_number text,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (cif)
);
GRANT UPDATE, DELETE ON offices TO app_rw;

-- office_members -- tenant column: office_id (design D-5). The extra
-- office_members_user_id_idx is a documented exception to "composite
-- indexes start with the tenant column": GET /v1/me looks up by user,
-- not by office.
CREATE TABLE office_members (
    id uuid PRIMARY KEY,
    office_id uuid NOT NULL REFERENCES offices (id),
    user_id uuid NOT NULL REFERENCES users (id),
    role text NOT NULL CHECK (role IN ('admin', 'admin_staff')),
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (office_id, user_id)
);
GRANT UPDATE, DELETE ON office_members TO app_rw;
CREATE INDEX office_members_user_id_idx ON office_members (user_id);

-- communities -- tenant column: office_id (tenant owner); id is the
-- community tenant root (design D-5).
CREATE TABLE communities (
    id uuid PRIMARY KEY,
    office_id uuid NOT NULL REFERENCES offices (id),
    parent_community_id uuid REFERENCES communities (id),
    name text NOT NULL,
    cif text,
    address text,
    city text,
    province text,
    postal_code text,
    settings jsonb NOT NULL DEFAULT '{}'::jsonb,
    annual_budget numeric(12, 2),
    reserve_fund numeric(12, 2),
    secretary_is_office boolean NOT NULL DEFAULT false,
    last_ordinary_meeting_at timestamptz,
    dpa_signed_at timestamptz,
    transferred_from_office_id uuid REFERENCES offices (id),
    transferred_at timestamptz,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
GRANT UPDATE, DELETE ON communities TO app_rw;
CREATE INDEX communities_office_id_idx ON communities (office_id);

-- units -- viviendas / locales / garajes (§7.3). Tenant column:
-- community_id.
CREATE TABLE units (
    id uuid PRIMARY KEY,
    community_id uuid NOT NULL REFERENCES communities (id),
    block text,
    floor text,
    door text,
    type text NOT NULL CHECK (type IN ('flat', 'premises', 'garage', 'storage')),
    participation_coefficient numeric(6, 4),
    cadastral_ref text,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (community_id, block, floor, door)
);
GRANT UPDATE, DELETE ON units TO app_rw;
CREATE INDEX units_community_id_idx ON units (community_id);

-- unit_members -- tenant column: community_id, denormalised NOT NULL FK
-- (design D-5: §7.3 mandates a tenant column on every business table
-- even when derivable by a relation; deriving it through units on every
-- query is exactly what lint-scope cannot verify).
--
-- is_payer and iban_encrypted are DEFERRED to M5 (proposal Decision 2):
-- the versioned-key gate that makes an encrypted column safe does not
-- exist yet. board_role is created but never assigned in M1 (no
-- endpoint, no UI -- proposal Decision 2).
CREATE TABLE unit_members (
    id uuid PRIMARY KEY,
    unit_id uuid NOT NULL REFERENCES units (id),
    community_id uuid NOT NULL REFERENCES communities (id),
    user_id uuid NOT NULL REFERENCES users (id),
    role text NOT NULL CHECK (role IN ('owner', 'tenant')),
    tenure text NOT NULL DEFAULT 'full_owner' CHECK (tenure IN ('full_owner', 'bare_owner', 'usufructuary')),
    board_role text CHECK (board_role IN ('president', 'vice_president', 'secretary')),
    board_from timestamptz,
    board_to timestamptz,
    notification_address text,
    electronic_notifications_consent_at timestamptz,
    consent_text_version text,
    valid_from timestamptz,
    valid_to timestamptz,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (unit_id, user_id, role)
);
GRANT UPDATE, DELETE ON unit_members TO app_rw;
CREATE INDEX unit_members_user_id_idx ON unit_members (user_id);
-- At most one currently-serving president per community (design D-5).
CREATE UNIQUE INDEX unit_members_one_president_idx ON unit_members (community_id)
    WHERE board_role = 'president' AND board_to IS NULL AND deleted_at IS NULL;

RESET ROLE;

-- +goose Down
DROP TABLE IF EXISTS unit_members;
DROP TABLE IF EXISTS units;
DROP TABLE IF EXISTS communities;
DROP TABLE IF EXISTS office_members;
DROP TABLE IF EXISTS offices;
