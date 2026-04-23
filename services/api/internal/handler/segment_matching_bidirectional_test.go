package handler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

func TestMatcherBidirectionalIntegration_DB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	pool := fusionTestPool(t)
	defer pool.Close()
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)
	db := h.DB

	// 1. Create a bidirectional=false segment
	// Tiny segment: 10.0000 50.0000 -> 10.0001 50.0001
	segID := uuid.New().String()
	fmt.Printf("DEBUG TEST: segID=%s\n", segID)
	_, err := pool.Exec(context.Background(), `
		INSERT INTO segments (id, created_by, name, sport_group, segment_type, bidirectional, visibility)
		VALUES ($1, $2, 'uni-test', 'bike', 'standard', false, 'public')
	`, segID, userID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(context.Background(), `
		INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		VALUES (gen_random_uuid(), $1, 1, ST_GeomFromText('LINESTRING(10.0000 50.0000, 10.0001 50.0001)', 4326), '1990-01-01', $2)
	`, segID, userID)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Insert activity with REVERSE stream
	// Must match the segment endpoints EXACTLY.
	// Reversed: 10.0001 50.0001 -> 10.0000 50.0000
	actID := insertMinimalActivity(t, db, userID)
	baseTime := time.Date(2024, 3, 1, 9, 0, 0, 0, time.UTC)
	
	gpsPoints := []model.StreamPoint{
		{Time: baseTime, Lat: fusionPtrF64(50.0001), Lng: fusionPtrF64(10.0001)},
		{Time: baseTime.Add(1 * time.Second), Lat: fusionPtrF64(50.00008), Lng: fusionPtrF64(10.00008)},
		{Time: baseTime.Add(2 * time.Second), Lat: fusionPtrF64(50.00006), Lng: fusionPtrF64(10.00006)},
		{Time: baseTime.Add(3 * time.Second), Lat: fusionPtrF64(50.00004), Lng: fusionPtrF64(10.00004)},
		{Time: baseTime.Add(4 * time.Second), Lat: fusionPtrF64(50.00002), Lng: fusionPtrF64(10.00002)},
		{Time: baseTime.Add(5 * time.Second), Lat: fusionPtrF64(50.0000), Lng: fusionPtrF64(10.0000)},
	}
	insertStreams(t, db, actID, gpsPoints)
	if err := db.EnsureActivityTrack(context.Background(), actID); err != nil {
		t.Fatal(err)
	}

	// 3. Match (Segment is unidirectional, stream is reversed -> 0 efforts)
	if err := h.RecomputeEffortsForActivity(context.Background(), actID, userID); err != nil {
		t.Fatalf("RecomputeEfforts: %v", err)
	}

	var count int
	pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM segment_efforts WHERE segment_id = $1", segID).Scan(&count)
	if count != 0 {
		t.Errorf("got %d efforts for unidirectional segment with reverse stream, want 0", count)
	}

	// 4. Update segment to bidirectional=true
	_, err = pool.Exec(context.Background(), "UPDATE segments SET bidirectional = true WHERE id = $1", segID)
	if err != nil {
		t.Fatal(err)
	}

	// 5. Match again
	if err := h.RecomputeEffortsForActivity(context.Background(), actID, userID); err != nil {
		t.Fatal(err)
	}

	// 6. Assert 1 reverse effort
	pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM segment_efforts WHERE segment_id = $1 AND is_reverse = true", segID).Scan(&count)
	if count != 1 {
		t.Errorf("got %d reverse efforts for bidirectional segment, want 1", count)
	}
}
