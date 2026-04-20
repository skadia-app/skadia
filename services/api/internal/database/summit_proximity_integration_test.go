//go:build integration

package database

// Integration tests for FindSummitsAlongGeom and FindSummitsAlongActivityTrack.
//
// Run with:
//
//	TEST_DATABASE_URL=... go test ./internal/database/ -tags=integration -run TestFindSummits -v
//
// Tests insert ephemeral osm_pois rows and delete them in t.Cleanup.
// The tests skip automatically when TEST_DATABASE_URL is not set.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// summitProxTestPool returns a *pgxpool.Pool connected to TEST_DATABASE_URL.
// Skips if the env var is unset.
func summitProxTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("summitProxTestPool: connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// insertTestSummit inserts a minimal osm_pois row of category 'summit' at the
// given WGS-84 coordinates and registers cleanup. Returns the inserted osm_id.
//
// Uses a large negative ID derived from nanosecond timestamp to avoid
// collisions with real OSM data.
func insertTestSummit(t *testing.T, pool *pgxpool.Pool, name string, lat, lng float64) int64 {
	t.Helper()
	ctx := context.Background()

	osmID := -(time.Now().UnixNano() % 1_000_000_000)

	_, err := pool.Exec(ctx, `
		INSERT INTO osm_pois (osm_id, name, category, geom)
		VALUES ($1, $2, 'summit',
		        ST_SetSRID(ST_MakePoint($4, $3), 4326))
	`, osmID, name, lat, lng)
	if err != nil {
		t.Fatalf("insertTestSummit: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM osm_pois WHERE osm_id = $1`, osmID)
	})
	return osmID
}

// buildRouteWKB asks PostGIS to produce the WKB for a two-point LineString.
// WKB is used directly as the geomWKB argument to FindSummitsAlongGeom,
// avoiding hand-rolled binary encoding.
func buildRouteWKB(t *testing.T, pool *pgxpool.Pool, lat1, lng1, lat2, lng2 float64) []byte {
	t.Helper()
	var wkb []byte
	err := pool.QueryRow(context.Background(), `
		SELECT ST_AsWKB(
			ST_SetSRID(
				ST_MakeLine(
					ST_MakePoint($2, $1),
					ST_MakePoint($4, $3)
				), 4326)
		)
	`, lat1, lng1, lat2, lng2).Scan(&wkb)
	if err != nil {
		t.Fatalf("buildRouteWKB: %v", err)
	}
	return wkb
}

// ---------------------------------------------------------------------------
// TestFindSummitsAlongGeom_Found: POI at ~55 m from route, radius 100 → found.
// ---------------------------------------------------------------------------
func TestFindSummitsAlongGeom_Found(t *testing.T) {
	pool := summitProxTestPool(t)
	ctx := context.Background()

	// Summit at (47.000000, 11.000000).
	name := fmt.Sprintf("TestGipfel-%d", time.Now().UnixNano())
	osmID := insertTestSummit(t, pool, name, 47.0, 11.0)

	// Route passes ~55 m north of the summit.
	wkb := buildRouteWKB(t, pool, 47.0005, 10.999, 47.0005, 11.001)

	summits, err := FindSummitsAlongGeom(ctx, pool, wkb, 100)
	if err != nil {
		t.Fatalf("FindSummitsAlongGeom: %v", err)
	}

	found := false
	for _, s := range summits {
		if s.OsmID == osmID {
			found = true
			if s.Category != "summit" {
				t.Errorf("category = %q, want summit", s.Category)
			}
			if s.DistM > 100 {
				t.Errorf("dist_m = %d, should be ≤ 100", s.DistM)
			}
		}
	}
	if !found {
		t.Errorf("expected summit osm_id=%d in results %v", osmID, summits)
	}
}

// ---------------------------------------------------------------------------
// TestFindSummitsAlongGeom_NotFound: POI at ~55 m, radius 10 → not returned.
// ---------------------------------------------------------------------------
func TestFindSummitsAlongGeom_NotFound(t *testing.T) {
	pool := summitProxTestPool(t)
	ctx := context.Background()

	name := fmt.Sprintf("TestGipfelFar-%d", time.Now().UnixNano())
	osmID := insertTestSummit(t, pool, name, 47.0, 11.0)
	wkb := buildRouteWKB(t, pool, 47.0005, 10.999, 47.0005, 11.001)

	summits, err := FindSummitsAlongGeom(ctx, pool, wkb, 10)
	if err != nil {
		t.Fatalf("FindSummitsAlongGeom: %v", err)
	}

	for _, s := range summits {
		if s.OsmID == osmID {
			t.Errorf("summit at ~55 m should NOT appear with radius=10, but got dist_m=%d", s.DistM)
		}
	}
}

// ---------------------------------------------------------------------------
// TestFindSummitsAlongActivityTrack_MissingActivity: unknown activity → no error, empty.
// ---------------------------------------------------------------------------
func TestFindSummitsAlongActivityTrack_MissingActivity(t *testing.T) {
	pool := summitProxTestPool(t)
	ctx := context.Background()

	summits, err := FindSummitsAlongActivityTrack(ctx, pool, "00000000-0000-0000-0000-000000000000", 100)
	if err != nil {
		t.Fatalf("unexpected error for missing activity: %v", err)
	}
	if len(summits) != 0 {
		t.Errorf("expected empty slice for missing activity, got %d entries", len(summits))
	}
}
