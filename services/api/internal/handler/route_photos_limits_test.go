package handler

import (
	"bytes"
	"context"
	"image/color"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUploadRoutePhotoLimitBlocksNewRows(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	fs := newActivityPhotoStorageTestFS(t, pool)
	store := settings.NewStoreForTest(map[string]string{"limits.max_route_photos_per_route": "1"})
	h := &Handler{DB: db, ShadowFS: fs, Settings: store}
	routeID := insertTestRoute(t, h, userID)

	firstReq := newRoutePhotoUploadRequest(t, userID, routeID, "first.jpg", jpegTestImage(t, 24, 24, color.RGBA{R: 100, G: 40, B: 20, A: 255}, 90))
	firstRR := httptest.NewRecorder()
	h.UploadRoutePhoto(firstRR, firstReq)
	if firstRR.Code != http.StatusCreated {
		t.Fatalf("first route photo status = %d, want %d; body=%s", firstRR.Code, http.StatusCreated, firstRR.Body.String())
	}

	secondReq := newRoutePhotoUploadRequest(t, userID, routeID, "second.jpg", jpegTestImage(t, 24, 24, color.RGBA{R: 20, G: 140, B: 220, A: 255}, 90))
	secondRR := httptest.NewRecorder()
	h.UploadRoutePhoto(secondRR, secondReq)
	if secondRR.Code != http.StatusConflict {
		t.Fatalf("second route photo status = %d, want %d; body=%s", secondRR.Code, http.StatusConflict, secondRR.Body.String())
	}
	if !strings.Contains(secondRR.Body.String(), codeRoutePhotoLimitExceeded) {
		t.Fatalf("route limit response missing code %q: %s", codeRoutePhotoLimitExceeded, secondRR.Body.String())
	}
	if got := routePhotoRowCount(t, pool, userID, routeID); got != 1 {
		t.Fatalf("route photo rows after rejected upload = %d, want 1", got)
	}
}

func TestUploadRoutePhotoQuotaExceededReturns507(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	dir := t.TempDir()
	fs := shadowfs.NewUnifiedShadowFS(pool, shadowfs.NewLocalBackend(dir), nil, "local", dir, shadowfs.NewSchemaRenderer(), nil, 1, 1, "", "", "")
	h := &Handler{DB: db, ShadowFS: fs}
	routeID := insertTestRoute(t, h, userID)

	req := newRoutePhotoUploadRequest(t, userID, routeID, "quota.jpg", jpegTestImage(t, 24, 24, color.RGBA{R: 120, G: 160, B: 220, A: 255}, 90))
	rr := httptest.NewRecorder()
	h.UploadRoutePhoto(rr, req)
	if rr.Code != http.StatusInsufficientStorage {
		t.Fatalf("route quota status = %d, want %d; body=%s", rr.Code, http.StatusInsufficientStorage, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), codeStorageQuotaExceeded) {
		t.Fatalf("quota response missing code %q: %s", codeStorageQuotaExceeded, rr.Body.String())
	}
}

func newRoutePhotoUploadRequest(t *testing.T, userID, routeID, filename string, data []byte) *http.Request {
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
		t.Fatalf("write route photo: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/routes/"+routeID+"/photos", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", routeID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
	ctx = withUserAndAdmin(ctx, userID, false)
	return req.WithContext(ctx)
}

func routePhotoRowCount(t *testing.T, pool *pgxpool.Pool, userID, routeID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM route_photos WHERE user_id = $1 AND route_id = $2`,
		userID, routeID,
	).Scan(&count); err != nil {
		t.Fatalf("count route photos: %v", err)
	}
	return count
}
