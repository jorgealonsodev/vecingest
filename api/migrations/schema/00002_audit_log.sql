-- +goose Up
SET ROLE vecingest_owner;

-- Partitioned by month (PRD §7.3, D-C): pg_partman is deferred because
-- postgres:17-alpine ships core+contrib only (D-C); the shape is honoured
-- natively with PARTITION BY RANGE and pre-created monthly partitions.
CREATE TABLE audit_log (
    id uuid NOT NULL,
    user_id uuid,
    community_id uuid,
    action text NOT NULL,
    entity text NOT NULL,
    entity_id uuid,
    before jsonb,
    after jsonb,
    ip inet,
    request_id uuid,
    prev_hash bytea,
    hash bytea,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (created_at, id)
) PARTITION BY RANGE (created_at);

-- Defence in depth (D-B): the fail-closed default-privilege baseline
-- already leaves this table with no UPDATE/DELETE/TRUNCATE grant, and
-- the bootstrap event trigger (vecingest_append_only_guard) revokes on
-- every relation in this table's partition tree automatically. This
-- explicit REVOKE on the parent is still stated, per the design's own
-- "every future migration that creates an append-only table still
-- carries its own explicit REVOKE" rule -- it costs nothing since there
-- is nothing to remove, and it documents intent for a human reader.
REVOKE UPDATE, DELETE, TRUNCATE ON audit_log FROM app_rw, PUBLIC;

-- Twelve monthly partitions, starting at the first day of the current
-- month, pre-created by this migration. The event trigger installed by
-- the bootstrap set fires on each CREATE TABLE ... PARTITION OF (tag
-- CREATE TABLE) and revokes on the resulting child too -- this loop
-- additionally issues the same explicit revoke per partition as defence
-- in depth, matching the parent above.
-- +goose StatementBegin
DO $$
DECLARE
    month_start date := date_trunc('month', now())::date;
    partition_start date;
    partition_end date;
    partition_name text;
    i integer;
BEGIN
    FOR i IN 0..11 LOOP
        partition_start := month_start + (i || ' months')::interval;
        partition_end := partition_start + interval '1 month';
        partition_name := 'audit_log_' || to_char(partition_start, 'YYYY_MM');

        EXECUTE format(
            'CREATE TABLE %I PARTITION OF audit_log FOR VALUES FROM (%L) TO (%L)',
            partition_name, partition_start, partition_end
        );
        EXECUTE format(
            'REVOKE UPDATE, DELETE, TRUNCATE ON %I FROM app_rw, PUBLIC',
            partition_name
        );
    END LOOP;
END;
$$;
-- +goose StatementEnd

RESET ROLE;

-- +goose Down
DROP TABLE IF EXISTS audit_log;
