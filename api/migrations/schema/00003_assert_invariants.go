package schema

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

// assertInvariantsMigration registers the D-B invariant assertions (I1-I4)
// as this set's version-3 migration, run after the auth schema and
// audit_log exist and before River's schema is provisioned. Each
// invariant is a DO $$ ... RAISE EXCEPTION $$ block: a violation raises a
// real Postgres error, which fails this migration and therefore the
// whole deploy (db-access-control: SECURITY DEFINER EXECUTE Invariant).
func assertInvariantsMigration() *goose.Migration {
	return goose.NewGoMigration(3, &goose.GoFunc{RunDB: assertInvariantsUp}, &goose.GoFunc{RunDB: assertInvariantsDown})
}

// assertInvariantsUp runs each invariant check in its own statement so a
// failure names exactly which invariant broke.
func assertInvariantsUp(ctx context.Context, db *sql.DB) error {
	checks := []struct {
		name string
		sql  string
	}{
		{"I1", invariantI1SQL},
		{"I2", invariantI2SQL},
		{"I3", invariantI3SQL},
		{"I4", invariantI4SQL},
	}
	for _, c := range checks {
		if _, err := db.ExecContext(ctx, c.sql); err != nil {
			return fmt.Errorf("schema migration: invariant %s failed: %w", c.name, err)
		}
	}
	return nil
}

// assertInvariantsDown is a no-op: this migration asserts state, it does
// not create any. Nothing to reverse.
func assertInvariantsDown(_ context.Context, _ *sql.DB) error {
	return nil
}

// I1 -- app_rw holds no UPDATE/DELETE/TRUNCATE on audit_log or on any
// relation in its partition hierarchy, walked transitively with the
// PostgreSQL 17 built-in pg_partition_tree (which includes the root
// itself), matching D-B's bidirectional-closure requirement.
const invariantI1SQL = `
DO $$
DECLARE
  v_root regclass := 'audit_log'::regclass;
  v_rel regclass;
BEGIN
  FOR v_rel IN SELECT relid FROM pg_partition_tree(v_root) LOOP
    IF has_table_privilege('app_rw', v_rel, 'UPDATE')
       OR has_table_privilege('app_rw', v_rel, 'DELETE')
       OR has_table_privilege('app_rw', v_rel, 'TRUNCATE') THEN
      RAISE EXCEPTION 'invariant I1 violated: app_rw holds a write privilege on %', v_rel;
    END IF;
  END LOOP;
END;
$$;
`

// I2 -- no SECURITY DEFINER routine owned by vecingest_owner is
// EXECUTE-able by app_rw or PUBLIC. PUBLIC is checked via the
// has_function_privilege 'public' pseudo-role, per PostgreSQL's own
// access-privilege-inquiry documentation.
const invariantI2SQL = `
DO $$
DECLARE
  v_fn RECORD;
BEGIN
  FOR v_fn IN
    SELECT p.oid, p.proname
    FROM pg_proc p
    WHERE p.prosecdef
      AND p.proowner = 'vecingest_owner'::regrole
  LOOP
    IF has_function_privilege('app_rw', v_fn.oid, 'EXECUTE')
       OR has_function_privilege('public', v_fn.oid, 'EXECUTE') THEN
      RAISE EXCEPTION 'invariant I2 violated: SECURITY DEFINER function % is EXECUTE-able by app_rw or PUBLIC', v_fn.proname;
    END IF;
  END LOOP;
END;
$$;
`

// I3 -- app_rw owns no relation in audit_log's partition hierarchy and is
// not a member of vecingest_owner (re-asserted here, alongside the
// bootstrap-time check, because ownership is a superset of any grantable
// privilege).
const invariantI3SQL = `
DO $$
DECLARE
  v_root regclass := 'audit_log'::regclass;
  v_rel regclass;
  v_owner text;
BEGIN
  FOR v_rel IN SELECT relid FROM pg_partition_tree(v_root) LOOP
    SELECT pg_get_userbyid(c.relowner) INTO v_owner FROM pg_class c WHERE c.oid = v_rel;
    IF v_owner = 'app_rw' THEN
      RAISE EXCEPTION 'invariant I3 violated: app_rw owns append-only relation %', v_rel;
    END IF;
  END LOOP;

  IF EXISTS (
    SELECT 1 FROM pg_auth_members m
    JOIN pg_roles owner ON owner.oid = m.roleid AND owner.rolname = 'vecingest_owner'
    JOIN pg_roles member ON member.oid = m.member AND member.rolname = 'app_rw'
  ) THEN
    RAISE EXCEPTION 'invariant I3 violated: app_rw is a member of vecingest_owner';
  END IF;
END;
$$;
`

// I4 -- the default-privilege baseline for vecingest_owner's schema-less
// FOR ROLE ACL (defaclnamespace = 0, per D-B) grants nothing beyond
// SELECT, INSERT to app_rw, and to no other role.
const invariantI4SQL = `
DO $$
DECLARE
  v_bad_grantee boolean;
  v_bad_priv boolean;
BEGIN
  -- The object creator (vecingest_owner) always appears in its own
  -- default ACL with full rights -- that is Postgres's own ownership
  -- semantics surfacing through aclexplode, not an over-grant. Only a
  -- grantee that is neither vecingest_owner nor app_rw is a violation.
  SELECT EXISTS (
    SELECT 1
    FROM pg_default_acl da, aclexplode(da.defaclacl) ex
    WHERE da.defaclrole = 'vecingest_owner'::regrole
      AND da.defaclnamespace = 0
      AND da.defaclobjtype = 'r'
      AND ex.grantee NOT IN ('app_rw'::regrole, 'vecingest_owner'::regrole)
  ) INTO v_bad_grantee;

  IF v_bad_grantee THEN
    RAISE EXCEPTION 'invariant I4 violated: default table privilege baseline grants a role other than app_rw';
  END IF;

  SELECT EXISTS (
    SELECT 1
    FROM pg_default_acl da, aclexplode(da.defaclacl) ex
    WHERE da.defaclrole = 'vecingest_owner'::regrole
      AND da.defaclnamespace = 0
      AND da.defaclobjtype = 'r'
      AND ex.grantee = 'app_rw'::regrole
      AND ex.privilege_type NOT IN ('SELECT', 'INSERT')
  ) INTO v_bad_priv;

  IF v_bad_priv THEN
    RAISE EXCEPTION 'invariant I4 violated: default table privilege baseline for app_rw exceeds SELECT, INSERT';
  END IF;
END;
$$;
`
