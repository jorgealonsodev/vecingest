package test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	incidentdb "github.com/jorgealonsodev/vecingest/internal/db"
)

func TestIncidentPersistence(t *testing.T) {
	sqlDB, dsn := startPostgres(t)
	migrateFully(t, sqlDB)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("create pgx pool: %v", err)
	}
	t.Cleanup(pool.Close)
	q := incidentdb.New(pool)
	f := seedIncidentPersistence(t, sqlDB)

	t.Run("integrity and documented values", func(t *testing.T) {
		common := persistTestIncident(t, q, f.community, pgtype.UUID{}, f.creator, "common")
		if common.Status != "open" || common.Priority != "normal" || common.AffectedCount != 1 {
			t.Fatalf("defaults = status %q, priority %q, affected_count %d; want open, normal, 1", common.Status, common.Priority, common.AffectedCount)
		}
		unitIncident := persistTestIncident(t, q, f.community, incidentUUID(f.unitOne), f.creator, "unit")
		if unitIncident.Scope != "unit" || unitIncident.UnitID.Bytes != f.unitOne {
			t.Fatalf("unit incident target/scope mismatch: scope=%q unit=%v", unitIncident.Scope, unitIncident.UnitID)
		}

		for _, category := range []string{"elevator", "plumbing", "electricity", "cleaning", "locksmith", "gardening", "works", "mandatory_works", "noise_and_coexistence", "other"} {
			if _, err := sqlDB.ExecContext(ctx, `UPDATE incidents SET category = $2 WHERE id = $1`, common.ID, category); err != nil {
				t.Errorf("documented category %q rejected: %v", category, err)
			}
		}
		if _, err := sqlDB.ExecContext(ctx, `UPDATE incidents SET category = 'plumbing' WHERE id = $1`, common.ID); err != nil {
			t.Fatalf("restore test category: %v", err)
		}
		for _, status := range []string{"open", "assigned", "in_progress", "resolved", "closed", "rejected"} {
			if _, err := sqlDB.ExecContext(ctx, `UPDATE incidents SET status = $2 WHERE id = $1`, common.ID, status); err != nil {
				t.Errorf("documented status %q rejected: %v", status, err)
			}
		}
		for _, priority := range []string{"low", "normal", "high", "urgent"} {
			if _, err := sqlDB.ExecContext(ctx, `UPDATE incidents SET priority = $2 WHERE id = $1`, common.ID, priority); err != nil {
				t.Errorf("documented priority %q rejected: %v", priority, err)
			}
		}

		if _, err := sqlDB.ExecContext(ctx, `UPDATE incidents SET category = 'invented' WHERE id = $1`, common.ID); err == nil {
			t.Fatal("expected undocumented category to be rejected")
		}
		if _, err := sqlDB.ExecContext(ctx, `UPDATE incidents SET status = 'pending' WHERE id = $1`, common.ID); err == nil {
			t.Fatal("expected undocumented status to be rejected")
		}
		if _, err := sqlDB.ExecContext(ctx, `UPDATE incidents SET priority = 'critical' WHERE id = $1`, common.ID); err == nil {
			t.Fatal("expected undocumented priority to be rejected")
		}
		if _, err := sqlDB.ExecContext(ctx, `UPDATE incidents SET status = 'open', priority = 'normal' WHERE id = $1`, common.ID); err != nil {
			t.Fatalf("restore test defaults: %v", err)
		}
		for _, tc := range []struct {
			name string
			args incidentdb.InsertIncidentParams
		}{
			{"cross-community unit", incidentParams(f.community, incidentUUID(f.foreignUnit), f.creator, "unit")},
			{"common with unit", incidentParams(f.community, incidentUUID(f.unitOne), f.creator, "common")},
			{"unit without unit", incidentParams(f.community, pgtype.UUID{}, f.creator, "unit")},
			{"undocumented scope", incidentParams(f.community, pgtype.UUID{}, f.creator, "private")},
			{"missing creator", incidentParams(f.community, pgtype.UUID{}, uuid.New(), "common")},

		} {
			t.Run(tc.name, func(t *testing.T) {
				tc.args.ID = uuid.New()
				if _, err := q.InsertIncident(ctx, tc.args); err == nil {
					t.Fatalf("expected %s insert to fail", tc.name)
				}
			})
		}
		if _, err := sqlDB.ExecContext(ctx, `UPDATE incidents SET deleted_at = now() WHERE community_id = $1`, f.community); err != nil {
			t.Fatalf("isolate visibility fixtures from integrity fixtures: %v", err)
		}
	})

	t.Run("visibility, filters, and same-timestamp keyset", func(t *testing.T) {
		common := persistTestIncident(t, q, f.community, pgtype.UUID{}, f.creator, "common")
		privateOne := persistTestIncident(t, q, f.community, incidentUUID(f.unitOne), f.creator, "unit")
		privateTwo := persistTestIncident(t, q, f.community, incidentUUID(f.unitTwo), f.otherCreator, "unit")
		tieTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		for _, id := range []uuid.UUID{common.ID, privateOne.ID, privateTwo.ID} {
			if _, err := sqlDB.ExecContext(ctx, `UPDATE incidents SET created_at = $2 WHERE id = $1`, id, tieTime); err != nil {
				t.Fatalf("set deterministic keyset timestamp: %v", err)
			}
		}

		assertVisibleCount := func(t *testing.T, user uuid.UUID, want int) {
			t.Helper()
			rows, err := q.ListVisibleIncidents(ctx, incidentdb.ListVisibleIncidentsParams{
				CommunityID: f.community, UserID: user, Status: pgtype.Text{}, Category: pgtype.Text{},
					UnitID: pgtype.UUID{}, CursorCreatedAt: pgtype.Timestamptz{}, CursorID: pgtype.UUID{}, PageSize: 20,
			})
			if err != nil {
				t.Fatalf("ListVisibleIncidents: %v", err)
			}
			if len(rows) != want {
				t.Fatalf("visible incident count = %d, want %d", len(rows), want)
			}
		}
		assertVisibleCount(t, f.unitMember, 2) // common plus same-unit private
		assertVisibleCount(t, f.creator, 2)
		assertVisibleCount(t, f.president, 2) // common plus own unit, never the other unit
		assertVisibleCount(t, f.admin, 3)
		assertVisibleCount(t, f.adminStaff, 3)
		assertVisibleCount(t, f.outsider, 0)
		assertVisibleCount(t, f.foreignAdmin, 0)

		if _, err := sqlDB.ExecContext(ctx, `UPDATE unit_members SET valid_to = now() - interval '1 second' WHERE community_id = $1 AND user_id = $2`, f.community, f.unitMember); err != nil {
			t.Fatalf("expire unit membership: %v", err)
		}
		assertVisibleCount(t, f.unitMember, 0)
		if _, err := sqlDB.ExecContext(ctx, `UPDATE unit_members SET valid_to = NULL WHERE community_id = $1 AND user_id = $2`, f.community, f.unitMember); err != nil {
			t.Fatalf("restore unit membership: %v", err)
		}
		if _, err := sqlDB.ExecContext(ctx, `UPDATE unit_members SET valid_to = now() - interval '1 second' WHERE community_id = $1 AND user_id = $2`, f.community, f.creator); err != nil {
			t.Fatalf("expire creator membership: %v", err)
		}
		creatorRows, err := q.ListVisibleIncidents(ctx, incidentdb.ListVisibleIncidentsParams{
			CommunityID: f.community, UserID: f.creator, Status: pgtype.Text{}, Category: pgtype.Text{},
			UnitID: pgtype.UUID{}, CursorCreatedAt: pgtype.Timestamptz{}, CursorID: pgtype.UUID{}, PageSize: 20,
		})
		if err != nil || len(creatorRows) != 1 || creatorRows[0].ID != privateOne.ID {
			t.Fatalf("creator lost access after membership ended: rows=%v err=%v", incidentIDs(creatorRows), err)
		}
		if _, err := sqlDB.ExecContext(ctx, `UPDATE unit_members SET valid_to = NULL WHERE community_id = $1 AND user_id = $2`, f.community, f.creator); err != nil {
			t.Fatalf("restore creator membership: %v", err)
		}

		if _, err := q.GetVisibleIncidentByID(ctx, incidentdb.GetVisibleIncidentByIDParams{
			CommunityID: f.community, UserID: f.president, ID: privateOne.ID,
		}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("presidency alone exposed another unit's incident: err=%v", err)
		}
		if _, err := q.GetVisibleIncidentByID(ctx, incidentdb.GetVisibleIncidentByIDParams{
			CommunityID: f.foreignCommunity, UserID: f.foreignAdmin, ID: privateOne.ID,
		}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("foreign community detail lookup was not isolated: err=%v", err)
		}

		filtered, err := q.ListVisibleIncidents(ctx, incidentdb.ListVisibleIncidentsParams{
			CommunityID: f.community, UserID: f.admin, Status: pgtype.Text{String: "open", Valid: true},
			Category: pgtype.Text{String: "plumbing", Valid: true}, UnitID: incidentUUID(f.unitOne), CursorCreatedAt: pgtype.Timestamptz{},
			CursorID: pgtype.UUID{}, PageSize: 20,
		})
		if err != nil || len(filtered) != 1 || filtered[0].ID != privateOne.ID {
			t.Fatalf("unit filter result = %v, err=%v; want only %s", incidentIDs(filtered), err, privateOne.ID)
		}

		firstPage, err := q.ListVisibleIncidents(ctx, incidentdb.ListVisibleIncidentsParams{
			CommunityID: f.community, UserID: f.admin, Status: pgtype.Text{}, Category: pgtype.Text{},
			UnitID: pgtype.UUID{}, CursorCreatedAt: pgtype.Timestamptz{}, CursorID: pgtype.UUID{}, PageSize: 2,
		})
		if err != nil || len(firstPage) != 2 {
			t.Fatalf("first keyset page: rows=%d err=%v", len(firstPage), err)
		}
		allIDs := []uuid.UUID{common.ID, privateOne.ID, privateTwo.ID}
		sort.Slice(allIDs, func(i, j int) bool { return bytes.Compare(allIDs[i][:], allIDs[j][:]) > 0 })
		if firstPage[0].ID != allIDs[0] || firstPage[1].ID != allIDs[1] {
			t.Fatalf("same-timestamp ordering = %v, want %v", incidentIDs(firstPage), allIDs[:2])
		}
		last := firstPage[len(firstPage)-1]
		secondPage, err := q.ListVisibleIncidents(ctx, incidentdb.ListVisibleIncidentsParams{
			CommunityID: f.community, UserID: f.admin, Status: pgtype.Text{}, Category: pgtype.Text{},
			UnitID: pgtype.UUID{}, CursorCreatedAt: pgtype.Timestamptz{Time: last.CreatedAt, Valid: true},
			CursorID: incidentUUID(last.ID), PageSize: 2,
		})
		if err != nil || len(secondPage) != 1 || secondPage[0].ID != allIDs[2] {
			t.Fatalf("second keyset page = %v err=%v, want [%s]", incidentIDs(secondPage), err, allIDs[2])
		}
	})
}

func incidentParams(community uuid.UUID, unit pgtype.UUID, creator uuid.UUID, scope string) incidentdb.InsertIncidentParams {
	return incidentdb.InsertIncidentParams{
		ID: uuid.New(), CommunityID: community, UnitID: unit, CreatedBy: creator,
		Title: "Leaking pipe", Description: "Water is visible", Category: "plumbing",
		Scope: scope, LocationText: pgtype.Text{},
	}
}

func persistTestIncident(t *testing.T, q *incidentdb.Queries, community uuid.UUID, unit pgtype.UUID, creator uuid.UUID, scope string) incidentdb.Incident {
	t.Helper()
	incident, err := q.InsertIncident(context.Background(), incidentParams(community, unit, creator, scope))
	if err != nil {
		t.Fatalf("InsertIncident: %v", err)
	}
	return incident
}

func incidentUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func incidentIDs(rows []incidentdb.Incident) []uuid.UUID {
	ids := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	return ids
}

type incidentFixture struct {
	community, foreignCommunity uuid.UUID
	unitOne, unitTwo, foreignUnit uuid.UUID
	creator, unitMember, otherCreator, president, outsider, admin, adminStaff, foreignAdmin uuid.UUID
}

func seedIncidentPersistence(t *testing.T, db *sql.DB) incidentFixture {
	t.Helper()
	ctx := context.Background()
	f := incidentFixture{
		community: uuid.New(), foreignCommunity: uuid.New(),
		unitOne: uuid.New(), unitTwo: uuid.New(), foreignUnit: uuid.New(),
		creator: uuid.New(), unitMember: uuid.New(), otherCreator: uuid.New(), president: uuid.New(), outsider: uuid.New(),
		admin: uuid.New(), adminStaff: uuid.New(), foreignAdmin: uuid.New(),
	}
	users := []struct {
		id    uuid.UUID
		label string
	}{
		{f.creator, "creator"}, {f.unitMember, "unit-member"}, {f.otherCreator, "other-creator"},
		{f.president, "president"}, {f.outsider, "outsider"}, {f.admin, "admin"},
		{f.adminStaff, "admin-staff"}, {f.foreignAdmin, "foreign-admin"},
	}
	for _, user := range users {
		if _, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, 'test-hash', $3)`,
			user.id, user.id.String()+"@example.invalid", user.label); err != nil {
			t.Fatalf("insert test user %s: %v", user.label, err)
		}
	}
	officeOne, officeTwo := uuid.New(), uuid.New()
	for i, office := range []uuid.UUID{officeOne, officeTwo} {
		if _, err := db.ExecContext(ctx, `INSERT INTO offices (id, name, cif) VALUES ($1, $2, $3)`, office, fmt.Sprintf("Test Office %d", i), fmt.Sprintf("T%08d", i)); err != nil {
			t.Fatalf("insert test office: %v", err)
		}
	}
	for _, member := range []struct {
		id, office, user uuid.UUID
		role             string
	}{{uuid.New(), officeOne, f.admin, "admin"}, {uuid.New(), officeOne, f.adminStaff, "admin_staff"}, {uuid.New(), officeTwo, f.foreignAdmin, "admin"}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO office_members (id, office_id, user_id, role) VALUES ($1, $2, $3, $4)`, member.id, member.office, member.user, member.role); err != nil {
			t.Fatalf("insert office member: %v", err)
		}
	}
	for _, community := range []struct {
		id, office uuid.UUID
		name       string
	}{{f.community, officeOne, "Incident Community"}, {f.foreignCommunity, officeTwo, "Foreign Community"}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO communities (id, office_id, name) VALUES ($1, $2, $3)`, community.id, community.office, community.name); err != nil {
			t.Fatalf("insert test community: %v", err)
		}
	}
	for _, unit := range []struct{ id, community uuid.UUID }{{f.unitOne, f.community}, {f.unitTwo, f.community}, {f.foreignUnit, f.foreignCommunity}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO units (id, community_id, type) VALUES ($1, $2, 'flat')`, unit.id, unit.community); err != nil {
			t.Fatalf("insert test unit: %v", err)
		}
	}
	for _, member := range []struct {
		id, unit, community, user uuid.UUID
		role                     string
		boardRole                sql.NullString
	}{{uuid.New(), f.unitOne, f.community, f.creator, "owner", sql.NullString{}},
		{uuid.New(), f.unitOne, f.community, f.unitMember, "tenant", sql.NullString{}},
		{uuid.New(), f.unitTwo, f.community, f.otherCreator, "tenant", sql.NullString{}},
		{uuid.New(), f.unitTwo, f.community, f.president, "owner", sql.NullString{String: "president", Valid: true}}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO unit_members (id, unit_id, community_id, user_id, role, board_role) VALUES ($1, $2, $3, $4, $5, $6)`,
			member.id, member.unit, member.community, member.user, member.role, nullableString(member.boardRole)); err != nil {
			t.Fatalf("insert test unit member: %v", err)
		}
	}
	return f
}

func nullableString(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}
