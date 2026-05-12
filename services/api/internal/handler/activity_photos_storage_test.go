package handler

import (
	"bytes"
	"context"
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
