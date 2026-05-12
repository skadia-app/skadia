package handler

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/importer"
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
}

func newActivityPhotoStorageTestFS(t *testing.T, pool *pgxpool.Pool) shadowfs.ShadowFS {
	t.Helper()
	dir := t.TempDir()
	local := shadowfs.NewLocalBackend(dir)
	return shadowfs.NewUnifiedShadowFS(pool, local, nil, "local", dir, shadowfs.NewSchemaRenderer(), nil, 1, 0, "", "", "")
}

func newPhotoUploadRequest(t *testing.T, userID, activityID, filename string, data []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="photo"; filename="`+filename+`"`)
	header.Set("Content-Type", "image/jpeg")
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
