package dataimport

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestLoadActivityFileTagIndexWarnsAndContinuesForBadRows(t *testing.T) {
	duplicateID := uuid.New().String()
	emptyID := "   "
	zipPath := buildExportZip(t, zipBuildOpts{
		activities: []map[string]any{
			{"id": uuid.New().String(), "file_id": duplicateID, "sport_type": "running"},
			{"id": uuid.New().String(), "file_id": duplicateID, "sport_type": "cycling"},
			{"id": uuid.New().String(), "file_id": emptyID, "sport_type": "walking"},
			{"id": uuid.New().String(), "sport_type": "hiking"},
		},
	})
	src := openZipSource(t, zipPath)

	index, warnings, err := loadActivityFileTagIndex(src)
	if err != nil {
		t.Fatalf("loadActivityFileTagIndex: %v", err)
	}
	if len(index) != 1 {
		t.Fatalf("index size = %d, want 1", len(index))
	}
	if got := index[duplicateID].SportType; got != "running" {
		t.Fatalf("duplicate file_id should keep first row, got sport_type=%q", got)
	}
	if !containsWarning(warnings, "duplicate file_id") {
		t.Fatalf("warnings should mention duplicate file_id, got %v", warnings)
	}
	if !containsWarning(warnings, "blank file_id") {
		t.Fatalf("warnings should mention blank file_id, got %v", warnings)
	}
}

func TestLoadActivityFileTagIndexMalformedJSONReturnsWarning(t *testing.T) {
	zipPath := buildExportZip(t, zipBuildOpts{
		extraFiles: map[string][]byte{"data/activities.json": []byte(`{"not":"an array"}`)},
	})
	src := openZipSource(t, zipPath)

	index, warnings, err := loadActivityFileTagIndex(src)
	if err != nil {
		t.Fatalf("loadActivityFileTagIndex should degrade to warnings: %v", err)
	}
	if len(index) != 0 {
		t.Fatalf("index size = %d, want 0", len(index))
	}
	if !containsWarning(warnings, "activity file tag index skipped") {
		t.Fatalf("warnings should mention skipped activity file tag index, got %v", warnings)
	}
}

func TestImportFileByIDWarnsWhenBlobIsMissing(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)
	fileOldID := uuid.New().String()
	userFilesJSON := mustMarshalJSON(t, []map[string]any{{
		"id":           fileOldID,
		"file_type":    "activity",
		"content_type": "application/octet-stream",
		"file_name":    "missing.fit",
		"size_bytes":   4,
		"status":       "active",
	}})
	zipPath := buildExportZip(t, zipBuildOpts{
		extraFiles: map[string][]byte{"data/user_files.json": userFilesJSON},
	})
	src := openZipSource(t, zipPath)
	eng := &Engine{DB: pool, ShadowFS: newDBMockShadowFS(pool), Config: EngineConfig{}}

	result, err := eng.RunImport(context.Background(), ImportInput{Source: src, TargetUserID: userID})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}
	if result.FilesImported != 0 {
		t.Fatalf("FilesImported = %d, want 0", result.FilesImported)
	}
	if !containsWarning(result.Warnings, "file blob not found") {
		t.Fatalf("warnings should mention missing file blob, got %v", result.Warnings)
	}
}

func containsWarning(warnings []string, needle string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, needle) {
			return true
		}
	}
	return false
}

func TestBuildExportZipAllowsMalformedActivitiesFixture(t *testing.T) {
	raw := []byte(`{"not":"an array"}`)
	zipPath := buildExportZip(t, zipBuildOpts{
		extraFiles: map[string][]byte{"data/activities.json": raw},
	})
	src := openZipSource(t, zipPath)

	rc, err := src.OpenData("data/activities.json")
	if err != nil {
		t.Fatalf("OpenData: %v", err)
	}
	defer rc.Close()
	var decoded map[string]string
	if err := json.NewDecoder(rc).Decode(&decoded); err != nil {
		t.Fatalf("decode malformed fixture as object: %v", err)
	}
	if decoded["not"] != "an array" {
		t.Fatalf("fixture was not preserved: %#v", decoded)
	}
}
