package handler

import (
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestBuildRouteDraftFromActivityPreservesTrackGeometry(t *testing.T) {
	title := "Recorded loop"
	distance := 1234.0
	ascent := 56.0
	descent := 34.0
	activity := &model.Activity{
		ID:                     "activity-1",
		UserID:                 "user-1",
		Title:                  &title,
		SportType:              "hiking",
		HasGPS:                 true,
		DistanceM:              &distance,
		ElevationGainM:         &ascent,
		ElevationLossM:         &descent,
		StartedAt:              time.Date(2026, 5, 17, 8, 0, 0, 0, time.UTC),
		EnrichmentStatus:       "done",
		DistanceSource:         "stream",
		ElevationSource:        "stream",
		ExcludeLiftSubtraction: true,
	}
	points := []database.GPSStreamPoint{
		{Lat: 49.0, Lng: 12.0},
		{Lat: 49.01, Lng: 12.01},
		{Lat: 49.0, Lng: 12.02},
		{Lat: 49.0, Lng: 12.0},
	}

	route, err := buildRouteDraftFromActivity(activity, "user-1", points)
	if err != nil {
		t.Fatalf("buildRouteDraftFromActivity: %v", err)
	}

	if route.ID != "" {
		t.Fatalf("route.ID = %q, want empty draft id", route.ID)
	}
	if route.Name != "Route: Recorded loop" {
		t.Fatalf("route.Name = %q", route.Name)
	}
	if len(route.Waypoints) != 2 {
		t.Fatalf("waypoints len = %d, want 2", len(route.Waypoints))
	}
	if len(route.Segments) != 1 {
		t.Fatalf("segments len = %d, want 1", len(route.Segments))
	}
	seg := route.Segments[0]
	if seg.Geometry == nil {
		t.Fatal("segment geometry is nil")
	}
	if got := len(seg.Geometry.Coordinates); got != len(points) {
		t.Fatalf("segment geometry coords = %d, want %d", got, len(points))
	}
	if seg.Geometry.Coordinates[1][0] != 12.01 || seg.Geometry.Coordinates[1][1] != 49.01 {
		t.Fatalf("segment geometry coord[1] = %#v", seg.Geometry.Coordinates[1])
	}
}
