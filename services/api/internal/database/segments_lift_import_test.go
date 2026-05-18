package database

import (
	"context"
	"testing"
	"time"
)

func TestImportLiftSegmentsCreatesCompleteSegmentAndIsIdempotent(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()
	osmID := time.Now().UnixNano()
	cleanupLiftImportFixture(t, db, osmID)
	insertOSMLiftFixture(t, db, osmID, "Test lift")

	count, err := db.ImportLiftSegments(ctx, userID)
	if err != nil {
		t.Fatalf("ImportLiftSegments first: %v", err)
	}
	if count != 1 {
		t.Fatalf("first import count = %d, want 1", count)
	}
	assertImportedLiftShape(t, db, osmID, 1, 1)

	count, err = db.ImportLiftSegments(ctx, userID)
	if err != nil {
		t.Fatalf("ImportLiftSegments second: %v", err)
	}
	if count != 0 {
		t.Fatalf("second import count = %d, want 0", count)
	}
	assertImportedLiftShape(t, db, osmID, 1, 1)
}

func TestImportLiftSegmentsRepairsExistingSegmentWithoutVersion(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()
	osmID := time.Now().UnixNano()
	cleanupLiftImportFixture(t, db, osmID)
	insertOSMLiftFixture(t, db, osmID, "Repair lift")

	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segments (created_by, name, sport_group, segment_type, segment_type_meta, osm_lift_id, visibility)
		VALUES ($1, 'existing lift', 'generic', 'lift', '{}', $2, 'public')
	`, userID, osmID); err != nil {
		t.Fatalf("insert existing lift segment: %v", err)
	}

	count, err := db.ImportLiftSegments(ctx, userID)
	if err != nil {
		t.Fatalf("ImportLiftSegments: %v", err)
	}
	if count != 0 {
		t.Fatalf("repair import count = %d, want 0 new segments", count)
	}
	assertImportedLiftShape(t, db, osmID, 1, 1)
}

func insertOSMLiftFixture(t *testing.T, db *DB, osmID int64, name string) {
	t.Helper()
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO osm_lifts (osm_id, name, geom)
		VALUES ($1, $2, ST_GeomFromText('LINESTRING(7 47, 7.001 47.001)', 4326))
	`, osmID, name); err != nil {
		t.Fatalf("insert osm lift fixture: %v", err)
	}
}

func cleanupLiftImportFixture(t *testing.T, db *DB, osmID int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, `
		DELETE FROM segment_versions
		WHERE segment_id IN (SELECT id FROM segments WHERE osm_lift_id = $1)
	`, osmID); err != nil {
		t.Fatalf("cleanup segment_versions: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `DELETE FROM segments WHERE osm_lift_id = $1`, osmID); err != nil {
		t.Fatalf("cleanup segments: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `DELETE FROM osm_lifts WHERE osm_id = $1`, osmID); err != nil {
		t.Fatalf("cleanup osm_lifts: %v", err)
	}
}

func assertImportedLiftShape(t *testing.T, db *DB, osmID int64, wantSegments, wantVersions int) {
	t.Helper()
	ctx := context.Background()
	var segments, versions int
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM segments
		WHERE osm_lift_id = $1 AND segment_type = 'lift'
	`, osmID).Scan(&segments); err != nil {
		t.Fatalf("count lift segments: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM segment_versions sv
		JOIN segments s ON s.id = sv.segment_id
		WHERE s.osm_lift_id = $1 AND s.segment_type = 'lift'
	`, osmID).Scan(&versions); err != nil {
		t.Fatalf("count lift segment versions: %v", err)
	}
	if segments != wantSegments || versions != wantVersions {
		t.Fatalf("lift import shape = segments:%d versions:%d, want %d/%d", segments, versions, wantSegments, wantVersions)
	}
	var terrainType, climbType, elevationKind string
	if err := db.Pool.QueryRow(ctx, `
		SELECT terrain_type, climb_type, elevation_kind
		FROM segments
		WHERE osm_lift_id = $1 AND segment_type = 'lift'
		LIMIT 1
	`, osmID).Scan(&terrainType, &climbType, &elevationKind); err != nil {
		t.Fatalf("query lift classification: %v", err)
	}
	if terrainType != "lift" || climbType != "bergfahrt" || elevationKind != "uphill" {
		t.Fatalf("lift classification = terrain:%s climb:%s elevation:%s, want lift/bergfahrt/uphill",
			terrainType, climbType, elevationKind)
	}
}
