package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUploadActivityPhotoStoresOneOriginalUserFile(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	activityID := insertMinimalActivity(t, db, userID)
	fs := newActivityPhotoStorageTestFS(t, pool)
	h := &Handler{
		DB:            db,
		ShadowFS:      fs,
		ImportService: importer.NewService(db, fs, nil),
	}

	body := jpegTestImage(t, 24, 24, color.RGBA{R: 210, G: 50, B: 40, A: 255}, 90)
	req := newPhotoUploadRequest(t, userID, activityID, "created.jpg", body)
	rr := httptest.NewRecorder()

	h.UploadActivityPhoto(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("UploadActivityPhoto status = %d, want %d; body=%s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	var activeMedia int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM user_files
		WHERE user_id = $1 AND file_type = 'media' AND status = 'active'`,
		userID,
	).Scan(&activeMedia); err != nil {
		t.Fatalf("count active media user_files: %v", err)
	}
	if activeMedia != 1 {
		t.Fatalf("active media user_files = %d, want 1", activeMedia)
	}

	var photoFileID string
	if err := pool.QueryRow(context.Background(), `
		SELECT COALESCE(file_id::text, '')
		FROM activity_photos
		WHERE user_id = $1 AND activity_id = $2`,
		userID, activityID,
	).Scan(&photoFileID); err != nil {
		t.Fatalf("select activity photo file_id: %v", err)
	}
	if photoFileID == "" {
		t.Fatal("activity_photos.file_id is empty")
	}

	var linkedRows int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM user_files
		WHERE id = $1 AND user_id = $2 AND file_type = 'media' AND status = 'active'`,
		photoFileID, userID,
	).Scan(&linkedRows); err != nil {
		t.Fatalf("count linked user_files row: %v", err)
	}
	if linkedRows != 1 {
		t.Fatalf("linked active media rows = %d, want 1", linkedRows)
	}

	var logStatus, importType, targetType, targetID string
	if err := pool.QueryRow(context.Background(), `
		SELECT status, import_type, metadata->>'target_type', metadata->>'target_id'
		FROM import_log
		WHERE user_id = $1 AND filename = 'created.jpg'
		ORDER BY created_at DESC
		LIMIT 1`,
		userID,
	).Scan(&logStatus, &importType, &targetType, &targetID); err != nil {
		t.Fatalf("select activity photo import_log: %v", err)
	}
	if logStatus != "completed" || importType != "photo" || targetType != "activity_photo" || targetID == "" {
		t.Fatalf("activity photo import_log = status %q type %q target_type %q target_id %q, want completed/photo/activity_photo/<id>",
			logStatus, importType, targetType, targetID)
	}
}

func TestActivityPhotoPreviewURLUsesPreviewEndpointRegardlessOfLegacyS3Field(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	activityID := insertMinimalActivity(t, db, userID)
	if _, err := pool.Exec(context.Background(), `UPDATE activities SET title = 'Preview Search Ride' WHERE id = $1`, activityID); err != nil {
		t.Fatalf("set activity title: %v", err)
	}
	fs := newActivityPhotoStorageTestFS(t, pool)
	h := &Handler{DB: db, ShadowFS: fs, ImportService: importer.NewService(db, fs, nil)}

	req := newPhotoUploadRequest(t, userID, activityID, "synthetic-preview-photo.jpg", jpegTestImage(t, 48, 48, color.RGBA{R: 20, G: 140, B: 220, A: 255}, 90))
	rr := httptest.NewRecorder()
	h.UploadActivityPhoto(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("UploadActivityPhoto status = %d; body=%s", rr.Code, rr.Body.String())
	}

	listReq := httptest.NewRequest("GET", "/api/v1/activities/"+activityID+"/photos", nil)
	listReq = listReq.WithContext(context.WithValue(listReq.Context(), middleware.UserIDKey, userID))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", activityID)
	listReq = listReq.WithContext(context.WithValue(listReq.Context(), chi.RouteCtxKey, rctx))
	listRR := httptest.NewRecorder()
	h.ListActivityPhotos(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("ListActivityPhotos status = %d; body=%s", listRR.Code, listRR.Body.String())
	}
	var photos []model.ActivityPhoto
	if err := json.NewDecoder(listRR.Body).Decode(&photos); err != nil {
		t.Fatalf("decode photos: %v", err)
	}
	if len(photos) != 1 {
		t.Fatalf("photos len = %d, want 1", len(photos))
	}
	if photos[0].PreviewURL != "/photos/"+photos[0].ID+"/preview" {
		t.Fatalf("PreviewURL = %q, want preview endpoint", photos[0].PreviewURL)
	}

	feedReq := httptest.NewRequest("GET", "/api/v1/photos?sort=added&page=1", nil)
	feedReq = feedReq.WithContext(context.WithValue(feedReq.Context(), middleware.UserIDKey, userID))
	feedRR := httptest.NewRecorder()
	h.ListPhotoFeed(feedRR, feedReq)
	if feedRR.Code != http.StatusOK {
		t.Fatalf("ListPhotoFeed status = %d; body=%s", feedRR.Code, feedRR.Body.String())
	}
	var feed model.PhotoFeedPage
	if err := json.NewDecoder(feedRR.Body).Decode(&feed); err != nil {
		t.Fatalf("decode feed: %v", err)
	}
	if len(feed.Groups) == 0 || len(feed.Groups[0].Photos) == 0 {
		t.Fatalf("feed response missing photo: %#v", feed)
	}
	if feed.Groups[0].Photos[0].PreviewURL != "/photos/"+photos[0].ID+"/preview" {
		t.Fatalf("feed PreviewURL = %q, want preview endpoint", feed.Groups[0].Photos[0].PreviewURL)
	}

	searchReq := httptest.NewRequest("GET", "/api/v1/photos/search?q=Preview%20Search&limit=10", nil)
	searchReq = searchReq.WithContext(context.WithValue(searchReq.Context(), middleware.UserIDKey, userID))
	searchRR := httptest.NewRecorder()
	h.SearchPhotos(searchRR, searchReq)
	if searchRR.Code != http.StatusOK {
		t.Fatalf("SearchPhotos status = %d; body=%s", searchRR.Code, searchRR.Body.String())
	}
	var results []model.PhotoSearchResult
	if err := json.NewDecoder(searchRR.Body).Decode(&results); err != nil {
		t.Fatalf("decode search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("search results len = %d, want 1", len(results))
	}
	if results[0].Photo.PreviewURL != "/photos/"+photos[0].ID+"/preview" {
		t.Fatalf("search PreviewURL = %q, want preview endpoint", results[0].Photo.PreviewURL)
	}
}

func TestUploadActivityPhotoUsesConfiguredUploadLimit(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	activityID := insertMinimalActivity(t, db, userID)
	fs := newActivityPhotoStorageTestFS(t, pool)
	h := &Handler{
		DB:            db,
		ShadowFS:      fs,
		ImportService: importer.NewService(db, fs, nil),
		Settings: settings.NewStoreForTest(map[string]string{
			"limits.max_upload_bytes": "128",
		}),
	}

	body := jpegTestImage(t, 64, 64, color.RGBA{R: 210, G: 50, B: 40, A: 255}, 90)
	req := newPhotoUploadRequest(t, userID, activityID, "too-large.jpg", body)
	rr := httptest.NewRecorder()

	h.UploadActivityPhoto(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("UploadActivityPhoto status = %d, want %d; body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

func TestUploadActivityPhotoAcceptsOctetStreamFFmpegStillByExtension(t *testing.T) {
	for _, tt := range []struct {
		name        string
		ext         string
		contentType string
	}{
		{name: "jxl", ext: ".jxl", contentType: "image/jxl"},
		{name: "avif", ext: ".avif", contentType: "image/avif"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pool := fusionTestPool(t)
			db := &database.DB{Pool: pool}
			userID := insertTestUser(t, pool)
			activityID := insertMinimalActivity(t, db, userID)
			fs := newActivityPhotoStorageTestFS(t, pool)
			h := &Handler{
				DB:            db,
				ShadowFS:      fs,
				ImportService: importer.NewService(db, fs, nil),
			}

			body := encodeHandlerImageWithFFmpegOrSkip(t, pngTestImage(t, 24, 24), ".png", tt.ext)
			req := newPhotoUploadRequestWithContentType(t, userID, activityID, "upload"+tt.ext, body, "application/octet-stream")
			rr := httptest.NewRecorder()

			h.UploadActivityPhoto(rr, req)

			if rr.Code != http.StatusCreated {
				t.Fatalf("UploadActivityPhoto status = %d, want %d; body=%s", rr.Code, http.StatusCreated, rr.Body.String())
			}

			var gotContentType string
			if err := pool.QueryRow(context.Background(), `
				SELECT content_type
				FROM activity_photos
				WHERE user_id = $1 AND activity_id = $2`,
				userID, activityID,
			).Scan(&gotContentType); err != nil {
				t.Fatalf("select stored content_type: %v", err)
			}
			if gotContentType != tt.contentType {
				t.Fatalf("stored content_type = %q, want %q", gotContentType, tt.contentType)
			}
		})
	}
}

func TestUploadActivityPhotoLimitBlocksOnlyNewRows(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	activityID := insertMinimalActivity(t, db, userID)
	fs := newActivityPhotoStorageTestFS(t, pool)
	store := settings.NewStoreForTest(map[string]string{"limits.max_activity_photos_per_activity": "1"})
	svc := importer.NewService(db, fs, nil)
	svc.ActivityPhotoCreateGuard = NewActivityPhotoCreateGuard(db, store)
	h := &Handler{
		DB:            db,
		ShadowFS:      fs,
		ImportService: svc,
		Settings:      store,
	}

	body := jpegPatternTestImage(t, 48, 48, 92)
	createdReq := newPhotoUploadRequest(t, userID, activityID, "created.jpg", body)
	createdRR := httptest.NewRecorder()
	h.UploadActivityPhoto(createdRR, createdReq)
	if createdRR.Code != http.StatusCreated {
		t.Fatalf("first upload status = %d, want %d; body=%s", createdRR.Code, http.StatusCreated, createdRR.Body.String())
	}

	duplicateReq := newPhotoUploadRequest(t, userID, activityID, "duplicate.jpg", body)
	duplicateRR := httptest.NewRecorder()
	h.UploadActivityPhoto(duplicateRR, duplicateReq)
	if duplicateRR.Code != http.StatusOK {
		t.Fatalf("duplicate at limit status = %d, want %d; body=%s", duplicateRR.Code, http.StatusOK, duplicateRR.Body.String())
	}

	newReq := newPhotoUploadRequest(t, userID, activityID, "new.jpg", jpegTestImage(t, 31, 29, color.RGBA{R: 10, G: 210, B: 120, A: 255}, 90))
	newRR := httptest.NewRecorder()
	h.UploadActivityPhoto(newRR, newReq)
	if newRR.Code != http.StatusConflict {
		t.Fatalf("new upload at limit status = %d, want %d; body=%s", newRR.Code, http.StatusConflict, newRR.Body.String())
	}
	if got := handlerActivityPhotoRowCount(t, pool, userID); got != 1 {
		t.Fatalf("activity photo rows after rejected upload = %d, want 1", got)
	}
}

func TestUploadActivityPhotoQuotaExceededReturns507(t *testing.T) {
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

	req := newPhotoUploadRequest(t, userID, activityID, "quota.jpg", jpegTestImage(t, 24, 24, color.RGBA{R: 50, G: 60, B: 220, A: 255}, 90))
	rr := httptest.NewRecorder()
	h.UploadActivityPhoto(rr, req)

	if rr.Code != http.StatusInsufficientStorage {
		t.Fatalf("quota upload status = %d, want %d; body=%s", rr.Code, http.StatusInsufficientStorage, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), codeStorageQuotaExceeded) {
		t.Fatalf("quota response missing code %q: %s", codeStorageQuotaExceeded, rr.Body.String())
	}
}

func TestUploadActivityPhotoLimitConcurrentUploadsAllowOneRow(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	activityID := insertMinimalActivity(t, db, userID)
	fs := newActivityPhotoStorageTestFS(t, pool)
	store := settings.NewStoreForTest(map[string]string{"limits.max_activity_photos_per_activity": "1"})
	svc := importer.NewService(db, fs, nil)
	svc.ActivityPhotoCreateGuard = NewActivityPhotoCreateGuard(db, store)
	h := &Handler{
		DB:            db,
		ShadowFS:      fs,
		ImportService: svc,
		Settings:      store,
	}

	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for i, body := range [][]byte{
		jpegPatternTestImage(t, 48, 48, 91),
		jpegTestImage(t, 31, 29, color.RGBA{R: 210, G: 70, B: 30, A: 255}, 90),
	} {
		wg.Add(1)
		go func(i int, body []byte) {
			defer wg.Done()
			req := newPhotoUploadRequest(t, userID, activityID, fmt.Sprintf("concurrent-%d.jpg", i), body)
			rr := httptest.NewRecorder()
			h.UploadActivityPhoto(rr, req)
			statuses <- rr.Code
		}(i, body)
	}
	wg.Wait()
	close(statuses)

	var created, conflicted int
	for status := range statuses {
		switch status {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflicted++
		default:
			t.Fatalf("unexpected concurrent upload status %d", status)
		}
	}
	if created != 1 || conflicted != 1 {
		t.Fatalf("concurrent statuses: created=%d conflicted=%d, want 1/1", created, conflicted)
	}
	if got := handlerActivityPhotoRowCount(t, pool, userID); got != 1 {
		t.Fatalf("activity photo rows after concurrent uploads = %d, want 1", got)
	}
	if got := handlerActiveMediaCount(t, pool, userID); got != 1 {
		t.Fatalf("active media files after concurrent uploads = %d, want 1", got)
	}
}

func TestFFmpegStillImageContentTypesIncludeAVIF(t *testing.T) {
	for _, ct := range []string{"image/heic", "image/heif", "image/jxl", "image/avif"} {
		if !isFFmpegStillImageContentType(ct) {
			t.Fatalf("isFFmpegStillImageContentType(%q) = false, want true", ct)
		}
	}
}

func newActivityPhotoStorageTestFS(t *testing.T, pool *pgxpool.Pool) shadowfs.ShadowFS {
	t.Helper()
	dir := t.TempDir()
	local := shadowfs.NewLocalBackend(dir)
	return shadowfs.NewUnifiedShadowFS(pool, local, nil, "local", dir, shadowfs.NewSchemaRenderer(), nil, 1, 0, "", "", "")
}

func newPhotoUploadRequest(t *testing.T, userID, activityID, filename string, data []byte) *http.Request {
	t.Helper()
	return newPhotoUploadRequestWithContentType(t, userID, activityID, filename, data, "image/jpeg")
}

func newPhotoUploadRequestWithContentType(t *testing.T, userID, activityID, filename string, data []byte, contentType string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="photo"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := mw.CreatePart(header)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write multipart photo: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/activities/"+activityID+"/photos", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", activityID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
	ctx = withUserAndAdmin(ctx, userID, false)
	return req.WithContext(ctx)
}

func jpegTestImage(t *testing.T, width, height int, c color.RGBA, quality int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	return buf.Bytes()
}

func pngTestImage(t *testing.T, width, height int) []byte {
	t.Helper()
	return encodePNGHandlerImage(t, handlerPatternImage(width, height))
}

func jpegPatternTestImage(t *testing.T, width, height, quality int) []byte {
	t.Helper()
	return encodeJPEGHandlerImage(t, handlerPatternImage(width, height), quality)
}

func handlerPatternImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r := uint8(40 + (x * 150 / max(width-1, 1)))
			g := uint8(60 + (y * 130 / max(height-1, 1)))
			b := uint8(180 - ((x + y) * 90 / max(width+height-2, 1)))
			if x > width/3 && x < 2*width/3 && y > height/3 && y < 2*height/3 {
				r, g, b = 230, 80, 40
			}
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img
}

func encodeJPEGHandlerImage(t *testing.T, img image.Image, quality int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	return buf.Bytes()
}

func encodePNGHandlerImage(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

func encodeHandlerImageWithFFmpegOrSkip(t *testing.T, input []byte, inputExt, outputExt string) []byte {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skipf("ffmpeg not available: %v", err)
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "input"+inputExt)
	out := filepath.Join(dir, "output"+outputExt)
	if err := os.WriteFile(in, input, 0o600); err != nil {
		t.Fatalf("write ffmpeg input: %v", err)
	}
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", in, "-frames:v", "1", out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg cannot encode %s: %v: %s", outputExt, err, string(output))
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read ffmpeg output: %v", err)
	}
	if len(data) == 0 {
		t.Skipf("ffmpeg produced empty %s fixture", outputExt)
	}
	return data
}
