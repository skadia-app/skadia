package database

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestRouteProfileDifficultyScales(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	snapshot := createRouteProfileTestSnapshot(t, db, userID, json.RawMessage(`{
		"enrichment_version": 1,
		"forest_pct": 0.25,
		"protected_pct": 0.5,
		"sac_ranges": {
			"T1": [{"from": 0, "to": 1}],
			"T3": [{"from": 1, "to": 3}],
			"T5": [{"from": 3, "to": 4}]
		},
		"mtb_ranges": {
			"S0": [{"from": 0, "to": 1}],
			"S1": [{"from": 1, "to": 2}],
			"S2": [{"from": 2, "to": 3}],
			"S3": [{"from": 3, "to": 4}]
		},
		"vf_ranges": {
			"K2": [{"from": 0, "to": 1}]
		},
		"vf_unknown_ranges": [{"from": 1, "to": 2}],
		"summits_passed": [{"osm_id": 1, "name": "Summit", "lat": 47.0, "lng": 7.0}]
	}`))

	profile, err := db.CreateRouteProfileSnapshot(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("CreateRouteProfileSnapshot: %v", err)
	}

	var scales map[string]routeProfileTestScale
	if err := json.Unmarshal(profile.DifficultyScales, &scales); err != nil {
		t.Fatalf("unmarshal difficulty scales: %v", err)
	}
	sac := scales["sac"]
	if sac.MaxLevelKey != "T5" || sac.MaxLevelRank == nil || *sac.MaxLevelRank != 5 {
		t.Fatalf("SAC max = %#v, want T5 rank 5", sac)
	}
	if sac.DominantLevelKey != "T3" || sac.DominantLevelRank == nil || *sac.DominantLevelRank != 3 {
		t.Fatalf("SAC dominant = %#v, want T3 rank 3", sac)
	}
	if sac.WeightedMeanRank == nil || math.Abs(*sac.WeightedMeanRank-3) > 0.05 {
		t.Fatalf("SAC weighted mean = %v, want about 3", sac.WeightedMeanRank)
	}

	mtb := scales["mtb"]
	if mtb.MaxLevelKey != "S3" || len(mtb.Ranges["S0"]) != 1 || len(mtb.Ranges["S3"]) != 1 {
		t.Fatalf("MTB scale did not preserve full range distribution: %#v", mtb)
	}
	if scales["mtb_uphill"].State != "reserved" {
		t.Fatalf("mtb_uphill state = %#v, want reserved", scales["mtb_uphill"])
	}

	vf := scales["via_ferrata"]
	if vf.UnknownDistanceM <= 0 {
		t.Fatalf("via ferrata unknown distance = %f, want > 0", vf.UnknownDistanceM)
	}
	if _, ok := vf.Distribution["K0"]; ok {
		t.Fatalf("via ferrata unknown distance must not be encoded as K0: %#v", vf.Distribution)
	}

	var contextCounts map[string]int
	if err := json.Unmarshal(profile.ContextCounts, &contextCounts); err != nil {
		t.Fatalf("unmarshal context counts: %v", err)
	}
	if contextCounts["summits_passed"] != 1 {
		t.Fatalf("summits_passed count = %d, want 1", contextCounts["summits_passed"])
	}
}

func TestBuildRouteProfileSnapshotCopiesSurfaceAndWayProjection(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	snapshot := createRouteProfileTestSnapshot(t, db, userID, json.RawMessage(`{
		"enrichment_version": 1,
		"forest_pct": 0.2,
		"protected_pct": 0.3
	}`))

	profile, err := db.CreateRouteProfileSnapshot(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("CreateRouteProfileSnapshot: %v", err)
	}

	var surfaces map[string]routeProfileTestDistributionEntry
	if err := json.Unmarshal(profile.SurfaceDistribution, &surfaces); err != nil {
		t.Fatalf("unmarshal surface distribution: %v", err)
	}
	if surfaces["gravel"].DistanceM <= surfaces["asphalt"].DistanceM {
		t.Fatalf("surface distribution = %#v, want gravel to dominate", surfaces)
	}
	var ways map[string]routeProfileTestDistributionEntry
	if err := json.Unmarshal(profile.WayDistribution, &ways); err != nil {
		t.Fatalf("unmarshal way distribution: %v", err)
	}
	if ways["path"].DistanceM <= 0 || ways["track"].DistanceM <= 0 {
		t.Fatalf("way distribution missing expected keys: %#v", ways)
	}

	var projection struct {
		PrimarySurface *string `db:"primary_surface"`
		PrimaryWayType *string `db:"primary_way_type"`
	}
	if err := db.Pool.QueryRow(ctx, `
		SELECT primary_surface, primary_way_type
		FROM route_course_profile_projection
		WHERE subject_kind = 'planned_route_snapshot'
		  AND subject_id = $1
	`, snapshot.ID).Scan(&projection.PrimarySurface, &projection.PrimaryWayType); err != nil {
		t.Fatalf("load projection: %v", err)
	}
	if projection.PrimarySurface == nil || *projection.PrimarySurface != "gravel" {
		t.Fatalf("primary_surface = %v, want gravel", projection.PrimarySurface)
	}
	if projection.PrimaryWayType == nil || *projection.PrimaryWayType != "path" {
		t.Fatalf("primary_way_type = %v, want path", projection.PrimaryWayType)
	}
}

func TestRouteProfileUnknownRangesAndMissingEnrichment(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	snapshot := createRouteProfileTestSnapshot(t, db, userID, nil)

	profile, err := db.CreateRouteProfileSnapshot(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("CreateRouteProfileSnapshot missing enrichment: %v", err)
	}
	if profile.Confidence != "low" {
		t.Fatalf("confidence = %q, want low", profile.Confidence)
	}
	if profile.ForestPct != nil || profile.ProtectedPct != nil {
		t.Fatalf("missing enrichment should keep pct fields nil: forest=%v protected=%v", profile.ForestPct, profile.ProtectedPct)
	}
	var scales map[string]routeProfileTestScale
	if err := json.Unmarshal(profile.DifficultyScales, &scales); err != nil {
		t.Fatalf("unmarshal difficulty scales: %v", err)
	}
	if scales["sac"].MaxLevelRank != nil || scales["mtb"].MaxLevelRank != nil || scales["via_ferrata"].MaxLevelRank != nil {
		t.Fatalf("missing enrichment emitted ranks: %#v", scales)
	}
	if scales["sac"].UnknownDistanceM <= 0 || scales["mtb"].UnknownDistanceM <= 0 || scales["via_ferrata"].UnknownDistanceM <= 0 {
		t.Fatalf("missing enrichment should keep unknown distance explicit: %#v", scales)
	}
}

func createRouteProfileTestSnapshot(t *testing.T, db *DB, userID string, enrichment json.RawMessage) *model.PlannedRouteSnapshot {
	t.Helper()
	subType := "mountain"
	segments := []model.RouteSegment{
		{
			FromWaypoint: "a",
			ToWaypoint:   "b",
			Mode:         "routed",
			Smooth:       false,
			DistanceM:    200,
			Geometry:     &model.GeoJSONLineString{Type: "LineString", Coordinates: [][]float64{{7.000, 47.000}, {7.001, 47.000}, {7.002, 47.000}}},
			SubSegments:  json.RawMessage(`[{"from_idx":0,"to_idx":1,"highway":"track","surface":"asphalt"},{"from_idx":1,"to_idx":2,"highway":"path","surface":"gravel"}]`),
		},
		{
			FromWaypoint: "b",
			ToWaypoint:   "c",
			Mode:         "routed",
			Smooth:       false,
			DistanceM:    200,
			Geometry:     &model.GeoJSONLineString{Type: "LineString", Coordinates: [][]float64{{7.002, 47.000}, {7.003, 47.000}, {7.004, 47.000}}},
			SubSegments:  json.RawMessage(`[{"from_idx":0,"to_idx":2,"highway":"path","surface":"gravel"}]`),
		},
	}
	route := &model.PlannedRoute{
		UserID:          userID,
		Name:            "profile route",
		SportType:       "cycling",
		SubType:         &subType,
		Profile:         "mtb",
		GeometryGeoJSON: `{"type":"LineString","coordinates":[[7.000,47.000],[7.001,47.000],[7.002,47.000],[7.003,47.000],[7.004,47.000]]}`,
		Waypoints: []model.RouteWaypoint{
			{ID: "a", Type: "start", LatLng: [2]float64{47.000, 7.000}},
			{ID: "b", Type: "via", LatLng: [2]float64{47.000, 7.002}},
			{ID: "c", Type: "end", LatLng: [2]float64{47.000, 7.004}},
		},
		Segments:       segments,
		Enrichment:     enrichment,
		TotalDistanceM: routeProfilePtrFloat64(400),
	}
	routeID, err := db.CreateRoute(context.Background(), route)
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	loaded, err := db.GetRoute(context.Background(), routeID, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	snapshot, err := db.CreatePlannedRouteSnapshot(context.Background(), model.CreatePlannedRouteSnapshotInput{
		RouteID:            routeID,
		UserID:             userID,
		SourceRouteVersion: loaded.Version,
		OperationID:        "profile-snapshot-" + routeID,
	})
	if err != nil {
		t.Fatalf("CreatePlannedRouteSnapshot: %v", err)
	}
	return snapshot
}

type routeProfileTestScale struct {
	State             string                                       `json:"state"`
	MaxLevelKey       string                                       `json:"max_level_key"`
	MaxLevelRank      *int                                         `json:"max_level_rank"`
	DominantLevelKey  string                                       `json:"dominant_level_key"`
	DominantLevelRank *int                                         `json:"dominant_level_rank"`
	WeightedMeanRank  *float64                                     `json:"weighted_mean_rank"`
	KnownDistanceM    float64                                      `json:"known_distance_m"`
	UnknownDistanceM  float64                                      `json:"unknown_distance_m"`
	Distribution      map[string]routeProfileTestDistributionEntry `json:"distribution"`
	Ranges            map[string][]routeProfileTestRange           `json:"ranges"`
}

type routeProfileTestDistributionEntry struct {
	DistanceM float64 `json:"distance_m"`
	Rank      *int    `json:"rank,omitempty"`
}

type routeProfileTestRange struct {
	From int `json:"from"`
	To   int `json:"to"`
}

func routeProfilePtrFloat64(v float64) *float64 {
	return &v
}
