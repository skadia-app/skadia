package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
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

func TestUploadMediaAutoAssignSkipsWhenActivityPhotoLimitReached(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	activityID := insertMinimalActivity(t, db, userID)
	store := settings.NewStoreForTest(map[string]string{"limits.max_activity_photos_per_activity": "1"})
	fs := newActivityPhotoStorageTestFS(t, pool)
	svc := importer.NewService(db, fs, nil)
	svc.ActivityPhotoCreateGuard = NewActivityPhotoCreateGuard(db, store)
	h := &Handler{
		DB:            db,
		ShadowFS:      fs,
		ImportService: svc,
		Settings:      store,
	}

	hash := "existing-limit-row"
	actID := activityID
	if _, err := db.InsertActivityPhoto(context.Background(), model.ActivityPhoto{
		UserID:           userID,
		ActivityID:       &actID,
		OriginalFilename: "existing.jpg",
		ContentType:      "image/jpeg",
		FileSizeBytes:    1,
		FileHash:         &hash,
	}); err != nil {
		t.Fatalf("insert existing activity photo: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload/media", nil)
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	result := h.autoAssignPhoto(req, userID, "new.jpg", "image/jpeg", jpegPatternTestImage(t, 48, 48, 90), database.ActivityTimeMatch{
		ID: activityID,
	}, nil, nil, nil, nil)

	if result.Status != "skipped_limit_reached" {
		t.Fatalf("autoAssignPhoto status = %q, want skipped_limit_reached", result.Status)
	}
	if result.ErrorCode == nil || *result.ErrorCode != codeActivityPhotoLimitReached {
		t.Fatalf("autoAssignPhoto error_code = %v, want %q", result.ErrorCode, codeActivityPhotoLimitReached)
	}
	if got := handlerActivityPhotoRowCount(t, pool, userID); got != 1 {
		t.Fatalf("activity photo rows after skipped media upload = %d, want 1", got)
	}
}

func TestUploadMediaAutoAssignReportsStorageQuotaExceededStatus(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	activityID := insertMinimalActivity(t, db, userID)
	dir := t.TempDir()
	fs := shadowfs.NewUnifiedShadowFS(pool, shadowfs.NewLocalBackend(dir), nil, "local", dir, shadowfs.NewSchemaRenderer(), nil, 1, 1, "", "", "")
	h := &Handler{
		DB:            db,
		ShadowFS:      fs,
		ImportService: importer.NewService(db, fs, nil),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload/media", nil)
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	result := h.autoAssignPhoto(req, userID, "quota.jpg", "image/jpeg", jpegPatternTestImage(t, 48, 48, 90), database.ActivityTimeMatch{
		ID: activityID,
	}, nil, nil, nil, nil)

	if result.Status != "storage_quota_exceeded" {
		t.Fatalf("autoAssignPhoto status = %q, want storage_quota_exceeded", result.Status)
	}
	if result.ErrorCode == nil || *result.ErrorCode != codeStorageQuotaExceeded {
		t.Fatalf("autoAssignPhoto error_code = %v, want %q", result.ErrorCode, codeStorageQuotaExceeded)
	}
}

func TestUploadMediaUsesConfiguredUploadLimit(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	h := &Handler{
		DB:       db,
		ShadowFS: newActivityPhotoStorageTestFS(t, pool),
		Settings: settings.NewStoreForTest(map[string]string{
			"limits.max_upload_bytes": "128",
		}),
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("files", "too-large.jpg")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(bytes.Repeat([]byte("x"), 512)); err != nil {
		t.Fatalf("write upload body: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload/media", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rr := httptest.NewRecorder()

	h.UploadMedia(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("UploadMedia status = %d, want %d; body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

func TestQueueMediaFileRollbackIgnoresRequestCancellation(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	ctx, cancel := context.WithCancel(context.Background())
	fs := &cancelAfterPutShadowFS{cancel: cancel}
	h := &Handler{DB: db, ShadowFS: fs}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload/media", nil)
	req = req.WithContext(withUserAndAdmin(ctx, userID, false))

	result := h.queueMediaFile(req, userID, "rollback.jpg", "image/jpeg", []byte("image bytes"), "image_no_exif", nil)
	if result.Status != "error" {
		t.Fatalf("queueMediaFile status = %q, want error", result.Status)
	}
	if fs.deleteCalls != 1 {
		t.Fatalf("rollback delete calls = %d, want 1", fs.deleteCalls)
	}
	if fs.deleteSawCanceledContext {
		t.Fatal("rollback delete used canceled request context")
	}
}

type cancelAfterPutShadowFS struct {
	cancel                   context.CancelFunc
	deleteCalls              int
	deleteSawCanceledContext bool
}

func (fs *cancelAfterPutShadowFS) Put(ctx context.Context, entry shadowfs.FileEntry, data io.Reader) (shadowfs.FileRecord, error) {
	fs.cancel()
	return shadowfs.FileRecord{ID: "queued-file-id", UserID: entry.UserID}, nil
}

func (fs *cancelAfterPutShadowFS) Get(context.Context, string, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func (fs *cancelAfterPutShadowFS) Delete(ctx context.Context, userID, fileID string) error {
	fs.deleteCalls++
	if ctx.Err() != nil {
		fs.deleteSawCanceledContext = true
	}
	return nil
}

func (fs *cancelAfterPutShadowFS) DeleteAllForUser(context.Context, string) error {
	return errors.New("not implemented")
}

func (fs *cancelAfterPutShadowFS) List(context.Context, string, *string) ([]shadowfs.FileRecord, error) {
	return nil, errors.New("not implemented")
}

func (fs *cancelAfterPutShadowFS) StreamExportFiles(context.Context, string, shadowfs.ExportFileSink) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (fs *cancelAfterPutShadowFS) StreamToZip(context.Context, string, *zip.Writer) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (fs *cancelAfterPutShadowFS) Resolve(context.Context, string, string) (shadowfs.ResolvedFile, error) {
	return shadowfs.ResolvedFile{}, errors.New("not implemented")
}

func (fs *cancelAfterPutShadowFS) DiskUsage(context.Context, string) (int64, error) {
	return 0, errors.New("not implemented")
}

func (fs *cancelAfterPutShadowFS) Archive(context.Context, string, string, string) error {
	return errors.New("not implemented")
}

func (fs *cancelAfterPutShadowFS) PutCompanion(context.Context, string, string, string, []byte) error {
	return errors.New("not implemented")
}

func (fs *cancelAfterPutShadowFS) GetCompanion(context.Context, string, string, string) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (fs *cancelAfterPutShadowFS) DeleteCompanions(context.Context, string, string) error {
	return errors.New("not implemented")
}

func (fs *cancelAfterPutShadowFS) PutCompanionAt(context.Context, string, []byte) error {
	return errors.New("not implemented")
}

func (fs *cancelAfterPutShadowFS) GetCompanionAt(context.Context, string) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (fs *cancelAfterPutShadowFS) DeleteCompanionAt(context.Context, string) error {
	return errors.New("not implemented")
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
