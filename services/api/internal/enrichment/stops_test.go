package enrichment

import (
	"testing"
	"time"
)

func TestDetectPOIStops_BasicStop(t *testing.T) {
	// Wanderer steht 5 Min am Gipfel (speed ~0, confined)
	now := time.Now()
	points := make([]streamPoint, 300)
	for i := range 300 {
		points[i] = streamPoint{
			T:       now.Add(time.Duration(i) * time.Second),
			Lat:     47.5 + float64(i%3)*0.00001, // tiny drift
			Lng:     11.5 + float64(i%2)*0.00001,
			SpeedMS: 0.2,
		}
	}
	pois := []poiCandidate{{
		OsmID: 1, Name: "Gipfel", Category: "summit", Subtype: "peak",
		Lat: 47.5, Lng: 11.5, Config: poiThresholds["summit"],
	}}

	stops := detectPOIStops(points, pois)
	if len(stops) != 1 {
		t.Fatalf("got %d stops, want 1", len(stops))
	}
	if stops[0].DurationS < 290 {
		t.Errorf("duration: got %d, want ~300", stops[0].DurationS)
	}
}

func TestDetectPOIStops_FastReject(t *testing.T) {
	// Radfahrer faehrt bei 25 km/h (~7 m/s) am Restaurant vorbei -> kein Stopp
	now := time.Now()
	points := make([]streamPoint, 50)
	for i := range 50 {
		points[i] = streamPoint{
			T:       now.Add(time.Duration(i) * time.Second),
			Lat:     47.5 + float64(i)*0.00005,
			Lng:     11.5,
			SpeedMS: 7.0,
		}
	}
	pois := []poiCandidate{{
		OsmID: 2, Name: "Restaurant", Category: "gastronomy", Subtype: "restaurant",
		Lat: 47.5012, Lng: 11.5, Config: poiThresholds["gastronomy"],
	}}

	stops := detectPOIStops(points, pois)
	if len(stops) != 0 {
		t.Fatalf("got %d stops, want 0 (fast-reject)", len(stops))
	}
}

func TestDetectPOIStops_WalkPastNoStop(t *testing.T) {
	// Fussgaenger laeuft bei 1.2 m/s durch Fussgaengerzone an Restaurant vorbei
	// Spatial spread ist hoch (directional movement) -> kein Stopp
	now := time.Now()
	points := make([]streamPoint, 100)
	for i := range 100 {
		points[i] = streamPoint{
			T:       now.Add(time.Duration(i) * time.Second),
			Lat:     47.5 + float64(i)*0.000011, // ~1.2 m/s northward
			Lng:     11.5,
			SpeedMS: 1.2,
		}
	}
	pois := []poiCandidate{{
		OsmID: 3, Name: "Cafe", Category: "gastronomy", Subtype: "cafe",
		Lat: 47.5005, Lng: 11.5, Config: poiThresholds["gastronomy"],
	}}

	stops := detectPOIStops(points, pois)
	if len(stops) != 0 {
		t.Fatalf("got %d stops, want 0 (walking past, high spatial spread)", len(stops))
	}
}

func TestDetectPOIStops_GondolaShortDwell(t *testing.T) {
	// Bikepark Gondel: 30s an Bergstation -> unter MinDwell fuer transport (60s) -> KEIN Stopp
	now := time.Now()
	points := make([]streamPoint, 30)
	for i := range 30 {
		points[i] = streamPoint{
			T:       now.Add(time.Duration(i) * time.Second),
			Lat:     47.3 + float64(i%2)*0.000005,
			Lng:     12.4,
			SpeedMS: 0.1,
		}
	}
	pois := []poiCandidate{{
		OsmID: 4, Name: "Bergstation", Category: "transport", Subtype: "station",
		Lat: 47.3, Lng: 12.4, Config: poiThresholds["transport"],
	}}
	stops := detectPOIStops(points, pois)
	if len(stops) != 0 {
		t.Fatalf("got %d stops, want 0 (30s < 60s min dwell for transport)", len(stops))
	}
}

func TestDetectPOIStops_TwoRestaurantsDedup(t *testing.T) {
	// 45 Min Aufenthalt, 2 Restaurants 20m entfernt -> nur das naehere behalten
	now := time.Now()
	points := make([]streamPoint, 2700) // 45 min
	for i := range 2700 {
		points[i] = streamPoint{
			T:       now.Add(time.Duration(i) * time.Second),
			Lat:     47.5 + float64(i%3)*0.000003,
			Lng:     11.5 + float64(i%2)*0.000003,
			SpeedMS: 0.1,
		}
	}
	poiA := poiCandidate{
		OsmID: 5, Name: "Restaurant A", Category: "gastronomy", Subtype: "restaurant",
		Lat: 47.50001, Lng: 11.50001, Config: poiThresholds["gastronomy"],
	}
	poiB := poiCandidate{
		OsmID: 6, Name: "Restaurant B", Category: "gastronomy", Subtype: "restaurant",
		Lat: 47.50020, Lng: 11.50020, Config: poiThresholds["gastronomy"], // ~20m weiter
	}
	stops := detectPOIStops(points, []poiCandidate{poiA, poiB})
	if len(stops) != 1 {
		t.Fatalf("got %d stops, want 1 (dedup: closer restaurant wins)", len(stops))
	}
	if stops[0].POI.OsmID != 5 {
		t.Errorf("expected Restaurant A (closer), got OsmID %d", stops[0].POI.OsmID)
	}
}

func TestDetectPOIStops_SpeedFallbackPicnic(t *testing.T) {
	// Kein POI in der Naehe, aber 5 Min stationaer -> Speed-Fallback
	now := time.Now()
	points := make([]streamPoint, 400)
	for i := range 400 {
		speed := 3.0 // walking
		if i >= 100 && i < 400 {
			speed = 0.1 // stopped for 5 min (300s)
		}
		points[i] = streamPoint{
			T:       now.Add(time.Duration(i) * time.Second),
			Lat:     47.5 + float64(i%2)*0.000002,
			Lng:     11.5,
			SpeedMS: speed,
		}
	}
	// No POIs -> detectPOIStops returns empty
	poiStops := detectPOIStops(points, nil)
	if len(poiStops) != 0 {
		t.Fatalf("POI stops: got %d, want 0 (no POIs)", len(poiStops))
	}

	// Speed fallback should detect the 300s stop
	fallbackStops := detectSpeedFallbackStops(points)
	if len(fallbackStops) != 1 {
		t.Fatalf("fallback stops: got %d, want 1", len(fallbackStops))
	}
	if fallbackStops[0].DurationS < 280 {
		t.Errorf("duration: got %d, want ~300", fallbackStops[0].DurationS)
	}
	if fallbackStops[0].POI != nil {
		t.Error("fallback stop should have nil POI")
	}
}

func TestEquirectDist(t *testing.T) {
	// ~111m between 47.5 and 47.501 latitude
	d := equirectDist(47.5, 11.5, 47.501, 11.5)
	if d < 100 || d > 120 {
		t.Errorf("got %.1f, want ~111m", d)
	}
}
