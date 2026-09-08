package test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jorgealonsodev/vecingest/internal/platform/migrate"
	"github.com/jorgealonsodev/vecingest/migrations/schema"
)

// TestPrivilegeMatrix is the Phase 3 (WU-3) integration suite: the full
// bootstrap+schema migration pipeline against a real PostgreSQL 17
// container, asserting the append-only privilege matrix db-access-control
// requires. Every subtest carries its own container (via startPostgres's
// testing.Short() skip guard) so each scenario starts from a clean,
// independently-verifiable state.
func TestPrivilegeMatrix(t *testing.T) {
	t.Run("MigrationsUpDownUpIdempotent", testMigrationsUpDownUpIdempotent)
	t.Run("AuditLogPartitionedMonthlyAndPgPartmanAbsent", testAuditLogPartitionedMonthlyAndPgPartmanAbsent)
	t.Run("AppRWCannotWriteAuditLogParent", testAppRWCannotWriteAuditLogParent)
	t.Run("AppRWCannotWriteAuditLogPartition", testAppRWCannotWriteAuditLogPartition)
	t.Run("PartitionOfGuardCoversNewChildWithNoExplicitRevoke", testPartitionOfGuardCoversNewChild)
	t.Run("AttachPartitionGuardCoversNewChildWithNoExplicitRevoke", testAttachPartitionGuardCoversNewChild)
	t.Run("RiverGrantsPresentAndSchemaAtPinnedVersion", testRiverGrantsPresentAndSchemaAtPinnedVersion)
}

// platform-bootstrap / db-access-control: the schema set must be fully
// reversible and re-appliable with no residue -- the greenfield rollback
// boundary the design's own rollback section documents.
func testMigrationsUpDownUpIdempotent(t *testing.T) {
	db, _ := startPostgres(t)
	migrateFully(t, db) // first Up, exercised inside migrateFully

	provider, err := migrate.SchemaProvider(db)
	if err != nil {
		t.Fatalf("SchemaProvider failed: %v", err)
	}
	ctx := context.Background()

	if _, err := provider.DownTo(ctx, 0); err != nil {
		t.Fatalf("schema DownTo(0) failed: %v", err)
	}

	var usersExists bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users')`).Scan(&usersExists); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if usersExists {
		t.Fatalf("expected users table to be gone after DownTo(0)")
	}

	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("second schema Up failed: %v", err)
	}

	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users')`).Scan(&usersExists); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if !usersExists {
		t.Fatalf("expected users table to exist after re-applying Up")
	}

	var auditLogExists bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'audit_log')`).Scan(&auditLogExists); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if !auditLogExists {
		t.Fatalf("expected audit_log table to exist after re-applying Up")
	}
}

// D-C: the partitioned-by-month shape is honoured natively; pg_partman
// is confirmed absent from the pinned image, which is exactly why.
func testAuditLogPartitionedMonthlyAndPgPartmanAbsent(t *testing.T) {
	db, _ := startPostgres(t)
	migrateFully(t, db)

	var partmanAvailable bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'pg_partman')`).Scan(&partmanAvailable); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if partmanAvailable {
		t.Fatalf("expected pg_partman to be unavailable on postgres:17-alpine (core+contrib only)")
	}

	var partitionCount int
	if err := db.QueryRow(`SELECT count(*) FROM pg_partition_tree('audit_log'::regclass) WHERE relid <> 'audit_log'::regclass`).Scan(&partitionCount); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if partitionCount != 12 {
		t.Fatalf("expected 12 pre-created monthly partitions, got %d", partitionCount)
	}

	var isPartitioned bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_partitioned_table WHERE partrelid = 'audit_log'::regclass)`).Scan(&isPartitioned); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if !isPartitioned {
		t.Fatalf("expected audit_log to be a partitioned table")
	}
}

// db-access-control: Runtime Role Append-Only Restriction.
func testAppRWCannotWriteAuditLogParent(t *testing.T) {
	db, superuserDSN := startPostgres(t)
	migrateFully(t, db)
	appRW := connectAsAppRW(t, superuserDSN)

	if _, err := appRW.Exec(`UPDATE audit_log SET action = 'tampered'`); err == nil {
		t.Fatalf("expected UPDATE on audit_log to be rejected for app_rw")
	} else if !isInsufficientPrivilege(err) {
		t.Fatalf("expected insufficient-privilege error, got: %v", err)
	}

	if _, err := appRW.Exec(`DELETE FROM audit_log`); err == nil {
		t.Fatalf("expected DELETE on audit_log to be rejected for app_rw")
	} else if !isInsufficientPrivilege(err) {
		t.Fatalf("expected insufficient-privilege error, got: %v", err)
	}

	if _, err := appRW.Exec(`TRUNCATE audit_log`); err == nil {
		t.Fatalf("expected TRUNCATE on audit_log to be rejected for app_rw")
	} else if !isInsufficientPrivilege(err) {
		t.Fatalf("expected insufficient-privilege error, got: %v", err)
	}
}

// db-access-control: Append-Only Restriction Extends To Every Partition.
func testAppRWCannotWriteAuditLogPartition(t *testing.T) {
	db, superuserDSN := startPostgres(t)
	migrateFully(t, db)
	appRW := connectAsAppRW(t, superuserDSN)

	partitionName := currentMonthPartitionName(t, db)

	if _, err := appRW.Exec(fmt.Sprintf(`UPDATE %s SET action = 'tampered'`, partitionName)); err == nil {
		t.Fatalf("expected UPDATE on partition %s to be rejected for app_rw", partitionName)
	} else if !isInsufficientPrivilege(err) {
		t.Fatalf("expected insufficient-privilege error, got: %v", err)
	}
}

// db-access-control / D-B mechanism 2, non-droppable test (a):
// CREATE TABLE ... PARTITION OF audit_log as vecingest_owner. The
// resulting child MUST carry no write grant for app_rw, with NO
// explicit revoke issued anywhere in this test -- the fail-closed
// default-privilege baseline plus the bootstrap event trigger are the
// only things that can make this pass.
func testPartitionOfGuardCoversNewChild(t *testing.T) {
	db, superuserDSN := startPostgres(t)
	migrateFully(t, db)

	// A partition well outside the 12 pre-created months, so this test
	// cannot be passing merely because 00002_audit_log.sql's own
	// explicit per-partition REVOKE already covered it.
	start := time.Now().AddDate(0, 24, 0)
	partitionName := fmt.Sprintf("audit_log_%s", start.Format("2006_01"))
	rangeStart := start.Format("2006-01-02")
	rangeEnd := start.AddDate(0, 1, 0).Format("2006-01-02")

	withOwnerConn(t, db, func(conn *sql.Conn) {
		// G201 false positive: partitionName/rangeStart/rangeEnd are all
		// derived from time.Now() a few lines above in this same test
		// function, never from external or request-controlled input, and
		// DDL identifiers/date literals cannot be bound as query
		// parameters the way DML values can.
		stmt := fmt.Sprintf( //nolint:gosec // G201: test-only DDL built from local time.Now()-derived values, not external input
			`CREATE TABLE %s PARTITION OF audit_log FOR VALUES FROM ('%s') TO ('%s')`,
			partitionName, rangeStart, rangeEnd,
		)
		if _, err := conn.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("failed to create new partition as vecingest_owner: %v", err)
		}
	})

	appRW := connectAsAppRW(t, superuserDSN)
	if _, err := appRW.Exec(fmt.Sprintf(`UPDATE %s SET action = 'tampered'`, partitionName)); err == nil {
		t.Fatalf("expected UPDATE on newly-attached partition %s to be rejected for app_rw with no explicit revoke in this test", partitionName)
	} else if !isInsufficientPrivilege(err) {
		t.Fatalf("expected insufficient-privilege error, got: %v", err)
	}
}

// db-access-control / D-B mechanism 2, non-droppable test (b):
// CREATE TABLE (LIKE audit_log) then ALTER TABLE audit_log ATTACH
// PARTITION, as vecingest_owner. This is the test that actually proves
// which relation pg_event_trigger_ddl_commands() reports for ATTACH,
// which PostgreSQL's own documentation does not state. NO explicit
// revoke is issued anywhere in this test.
func testAttachPartitionGuardCoversNewChild(t *testing.T) {
	db, superuserDSN := startPostgres(t)
	migrateFully(t, db)

	start := time.Now().AddDate(0, 25, 0)
	partitionName := fmt.Sprintf("audit_log_attached_%s", start.Format("2006_01"))
	rangeStart := start.Format("2006-01-02")
	rangeEnd := start.AddDate(0, 1, 0).Format("2006-01-02")

	withOwnerConn(t, db, func(conn *sql.Conn) {
		ctx := context.Background()
		createStmt := fmt.Sprintf(`CREATE TABLE %s (LIKE audit_log)`, partitionName)
		if _, err := conn.ExecContext(ctx, createStmt); err != nil {
			t.Fatalf("failed to create table LIKE audit_log as vecingest_owner: %v", err)
		}
		// G201 false positive: same reasoning as the sibling test above --
		// every interpolated value is local, time.Now()-derived test data.
		attachStmt := fmt.Sprintf( //nolint:gosec // G201: test-only DDL built from local time.Now()-derived values, not external input
			`ALTER TABLE audit_log ATTACH PARTITION %s FOR VALUES FROM ('%s') TO ('%s')`,
			partitionName, rangeStart, rangeEnd,
		)
		if _, err := conn.ExecContext(ctx, attachStmt); err != nil {
			t.Fatalf("failed to attach partition as vecingest_owner: %v", err)
		}
	})

	appRW := connectAsAppRW(t, superuserDSN)
	if _, err := appRW.Exec(fmt.Sprintf(`UPDATE %s SET action = 'tampered'`, partitionName)); err == nil {
		t.Fatalf("expected UPDATE on ATTACHed partition %s to be rejected for app_rw with no explicit revoke in this test", partitionName)
	} else if !isInsufficientPrivilege(err) {
		t.Fatalf("expected insufficient-privilege error, got: %v", err)
	}
}

// D-L: River's schema is provisioned at the pinned target version, and
// its tables are explicitly NOT append-only.
func testRiverGrantsPresentAndSchemaAtPinnedVersion(t *testing.T) {
	db, superuserDSN := startPostgres(t)
	migrateFully(t, db)

	var maxVersion int
	if err := db.QueryRow(`SELECT COALESCE(max(version), 0) FROM river_migration`).Scan(&maxVersion); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if maxVersion != schema.RiverTargetVersion {
		t.Fatalf("expected river_migration max version %d, got %d", schema.RiverTargetVersion, maxVersion)
	}

	rows, err := db.Query(`
		SELECT c.relname
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relname LIKE 'river\_%' ESCAPE '\'
	`)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("failed to close rows: %v", err)
		}
	}()
	var riverTables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		riverTables = append(riverTables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if len(riverTables) == 0 {
		t.Fatalf("expected at least one river_* table after schema migration")
	}

	appRW := connectAsAppRW(t, superuserDSN)
	for _, table := range riverTables {
		for _, priv := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
			var has bool
			// G201 false positive: table comes from riverTables (read back
			// from pg_class two-hundred lines above by pattern, never from
			// external input) and priv iterates a hardcoded literal slice.
			q := fmt.Sprintf(`SELECT has_table_privilege('app_rw', '%s', '%s')`, table, priv) //nolint:gosec // G201: table/priv are test-local values (pg_class readback + a hardcoded literal slice), not external input
			if err := db.QueryRow(q).Scan(&has); err != nil {
				t.Fatalf("query failed: %v", err)
			}
			if !has {
				t.Fatalf("expected app_rw to hold %s on %s (river tables are not append-only)", priv, table)
			}
		}
		if _, err := appRW.Exec(fmt.Sprintf(`DELETE FROM %s WHERE false`, table)); err != nil {
			t.Fatalf("expected app_rw to be able to DELETE (matching zero rows) on %s, got: %v", table, err)
		}
	}

	var jobCount int
	if err := db.QueryRow(`SELECT count(*) FROM river_job`).Scan(&jobCount); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if jobCount != 0 {
		t.Fatalf("expected river_job to be empty (zero producers at M0), got %d rows", jobCount)
	}
}

// currentMonthPartitionName resolves the name of the partition covering
// "now" among the 12 pre-created monthly partitions.
func currentMonthPartitionName(t *testing.T, db *sql.DB) string {
	t.Helper()
	var name string
	const q = `
		SELECT c.relname
		FROM pg_partition_tree('audit_log'::regclass) pt
		JOIN pg_class c ON c.oid = pt.relid
		WHERE pt.relid <> 'audit_log'::regclass
		  AND pg_get_expr(c.relpartbound, c.oid) LIKE '%' || to_char(now(), 'YYYY-MM') || '%'
		LIMIT 1
	`
	if err := db.QueryRow(q).Scan(&name); err != nil {
		t.Fatalf("failed to resolve current-month partition name: %v", err)
	}
	return name
}

// isInsufficientPrivilege reports whether err is PostgreSQL's
// insufficient_privilege error class (SQLSTATE 42501).
func isInsufficientPrivilege(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42501"
	}
	return false
}
