package database

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestGetActivityHasGPSFromColumn is a regression guard for the self-fulfilling
// prophecy bug fixed in commit a18e414d (2026-04-19):
//
// GetActivity used to project `COALESCE(activity_tracks.point_count, 0) > 0`
// into a local hasTrack var and then set `a.HasGPS = hasTrack`, silently
// overriding the activities.has_gps column. That caused postCommitFusionCleanup
// to skip EnsureActivityTrack for freshly-merged activities (freshMerged.HasGPS
// came back as false because the track was not built yet), which meant the
// track was never built — HasGPS stayed false forever, streams existed, map
// stayed blank.
//
// The fix selects a.has_gps directly. This test fails if that regresses.
//
// Checks performed against activities_db.go source:
//  1. GetActivity's SELECT must contain `a.has_gps` (direct column read).
//  2. GetActivity's SELECT must NOT contain `point_count` (activity_tracks proxy).
//  3. GetActivity's body must NOT re-assign a.HasGPS = <anything> (override).
func TestGetActivityHasGPSFromColumn(t *testing.T) {
	src, err := os.ReadFile("activities_db.go")
	if err != nil {
		t.Fatalf("cannot read activities_db.go: %v", err)
	}
	content := string(src)

	// Locate GetActivity function body: from signature to next top-level `^}` line.
	const funcMarker = "func (db *DB) GetActivity(ctx context.Context, id string) (*model.Activity, error) {"
	funcStart := strings.Index(content, funcMarker)
	if funcStart < 0 {
		t.Fatal("GetActivity signature not found in activities_db.go — test marker drifted?")
	}

	// End-of-function: next line starting with `}` on its own at column 0.
	endRe := regexp.MustCompile(`(?m)^\}$`)
	rel := endRe.FindStringIndex(content[funcStart:])
	if rel == nil {
		t.Fatal("end of GetActivity function body not found")
	}
	body := content[funcStart : funcStart+rel[1]]

	// 1. Must select a.has_gps directly.
	if !strings.Contains(body, "a.has_gps") {
		t.Errorf("GetActivity must SELECT a.has_gps directly from activities — not found.\n"+
			"If you removed it, restore the column read or the API will silently lie about GPS state.\n"+
			"Body excerpt (first 400 chars): %s", body[:min(400, len(body))])
	}

	// 2. Must NOT use the activity_tracks.point_count proxy for has_gps.
	if strings.Contains(body, "point_count") {
		t.Errorf("GetActivity must not derive has_gps from activity_tracks.point_count.\n" +
			"That created a self-fulfilling prophecy where fresh merges had HasGPS=false " +
			"until the track rebuild ran — but the rebuild was gated on HasGPS. See commit a18e414d.")
	}

	// 3. Must not override a.HasGPS after Scan (e.g. `a.HasGPS = hasTrack`).
	overrideRe := regexp.MustCompile(`\ba\.HasGPS\s*=\s*\w`)
	if overrideRe.MatchString(body) {
		t.Errorf("GetActivity must not reassign a.HasGPS after Scan.\n" +
			"a.HasGPS should come from the activities.has_gps column via Scan — nothing else.")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
