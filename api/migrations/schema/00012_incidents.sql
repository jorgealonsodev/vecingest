-- +goose Up
SET ROLE vecingest_owner;

-- Incidents carry the community tenant key explicitly. The composite
-- unit reference prevents a row from pairing one community with another
-- community's unit, even when both foreign keys point at valid rows.
ALTER TABLE units
    ADD CONSTRAINT units_community_id_id_key UNIQUE (community_id, id);

CREATE TABLE incidents (
    id uuid PRIMARY KEY,
    community_id uuid NOT NULL REFERENCES communities (id),
    unit_id uuid,
    created_by uuid NOT NULL REFERENCES users (id),
    title text NOT NULL,
    description text NOT NULL,
    category text NOT NULL CHECK (category IN (
        'elevator', 'plumbing', 'electricity', 'cleaning', 'locksmith',
        'gardening', 'works', 'mandatory_works', 'noise_and_coexistence', 'other'
    )),
    priority text NOT NULL DEFAULT 'normal' CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
    status text NOT NULL DEFAULT 'open' CHECK (status IN (
        'open', 'assigned', 'in_progress', 'resolved', 'closed', 'rejected'
    )),
    scope text NOT NULL CHECK (scope IN ('common', 'unit')),
    location_text text,
    affected_count integer NOT NULL DEFAULT 1 CHECK (affected_count >= 1),
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT incidents_community_unit_fk
        FOREIGN KEY (community_id, unit_id) REFERENCES units (community_id, id),
    CONSTRAINT incidents_scope_unit_check CHECK (
        (scope = 'common' AND unit_id IS NULL) OR
        (scope = 'unit' AND unit_id IS NOT NULL)
    )
);
GRANT UPDATE, DELETE ON incidents TO app_rw;

-- Community-leading indexes support tenant/status filtering, stable
-- keyset ordering and the two optional list filters.
CREATE INDEX incidents_community_id_status_idx
    ON incidents (community_id, status);
CREATE INDEX incidents_community_id_created_at_id_idx
    ON incidents (community_id, created_at DESC, id DESC);
CREATE INDEX incidents_community_id_category_created_at_id_idx
    ON incidents (community_id, category, created_at DESC, id DESC);
CREATE INDEX incidents_community_id_unit_id_created_at_id_idx
    ON incidents (community_id, unit_id, created_at DESC, id DESC);

RESET ROLE;

-- +goose Down
DROP TABLE IF EXISTS incidents;
ALTER TABLE units DROP CONSTRAINT IF EXISTS units_community_id_id_key;
