package bootstrap

// guardFunctionSQL implements the append-only guard described in design
// D-B. It is SECURITY INVOKER (migrations run as vecingest_owner, which
// owns audit_log and may revoke on its own children; a superuser-run
// CREATE TABLE can revoke anything), so no SECURITY DEFINER routine is
// introduced here and invariant I2's surface does not grow.
//
// Reachability (upward, transitive) uses pg_partition_ancestors so a
// descendant reported at any depth of sub-partitioning still resolves to
// a registered root. Closure (downward, transitive) uses
// pg_partition_tree from the resolved root so every relation in the tree
// -- not only the reported one -- loses its write grant. Both walks are
// PostgreSQL 17 built-ins, not a hand-rolled pg_inherits recursion,
// matching I1's "walked recursively" requirement at every partitioning
// level.
//
// append_only_relations.relname is deliberately `text`, not `regclass`:
// the registry table is seeded (by this same bootstrap migration) with
// the literal name 'audit_log' before that table exists -- audit_log is
// created later by the schema migration set. to_regclass() resolves the
// name lazily, at guard-invocation time, and returns NULL (never
// matching) for a relation that does not exist yet.
const guardFunctionSQL = `
CREATE OR REPLACE FUNCTION vecingest_append_only_guard_fn()
RETURNS event_trigger
LANGUAGE plpgsql
SECURITY INVOKER
AS $fn$
DECLARE
  cmd RECORD;
  affected regclass;
  root regclass;
  ancestor regclass;
  child RECORD;
  in_scope boolean;
BEGIN
  FOR cmd IN SELECT * FROM pg_event_trigger_ddl_commands() WHERE object_type = 'table' LOOP
    affected := cmd.objid::regclass;

    -- Reachability: affected is in scope if it is itself registered, or
    -- any of its transitive ancestors is registered.
    in_scope := EXISTS (
      SELECT 1 FROM append_only_relations ar WHERE to_regclass(ar.relname) = affected
    );

    IF NOT in_scope THEN
      FOR ancestor IN SELECT * FROM pg_partition_ancestors(affected) LOOP
        IF EXISTS (SELECT 1 FROM append_only_relations ar WHERE to_regclass(ar.relname) = ancestor) THEN
          in_scope := true;
          EXIT;
        END IF;
      END LOOP;
    END IF;

    IF NOT in_scope THEN
      CONTINUE;
    END IF;

    -- Closure: revoke on every relation in the tree below the
    -- registered root, not only the reported relation.
    root := pg_partition_root(affected);
    FOR child IN SELECT relid FROM pg_partition_tree(root) LOOP
      IF has_table_privilege('app_rw', child.relid, 'UPDATE') THEN
        EXECUTE format('REVOKE UPDATE, DELETE, TRUNCATE ON %s FROM app_rw, PUBLIC', child.relid);
      END IF;
    END LOOP;
  END LOOP;
END;
$fn$;
`
