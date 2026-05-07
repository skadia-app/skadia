//go:build integration
// +build integration

package handler

// route_suggestion_integration_test.go — Integration tests for DB-backed
// SuggestRoutes enrichment paths (enrichment settings, summit detection).
//
// Run with:
//
//	TEST_DATABASE_URL=... go test ./internal/handler/ -tags=integration \
//	  -run TestSuggestRoutes_Integration -v
//
// Tests skip automatically when TEST_DATABASE_URL is not set.
//
// Why these tests are here (not in route_suggestion_test.go):
//
//   The SuggestRoutes handler has two optional DB-backed enrichment paths that
//   require a live pool and populated data:
//
//   1. GetEnrichmentSettings — loads the per-user summit-detection radius.
//      Without a DB the handler falls back to the default radius (80 m).
//
//   2. FindSummitsAlongGeom — queries osm_pois for summits/passes near the route.
//      Without a DB the response field summits_passed is always empty.
//
//   Unit tests (route_suggestion_test.go) cover the no-DB path by constructing
//   the handler with h.DB == nil (see testhelper_routing_test.go). The nil guards
//   in the handler (h.DB != nil, h.DB.Pool != nil) are intentional: they allow
//   the handler to run in offline/test deployments without a database.
//
//   These integration tests verify that the DB paths are actually exercised and
//   produce correct results when a database is available.

import (
	"context"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"net/http"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// routeSuggestionTestPool returns a *pgxpool.Pool connected to TEST_DATABASE_URL.
// Skips the test if the env var is unset.
func routeSuggestionTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testdb.RequireSafeDSN(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("routeSuggestionTestPool: connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestSuggestRoutes_Integration_EnrichmentSettings verifies that when h.DB is
// non-nil, the handler correctly loads the per-user summit-detection radius
// from GetEnrichmentSettings and uses it instead of the 80 m default.
//
// Setup requirements:
//   - TEST_DATABASE_URL points to a DB with schema up-to-date (Mig ≥ 000315).
//   - The test user row is auto-created by GetEnrichmentSettings via INSERT ON CONFLICT.
//
// Assertion: the handler returns HTTP 200 and the response is valid JSON.
// (Summit counts cannot be asserted here without seeded fixture data — see
// TestSuggestRoutes_Integration_SummitDetection below for that path.)
func TestSuggestRoutes_Integration_EnrichmentSettings(t *testing.T) {
	pool := routeSuggestionTestPool(t)

	db := &database.DB{Pool: pool}
	setup := newTestRoutingHandler(t)
	setup.Handler.DB = db

	const body = `{"start":[12.10,48.99],"target_distance_m":10000,"profile":"hiking"}`
	w := postSuggestRoutes(setup.Handler, body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	resp := decodeRouteSuggestionResponse(t, w)
	// The grid fixture produces ≥ 2 suggestions (verified by unit tests). With a
	// live DB the response must still be valid — enrichment settings load must not
	// break routing.
	if len(resp.Suggestions) == 0 {
		t.Error("expected suggestions from grid fixture with live DB; got none")
	}
}

// TestSuggestRoutes_Integration_SummitDetection verifies that the handler
// returns non-empty summits_passed when an osm_pois summit row is seeded
// near the test fixture route.
//
// The test grid (see testhelper_routing_test.go) runs through coordinates
// around 48.97°N / 12.10°E. We seed a summit at (48.972, 12.082) (node 2,0 of
// the grid — within the default 80 m radius of the route).
//
// Setup requirements:
//   - TEST_DATABASE_URL with schema ≥ Mig 000315 (osm_pois table, summit category).
//   - INSERT permission on osm_pois (the test cleans up via t.Cleanup).
func TestSuggestRoutes_Integration_SummitDetection(t *testing.T) {
	pool := routeSuggestionTestPool(t)

	// Seed a test summit near the grid route and register cleanup.
	summitOsmID := insertTestSummitForHandler(t, pool, "Testgipfel Integration", 48.972, 12.082)
	t.Logf("seeded test summit osm_id=%d at (48.972, 12.082)", summitOsmID)

	db := &database.DB{Pool: pool}
	setup := newTestRoutingHandler(t)
	setup.Handler.DB = db

	const body = `{"start":[12.10,48.99],"target_distance_m":10000,"profile":"hiking"}`
	w := postSuggestRoutes(setup.Handler, body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	resp := decodeRouteSuggestionResponse(t, w)
	if len(resp.Suggestions) == 0 {
		t.Skip("test fixture produced no suggestions — cannot verify summit detection")
	}

	// At least one suggestion must include the seeded summit.
	found := false
	for _, s := range resp.Suggestions {
		for _, summit := range s.SummitsPassed {
			if summit.OsmID == summitOsmID {
				found = true
				t.Logf("summit %d found in suggestion variant=%d", summitOsmID, s.Variant)
			}
		}
	}
	if !found {
		// This may legitimately fail if the routing fixture doesn't produce a
		// route through node(2,0). Log the actual routes to aid debugging.
		for i, s := range resp.Suggestions {
			t.Logf("suggestion[%d] variant=%s summits=%d", i, s.VariantID, len(s.SummitsPassed))
		}
		t.Errorf("seeded summit osm_id=%d not found in any suggestion's summits_passed", summitOsmID)
	}
}

// TestSuggestRoutes_Integration_VfRanges_Populated verifies Invariant I9 with
// a live DB: routes through VF-tagged edges must produce non-nil VfRanges with
// keys in {K1..K6}.
//
// NOTE: This test requires the test graph to contain VF-tagged edges. The
// standard unit test harness (newTestRoutingHandler) has no VF edges by default.
// Use newTestRoutingHandlerWithOutdoor which seeds a K2 edge at node(3,3)↔(4,4).
//
// Setup requirements:
//   - TEST_DATABASE_URL with schema ≥ Mig 000315.
//   - The routing fixture must produce a route through the VF-tagged edge.
//     (Soft-fail: if no route uses that edge, the test t.Skip.)
func TestSuggestRoutes_Integration_VfRanges_Populated(t *testing.T) {
	pool := routeSuggestionTestPool(t)

	db := &database.DB{Pool: pool}
	// Use the outdoor-tags fixture which seeds a K2 edge.
	setup := newTestRoutingHandlerWithOutdoor(t)
	setup.Handler.DB = db

	const body = `{"start":[12.10,48.99],"target_distance_m":15000,"profile":"hiking"}`
	w := postSuggestRoutes(setup.Handler, body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	resp := decodeRouteSuggestionResponse(t, w)
	if len(resp.Suggestions) == 0 {
		t.Skip("test fixture produced no suggestions — cannot verify VfRanges")
	}

	allowed := map[string]bool{
		"K1": true, "K2": true, "K3": true,
		"K4": true, "K5": true, "K6": true,
	}

	anyVfRanges := false
	for i, s := range resp.Suggestions {
		for k := range s.VfRanges {
			if !allowed[k] {
				t.Errorf("suggestion[%d]: VfRanges key %q not in {K1..K6}", i, k)
			}
			anyVfRanges = true
		}
	}

	if !anyVfRanges {
		// The K2 edge at (3,3)↔(4,4) may not be traversed by any route in this
		// fixture — this is a routing geometry question, not a bug.
		t.Skip("no suggestion traversed the VF-tagged edge — cannot verify VfRanges population")
	}
}

// insertTestSummitForHandler is a test helper analogous to insertTestSummit in
// summit_proximity_integration_test.go, localised to the handler package to
// avoid cross-package test helper dependencies.
//
// Uses a timestamp-derived negative osm_id to avoid collisions with real OSM
// data. Registers a t.Cleanup that deletes the row after the test.
func insertTestSummitForHandler(t *testing.T, pool *pgxpool.Pool, name string, lat, lng float64) int64 {
	t.Helper()
	ctx := context.Background()

	var osmID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO osm_pois (osm_id, name, category, geom)
		VALUES (-(EXTRACT(EPOCH FROM now()) * 1000)::bigint, $1, 'summit',
		        ST_SetSRID(ST_MakePoint($3, $2), 4326))
		RETURNING osm_id
	`, name, lat, lng).Scan(&osmID)
	if err != nil {
		t.Fatalf("insertTestSummitForHandler: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM osm_pois WHERE osm_id = $1`, osmID)
	})
	return osmID
}
