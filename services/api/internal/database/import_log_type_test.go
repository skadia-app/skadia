package database

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

func importTypeTestDB(t *testing.T) (*DB, string) {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	var userID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test') RETURNING id`,
		"import-type-"+suffix, "import-type-"+suffix+"@test.localhost",
	).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return &DB{Pool: pool}, userID
}

func TestListImportLog_ImportTypeFilter(t *testing.T) {
	db, userID := importTypeTestDB(t)
	ctx := context.Background()

	cases := []struct {
		filename string
		typ      string
		status   string
	}{
		{"run.fit", "activity", "completed"},
		{"course.fit", "route", "completed"},
		{"workout.fit", "workout", "completed"},
		{"sleep.json", "monitoring", "completed"},
		{"img.jpg", "photo", "queued"},
		{"clip.mp4", "video", "queued"},
		{"manual.pdf", "document", "queued"},
		{"bike.json", "gear", "completed"},
		{"segment.fit", "segment", "completed"},
		{"export.zip", "archive", "completed"},
	}
	insertedIDs := make(map[string]string)
	for _, c := range cases {
		typ := c.typ
		id, err := db.InsertImportLog(ctx, model.ImportLogEntry{
			UserID: userID, SourcePlatform: "upload", Filename: c.filename,
			Status: c.status, ImportType: &typ,
		})
		if err != nil {
			t.Fatalf("InsertImportLog %s: %v", c.typ, err)
		}
		insertedIDs[c.typ] = id
	}

	if _, err := db.InsertImportLog(ctx, model.ImportLogEntry{
		UserID: userID, SourcePlatform: "garmin", Filename: "old.fit",
		Status: "completed",
	}); err != nil {
		t.Fatalf("InsertImportLog null: %v", err)
	}

	for _, c := range cases {
		entries, total, err := db.ListImportLog(ctx, userID, nil, "", c.typ, 10, 0)
		if err != nil {
			t.Fatalf("ListImportLog %s: %v", c.typ, err)
		}
		if total != 1 || len(entries) != 1 || entries[0].ID != insertedIDs[c.typ] {
			t.Fatalf("filter %s: want 1 id=%s, got total=%d", c.typ, insertedIDs[c.typ], total)
		}
		if entries[0].ImportType == nil || *entries[0].ImportType != c.typ {
			t.Fatalf("filter %s: import_type mismatch: %v", c.typ, entries[0].ImportType)
		}
	}

	queuedPhoto, queuedTotal, err := db.ListImportLog(ctx, userID, []string{"queued"}, "", "photo", 10, 0)
	if err != nil {
		t.Fatalf("queued+photo: %v", err)
	}
	if queuedTotal != 1 || queuedPhoto[0].ID != insertedIDs["photo"] {
		t.Fatalf("queued+photo: want 1 id=%s, got %d", insertedIDs["photo"], queuedTotal)
	}

	_, unknownTotal, err := db.ListImportLog(ctx, userID, nil, "", "unknown", 10, 0)
	if err != nil {
		t.Fatalf("unknown: %v", err)
	}
	if unknownTotal != 1 {
		t.Fatalf("unknown: want 1 entry, got %d", unknownTotal)
	}

	_, allTotal, err := db.ListImportLog(ctx, userID, nil, "", "", 20, 0)
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if allTotal != 11 {
		t.Fatalf("no filter: want 11, got %d", allTotal)
	}
}

func TestImportType_Constants(t *testing.T) {
	for _, want := range []string{
		model.ImportTypeActivity, model.ImportTypeRoute, model.ImportTypeWorkout,
		model.ImportTypeMonitoring, model.ImportTypePhoto, model.ImportTypeVideo,
		model.ImportTypeDocument, model.ImportTypeGear, model.ImportTypeSegment,
		model.ImportTypeArchive,
	} {
		if want == "" {
			t.Fatalf("import type constant is empty")
		}
		if !model.IsValidImportType(want) {
			t.Fatalf("IsValidImportType(%q) = false, want true", want)
		}
	}
	if model.IsValidImportType("foto") {
		t.Fatalf("IsValidImportType(\"foto\") should be false (typo)")
	}
}
