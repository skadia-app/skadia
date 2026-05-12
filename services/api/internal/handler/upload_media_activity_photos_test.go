package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUploadMediaAutoAssignUsesActivityPhotoDedupPipeline(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	activityA := insertMinimalActivity(t, db, userID)
	activityB := insertMinimalActivity(t, db, userID)
	setHandlerPhotoPhashThreshold(t, pool, userID, 8)

	fs := newActivityPhotoStorageTestFS(t, pool)
	h := &Handler{
		DB:            db,
		ShadowFS:      fs,
		ImportService: importer.NewService(db, fs, nil),
	}

	first := jpegPatternTestImage(t, 48, 48, 95)
	created, err := h.ImportService.ProcessPhoto(context.Background(), userID, activityA, first, importer.PhotoMetadata{
		OriginalFilename: "activity-a.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto created: %v", err)
	}
	if created.Status != "created" {
		t.Fatalf("first ProcessPhoto status = %q, want created", created.Status)
	}

	second := pngTestImage(t, 48, 48)
	title := "second activity"
	takenAt := time.Now().UTC().Truncate(time.Second)
	lat, lng := 47.1, 11.2
	source := "exif"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload/media", nil)
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))

	result := h.autoAssignPhoto(req, userID, "activity-b.png", "image/png", second, database.ActivityTimeMatch{
		ID:    activityB,
		Title: &title,
	}, &takenAt, &lat, &lng, &source)

	if result.Status != "duplicate" {
		t.Fatalf("autoAssignPhoto status = %q, want duplicate", result.Status)
	}
	if got := handlerActiveMediaCount(t, pool, userID); got != 1 {
		t.Fatalf("active media user_files after auto-assign duplicate = %d, want 1", got)
	}
	if got := handlerActivityPhotoRowCount(t, pool, userID); got != 1 {
		t.Fatalf("activity_photos rows after auto-assign duplicate = %d, want 1", got)
	}
}

func TestMediaPhotoQueueMetadataPreservesExifFields(t *testing.T) {
	takenAt := time.Date(2026, 5, 12, 15, 30, 0, 123, time.UTC)
	lat, lng := 47.1, 11.2
	source := "exif"

	meta := mediaPhotoQueueMetadata(&takenAt, &lat, &lng, &source, map[string]any{
		"candidate_ids": []string{"a", "b"},
	})

	if meta["taken_at"] != takenAt.Format(time.RFC3339Nano) {
		t.Fatalf("taken_at metadata = %v, want %s", meta["taken_at"], takenAt.Format(time.RFC3339Nano))
	}
	if meta["lat"] != lat || meta["lng"] != lng {
		t.Fatalf("gps metadata = (%v,%v), want (%f,%f)", meta["lat"], meta["lng"], lat, lng)
	}
	if meta["location_source"] != "exif" {
		t.Fatalf("location_source metadata = %v, want exif", meta["location_source"])
	}
	if meta["candidate_ids"] == nil {
		t.Fatal("candidate_ids metadata was not preserved")
	}
}

func setHandlerPhotoPhashThreshold(t *testing.T, pool *pgxpool.Pool, userID string, threshold int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO user_enrichment_settings (user_id, phash_threshold_bits)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET phash_threshold_bits = EXCLUDED.phash_threshold_bits`,
		userID, threshold,
	); err != nil {
		t.Fatalf("set phash threshold: %v", err)
	}
}

func handlerActiveMediaCount(t *testing.T, pool *pgxpool.Pool, userID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM user_files
		WHERE user_id = $1 AND file_type = 'media' AND status = 'active'`,
		userID,
	).Scan(&count); err != nil {
		t.Fatalf("count active media: %v", err)
	}
	return count
}

func handlerActivityPhotoRowCount(t *testing.T, pool *pgxpool.Pool, userID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM activity_photos
		WHERE user_id = $1`,
		userID,
	).Scan(&count); err != nil {
		t.Fatalf("count activity_photos: %v", err)
	}
	return count
}
