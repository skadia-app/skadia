package strava

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestBuildStravaCSVSourceFileUsesCanonicalActivityTags(t *testing.T) {
	startedAt := time.Date(2026, 5, 3, 9, 8, 0, 0, time.UTC)
	data := &model.ActivityData{
		SportType:      "cycling",
		SubType:        "mountain",
		StartedAt:      startedAt,
		DistanceM:      12345,
		ElapsedTimeS:   3661,
		ElevationGainM: 321,
	}
	hdr := map[string]int{"Name": 0, "Activity ID": 1}
	row := []string{"Morning MTB", "123"}

	file, err := buildStravaCSVSourceFile("user-1", 123, data, "Morning MTB", row, hdr)
	if err != nil {
		t.Fatalf("buildStravaCSVSourceFile: %v", err)
	}

	if file.entry.FileName != "strava_123.json" {
		t.Fatalf("FileName = %q, want strava_123.json", file.entry.FileName)
	}
	if file.entry.Tags["format"] != "strava_csv" {
		t.Fatalf("format tag = %q, want strava_csv", file.entry.Tags["format"])
	}
	if file.entry.Tags["source"] != "strava" {
		t.Fatalf("source tag = %q, want strava", file.entry.Tags["source"])
	}
	if file.entry.Tags["time"] != "0908" {
		t.Fatalf("time tag = %q, want 0908", file.entry.Tags["time"])
	}
	if file.entry.Tags["sport"] != "cycling" || file.entry.Tags["subCategory"] != "mountain" {
		t.Fatalf("taxonomy tags = %#v, want cycling/mountain", file.entry.Tags)
	}
	if data.FileHash != file.hash || data.FileFormat != "strava_csv" {
		t.Fatalf("data file metadata = hash %q format %q, want %q/strava_csv", data.FileHash, data.FileFormat, file.hash)
	}
}

func TestStravaCSVOnlyShadowFSWriteHappensAfterSkippedBranch(t *testing.T) {
	src, err := os.ReadFile("importer_activities.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	skipIdx := strings.Index(text, `if actResult.Status == "skipped"`)
	putIdx := strings.Index(text, "putStravaCSVSourceFile(")
	if skipIdx < 0 || putIdx < 0 {
		t.Fatalf("required guard anchors missing: skip=%d put=%d", skipIdx, putIdx)
	}
	if putIdx < skipIdx {
		t.Fatalf("CSV-only ShadowFS write occurs before skipped-duplicate guard")
	}
}
