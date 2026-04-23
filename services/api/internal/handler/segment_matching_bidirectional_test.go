package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestMatcherSkipsReverseTraversal(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	userID := insertTestUser(t, pool)
	// Segment von West nach Ost (0 0 -> 1 0), NICHT bidirektional
	segmentID := insertTestSegment(t, pool, userID, TestSegmentOpts{
		Track:         "LINESTRING(0 0, 1 0)",
		Bidirectional: false,
		SegmentType:   "standard",
	})

	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	
	// Wir testen hier die interne Matcher-Logik indirekt über den Recompute-Pfad
	// oder direkt via Handler, falls verfügbar.
	// Da der Matcher oft asynchron läuft, prüfen wir hier die DB-Reaktion.
	
	// 1. Activity in RICHTIGER Richtung (0 0 -> 1 0)
	act1ID := insertTestActivity(t, pool, userID, "LINESTRING(0 0, 0.5 0, 1 0)")
	h.RecomputeEffortsForActivity(act1ID) // Synchroner Aufruf für Test
	
	var count1 int
	pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM segment_efforts WHERE activity_id = $1 AND segment_id = $2", act1ID, segmentID).Scan(&count1)
	require.Equal(t, 1, count1, "Should match in forward direction")

	// 2. Activity in FALSCHER Richtung (1 0 -> 0 0)
	act2ID := insertTestActivity(t, pool, userID, "LINESTRING(1 0, 0.5 0, 0 0)")
	h.RecomputeEffortsForActivity(act2ID)
	
	var count2 int
	pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM segment_efforts WHERE activity_id = $1 AND segment_id = $2", act2ID, segmentID).Scan(&count2)
	require.Equal(t, 0, count2, "Should NOT match in reverse direction for non-bidirectional segment")
}

func insertTestActivity(t *testing.T, pool *pgxpool.Pool, userID string, wkt string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), 
		"INSERT INTO activities (id, user_id, sport_type, track, started_at) VALUES (gen_random_uuid(), $1, 'cycling', ST_GeomFromText($2, 4326), NOW()) RETURNING id",
		userID, wkt).Scan(&id)
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	return id
}
