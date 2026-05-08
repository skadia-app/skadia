package dataimport

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sport"
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

func TestLoadActivityFileTagIndexUsesAutoTitleFallback(t *testing.T) {
	fileID := uuid.New().String()
	zipPath := buildExportZip(t, zipBuildOpts{
		activities: []map[string]any{
			{
				"id":         uuid.New().String(),
				"file_id":    fileID,
				"sport_type": "running",
				"title":      "",
				"title_auto": "Automatischer Lauf",
			},
		},
	})
	src := openZipSource(t, zipPath)

	index, warnings, err := loadActivityFileTagIndex(src)
	if err != nil {
		t.Fatalf("loadActivityFileTagIndex: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if got := index[fileID].Title; got != "Automatischer Lauf" {
		t.Fatalf("title = %q, want title_auto fallback", got)
	}
}

func TestLoadActivityFileTagIndexPreservesGeneratedActivityStateModifiers(t *testing.T) {
	fileID := uuid.New().String()
	row := map[string]any{
		"id":         uuid.New().String(),
		"file_id":    fileID,
		"sport_type": "cycling",
	}
	for _, key := range sport.SportModifierKeysWithCapability("activity_state") {
		modifier, ok := sport.SportModifierByKey(key)
		if !ok {
			t.Fatalf("missing modifier %s", key)
		}
		row[modifier.JSONKey] = true
	}
	zipPath := buildExportZip(t, zipBuildOpts{
		activities: []map[string]any{row},
	})
	src := openZipSource(t, zipPath)

	index, warnings, err := loadActivityFileTagIndex(src)
	if err != nil {
		t.Fatalf("loadActivityFileTagIndex: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	input, ok := index[fileID]
	if !ok {
		t.Fatalf("missing activity file tag input for %s", fileID)
	}
	value := reflect.ValueOf(input)
	for _, key := range sport.SportModifierKeysWithCapability("activity_state") {
		modifier, ok := sport.SportModifierByKey(key)
		if !ok {
			t.Fatalf("missing modifier %s", key)
		}
		field := value.FieldByName(modifier.GoField)
		if !field.IsValid() || field.Kind() != reflect.Bool {
			t.Fatalf("ActivitySchemaTagsInput.%s missing bool field for %s", modifier.GoField, key)
		}
		if !field.Bool() {
			t.Fatalf("ActivitySchemaTagsInput.%s = false, want true for %s", modifier.GoField, key)
		}
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
