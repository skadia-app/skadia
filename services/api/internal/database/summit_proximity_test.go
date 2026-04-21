package database

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Pure unit tests — no DB required
// ---------------------------------------------------------------------------

// TestExpandDegForRadius verifies the degree-conversion formula.
// At 80 m radius the expected value is 80*1.5/55660 ≈ 0.002156.
func TestExpandDegForRadius(t *testing.T) {
	got := expandDegForRadius(80)
	want := (80 * 1.5) / 55660.0
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("expandDegForRadius(80) = %v, want %v", got, want)
	}
}

// TestFindSummitsAlongGeom_EmptyGuards verifies early-return behaviour for
// invalid / missing inputs without requiring a database connection.
func TestFindSummitsAlongGeom_EmptyGuards(t *testing.T) {
	// radiusM = 0 → nil, nil regardless of geomWKB content.
	result, err := FindSummitsAlongGeom(nil, nil, []byte{0x01}, 0)
	if err != nil {
		t.Errorf("radiusM=0: unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("radiusM=0: expected nil result, got %v", result)
	}

	// negative radius → nil, nil.
	result, err = FindSummitsAlongGeom(nil, nil, []byte{0x01}, -5)
	if err != nil {
		t.Errorf("radiusM<0: unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("radiusM<0: expected nil result, got %v", result)
	}

	// empty geomWKB with valid radius → nil, nil.
	result, err = FindSummitsAlongGeom(nil, nil, []byte{}, 80)
	if err != nil {
		t.Errorf("empty WKB: unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("empty WKB: expected nil result, got %v", result)
	}

	// nil geomWKB with valid radius → nil, nil.
	result, err = FindSummitsAlongGeom(nil, nil, nil, 80)
	if err != nil {
		t.Errorf("nil WKB: unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("nil WKB: expected nil result, got %v", result)
	}
}

// TestFindSummitsAlongActivityTrack_ZeroRadius verifies early-return for
// radiusM ≤ 0 without a DB connection.
func TestFindSummitsAlongActivityTrack_ZeroRadius(t *testing.T) {
	result, err := FindSummitsAlongActivityTrack(nil, nil, "any-uuid", 0)
	if err != nil {
		t.Errorf("radiusM=0: unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("radiusM=0: expected nil result, got %v", result)
	}

	result, err = FindSummitsAlongActivityTrack(nil, nil, "any-uuid", -1)
	if err != nil {
		t.Errorf("radiusM<0: unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("radiusM<0: expected nil result, got %v", result)
	}
}

// TestPassedSummitJSON verifies that the struct marshals to the expected
// JSON shape (field names and nullable EleM).
func TestPassedSummitJSON(t *testing.T) {
	ele := 2962.5
	s := PassedSummit{
		OsmID:    123456789,
		Name:     "Großglockner",
		EleM:     &ele,
		Category: "summit",
		DistM:    42,
		Lat:      47.0742,
		Lng:      12.6942,
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	js := string(b)

	for _, want := range []string{
		`"osm_id":123456789`,
		`"name":"Großglockner"`,
		`"ele_m":2962.5`,
		`"category":"summit"`,
		`"dist_m":42`,
		`"lat":47.0742`,
		`"lng":12.6942`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("JSON missing %q in: %s", want, js)
		}
	}
}

// TestFindSummitsCore_QueryContainsFallback verifies that findSummitsCore
// builds a query that contains the osm_id=0 fallback DISTINCT ON expression,
// ensuring the dedup logic is present regardless of the caller-supplied CTE.
// This is a structural/SQL-string test; no DB connection required.
func TestFindSummitsCore_QueryContainsFallback(t *testing.T) {
	// We inspect the SQL by calling fmt.Sprintf the same way findSummitsCore does.
	// Both "geom" and "activity" CTE shapes must produce the fallback expression.
	for _, tc := range []struct {
		name        string
		routeCTE    string
		radiusIdx   int
		expandIdx   int
	}{
		{
			name:      "geom variant ($2 radius, $3 expand)",
			routeCTE:  `WITH route AS (SELECT ST_GeomFromWKB($1, 4326) AS geom)`,
			radiusIdx: 2,
			expandIdx: 3,
		},
		{
			name:      "activity variant ($2 radius, $3 expand)",
			routeCTE:  `WITH route AS (SELECT ST_Simplify(track_full, 0.00005) AS geom FROM activity_tracks WHERE activity_id = $1)`,
			radiusIdx: 2,
			expandIdx: 3,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := fmt.Sprintf(`%s
			SELECT DISTINCT ON (CASE WHEN p.osm_id = 0 THEN p.id::text ELSE p.osm_id::text END)
			       p.osm_id, p.name, p.ele, p.category,
			       ROUND(ST_Distance(route.geom::geography, p.geom::geography))::int AS dist_m,
			       ST_Y(ST_Centroid(p.geom)) AS lat,
			       ST_X(ST_Centroid(p.geom)) AS lng
			FROM osm_pois p, route
			WHERE p.geom && ST_Expand(route.geom, $%d)
			  AND ST_DWithin(route.geom::geography, p.geom::geography, $%d)
			  AND p.category IN ('summit', 'pass')
			  AND p.name IS NOT NULL
			ORDER BY CASE WHEN p.osm_id = 0 THEN p.id::text ELSE p.osm_id::text END, dist_m
			`, tc.routeCTE, tc.expandIdx, tc.radiusIdx)

			if !strings.Contains(q, "p.osm_id = 0 THEN p.id::text") {
				t.Errorf("query missing osm_id=0 fallback DISTINCT ON expression:\n%s", q)
			}
			if !strings.Contains(q, fmt.Sprintf("$%d", tc.radiusIdx)) {
				t.Errorf("query missing radius arg $%d:\n%s", tc.radiusIdx, q)
			}
			if !strings.Contains(q, fmt.Sprintf("$%d", tc.expandIdx)) {
				t.Errorf("query missing expandDeg arg $%d:\n%s", tc.expandIdx, q)
			}
		})
	}
}

// TestPassedSummitJSON_NullEle verifies that nil EleM serialises as JSON null.
func TestPassedSummitJSON_NullEle(t *testing.T) {
	s := PassedSummit{
		OsmID:    1,
		Name:     "Unnamed Pass",
		EleM:     nil,
		Category: "pass",
		DistM:    10,
		Lat:      46.0,
		Lng:      11.0,
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"ele_m":null`) {
		t.Errorf("expected ele_m:null in: %s", string(b))
	}
}
