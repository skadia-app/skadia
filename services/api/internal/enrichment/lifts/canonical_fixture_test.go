package lifts

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalFixtureSmoke(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "*.canonical.json"))
	if err != nil {
		t.Fatalf("glob canonical fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no canonical fixtures found")
	}
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), ".canonical.json")
		t.Run(name, func(t *testing.T) {
			fixture := loadCanonicalFixture(t, name)
			pts := fixture.Points
			gt := fixture.GT
			rides := DetectHeuristic(pts, DefaultDetectorConfig("ski"))
			if len(rides) != gt.RidesExpected {
				t.Fatalf("DetectHeuristic returned %d rides, want %d", len(rides), gt.RidesExpected)
			}
			for i, ride := range rides {
				assertRideWithinTolerance(t, ride, gt.Rides[i], gt.Anchors, fixture.Fixture.Samples)
			}
		})
	}
}

func TestCanonicalFixtureContentHashStable(t *testing.T) {
	fixture := loadCanonicalFixture(t, "slow_chairlift_a")
	gt := fixture.GT
	if gt.FixtureContentHash == "" || gt.FixtureContentHash == "pending" {
		t.Fatalf("fixture_content_hash must be promoted, got %q", gt.FixtureContentHash)
	}
	if gt.DetectorVerdictHash == "" || gt.DetectorVerdictHash == "pending" {
		t.Fatalf("detector_verdict_hash must be promoted, got %q", gt.DetectorVerdictHash)
	}
}

func TestSynthCableTrackAdmitsCableLockStep(t *testing.T) {
	pts := synthCableTrack(t, synthSpec{
		durationS:    420,
		meanSpeedMs:  2.2,
		altRateMs:    0.32,
		sinuosity:    1.005,
		speedCV:      0.02,
		sampleRateHz: 1,
	})
	rides := DetectHeuristic(pts, DefaultDetectorConfig("ski"))
	if len(rides) != 1 {
		t.Fatalf("DetectHeuristic returned %d rides, want 1", len(rides))
	}
}

func TestCanonicalFixtureMidRidePromotionDistanceGate(t *testing.T) {
	fixture := loadCanonicalFixture(t, "device_sleep_mid_ride")
	pts := fixture.Points
	gt := fixture.GT
	rides := DetectHeuristic(pts, DefaultDetectorConfig("ski"))
	if len(rides) != gt.RidesExpected {
		t.Fatalf("DetectHeuristic returned %d rides, want %d", len(rides), gt.RidesExpected)
	}
	assertRideWithinTolerance(t, rides[0], gt.Rides[0], gt.Anchors, fixture.Fixture.Samples)

	shifted := rides[0]
	shifted.StartedAt = pts[5].Time
	if err := checkRideWithinTolerance(shifted, gt.Rides[0], gt.Anchors, fixture.Fixture.Samples); err == nil {
		t.Fatal("shifted mid-ride boundary stayed inside promotion-distance growth tolerance")
	}
}
