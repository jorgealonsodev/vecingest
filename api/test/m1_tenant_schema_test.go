package test

import (
	"context"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/platform/migrate"
)

// TestM1TenantSchema is the Phase 1 (WU-1) migration integration suite
// for 00007_m1_tenant_schema.sql (task 1.2) and
// 00008_invitations.sql (task 1.4).
func TestM1TenantSchema(t *testing.T) {
	t.Run("UpDownUpIdempotent", testM1TenantSchemaUpDownUpIdempotent)
	t.Run("InvitationsTable", testM1InvitationsTable)
}

// m1TenantTables is every table 00007_m1_tenant_schema.sql creates
// (design D-5): offices, office_members, communities, units,
// unit_members.
var m1TenantTables = []string{"offices", "office_members", "communities", "units", "unit_members"}

// authz-membership / m1-communities design D-5: 00007_m1_tenant_schema.sql
// creates every M1 tenant table, each mutable (explicit GRANT UPDATE,
// DELETE -- the fail-closed baseline only grants SELECT, INSERT by
// default), with the indexes/uniques D-5's table enumerates, and the
// whole set survives DownTo/Up with no residue (task 1.2).
func testM1TenantSchemaUpDownUpIdempotent(t *testing.T) {
	db, _ := startPostgres(t)
	migrateFully(t, db)

	for _, table := range m1TenantTables {
		assertTableExists(t, db, table)
		assertAppRWCanMutate(t, db, table)
	}

	// D-5's named indexes: office_members_user_id_idx (documented
	// exception -- GET /v1/me looks up by user, not by office),
	// communities_office_id_idx, units_community_id_idx,
	// unit_members_user_id_idx, and the partial unique
	// unit_members_one_president_idx.
	for _, idx := range []string{
		"office_members_user_id_idx",
		"communities_office_id_idx",
		"units_community_id_idx",
		"unit_members_user_id_idx",
		"unit_members_one_president_idx",
	} {
		assertIndexExists(t, db, idx)
	}

	// Uniques declared inline via UNIQUE(...) table constraints, which
	// Postgres auto-names "<table>_<cols>_key".
	for _, uniq := range []string{
		"offices_cif_key",
		"office_members_office_id_user_id_key",
		"units_community_id_block_floor_door_key",
		"unit_members_unit_id_user_id_role_key",
	} {
		assertIndexExists(t, db, uniq)
	}

	provider, err := migrate.SchemaProvider(db)
	if err != nil {
		t.Fatalf("SchemaProvider failed: %v", err)
	}
	ctx := context.Background()

	// DownTo(6) undoes every M1 migration (00007, and 00008 once it
	// exists) in reverse order, leaving the M0 baseline.
	if _, err := provider.DownTo(ctx, 6); err != nil {
		t.Fatalf("DownTo(6) failed: %v", err)
	}
	for _, table := range m1TenantTables {
		assertTableAbsent(t, db, table)
	}

	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("second schema Up failed: %v", err)
	}
	for _, table := range m1TenantTables {
		assertTableExists(t, db, table)
	}
}

// authz-membership / invitations design D-5/D-6: 00008_invitations.sql
// creates invitations with the explicit status CHECK, unique
// token_hash/short_code_hash, invitations_community_id_status_idx and
// explicit grants (task 1.4).
func testM1InvitationsTable(t *testing.T) {
	db, _ := startPostgres(t)
	migrateFully(t, db)

	assertTableExists(t, db, "invitations")
	assertAppRWCanMutate(t, db, "invitations")
	assertIndexExists(t, db, "invitations_community_id_status_idx")
	assertIndexExists(t, db, "invitations_token_hash_key")
	assertIndexExists(t, db, "invitations_short_code_hash_key")

	var statusCheckExists bool
	if err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conrelid = 'invitations'::regclass AND contype = 'c' AND pg_get_constraintdef(oid) LIKE '%status%'
		)
	`).Scan(&statusCheckExists); err != nil {
		t.Fatalf("query status CHECK constraint: %v", err)
	}
	if !statusCheckExists {
		t.Fatalf("expected invitations.status to carry a CHECK constraint")
	}
}
