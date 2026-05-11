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
	if data.FileHash != "" || data.FileFormat != "" {
		t.Fatalf("buildStravaCSVSourceFile mutated data file metadata: hash %q format %q", data.FileHash, data.FileFormat)
	}
}

func TestStravaActivityGearNameReadsLocalizedAndEnglishColumns(t *testing.T) {
	germanHeader := map[string]int{"Ausrüstung": 1}
	if got := stravaActivityGearName([]string{"Morning Ride", "Propain Tyee 2020"}, germanHeader); got != "Propain Tyee 2020" {
		t.Fatalf("German gear name = %q, want Propain Tyee 2020", got)
	}
	englishHeader := map[string]int{"Activity Gear": 0}
	if got := stravaActivityGearName([]string{"ASICS GEL-Kayano 2011"}, englishHeader); got != "ASICS GEL-Kayano 2011" {
		t.Fatalf("English gear name = %q, want ASICS GEL-Kayano 2011", got)
	}
}

func TestBuildStravaCSVSourceFileHashIgnoresCanonicalSportMapping(t *testing.T) {
	startedAt := time.Date(2026, 5, 3, 9, 8, 0, 0, time.UTC)
	hdr := map[string]int{"Name": 0, "Activity ID": 1}
	row := []string{"Bouldern", "123"}
	oldMapping := &model.ActivityData{SportType: "generic", StartedAt: startedAt}
	newMapping := &model.ActivityData{SportType: "rock_climbing", SubType: "bouldering", StartedAt: startedAt}

	oldFile, err := buildStravaCSVSourceFile("user-1", 123, oldMapping, "Bouldern", row, hdr)
	if err != nil {
		t.Fatalf("old mapping buildStravaCSVSourceFile: %v", err)
	}
	newFile, err := buildStravaCSVSourceFile("user-1", 123, newMapping, "Bouldern", row, hdr)
	if err != nil {
		t.Fatalf("new mapping buildStravaCSVSourceFile: %v", err)
	}
	if oldFile.hash != newFile.hash {
		t.Fatalf("hash changed across canonical sport remap: %q != %q", oldFile.hash, newFile.hash)
	}
}

func TestBuildStravaCSVSourceFileIsDeterministic(t *testing.T) {
	startedAt := time.Date(2026, 5, 3, 9, 8, 0, 0, time.UTC)
	hdr := map[string]int{"Name": 0, "Activity ID": 1}
	row := []string{"Morning MTB", "123"}
	data := &model.ActivityData{
		SportType:      "cycling",
		SubType:        "mountain",
		StartedAt:      startedAt,
		DistanceM:      12345,
		ElapsedTimeS:   3661,
		ElevationGainM: 321,
	}

	first, err := buildStravaCSVSourceFile("user-1", 123, data, "Morning MTB", row, hdr)
	if err != nil {
		t.Fatalf("first buildStravaCSVSourceFile: %v", err)
	}
	second, err := buildStravaCSVSourceFile("user-1", 123, data, "Morning MTB", row, hdr)
	if err != nil {
		t.Fatalf("second buildStravaCSVSourceFile: %v", err)
	}
	if first.hash != second.hash {
		t.Fatalf("hash changed between identical builds: %q != %q", first.hash, second.hash)
	}
	if string(first.data) != string(second.data) {
		t.Fatalf("CSV sidecar bytes changed between identical builds:\nfirst=%s\nsecond=%s", first.data, second.data)
	}
}

func TestMergeActivityDataPreservesCommuteEvidence(t *testing.T) {
	base := &model.ActivityData{SportType: "cycling"}
	parsed := &model.ActivityData{SportType: "cycling", IsCommute: true}

	mergeActivityData(base, parsed)

	if !base.IsCommute {
		t.Fatal("IsCommute = false, want true from parsed activity data")
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
