package handler

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync"
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
	if !strings.Contains(secondRR.Body.String(), codeRoutePhotoLimitReached) {
		t.Fatalf("route limit response missing code %q: %s", codeRoutePhotoLimitReached, secondRR.Body.String())
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

func TestUploadRoutePhotoLimitConcurrentUploadsAllowOneRow(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	fs := newActivityPhotoStorageTestFS(t, pool)
	store := settings.NewStoreForTest(map[string]string{"limits.max_route_photos_per_route": "1"})
	h := &Handler{DB: db, ShadowFS: fs, Settings: store}
	routeID := insertTestRoute(t, h, userID)

	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for i, body := range [][]byte{
		jpegTestImage(t, 24, 24, color.RGBA{R: 210, G: 60, B: 40, A: 255}, 90),
		jpegTestImage(t, 25, 25, color.RGBA{R: 30, G: 120, B: 220, A: 255}, 90),
	} {
		wg.Add(1)
		go func(i int, body []byte) {
			defer wg.Done()
			req := newRoutePhotoUploadRequest(t, userID, routeID, fmt.Sprintf("concurrent-%d.jpg", i), body)
			rr := httptest.NewRecorder()
			h.UploadRoutePhoto(rr, req)
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
			t.Fatalf("unexpected concurrent route upload status %d", status)
		}
	}
	if created != 1 || conflicted != 1 {
		t.Fatalf("concurrent statuses: created=%d conflicted=%d, want 1/1", created, conflicted)
	}
	if got := routePhotoRowCount(t, pool, userID, routeID); got != 1 {
		t.Fatalf("route photo rows after concurrent uploads = %d, want 1", got)
	}
}

func TestDeleteRoutePhotoRemovesDerivatives(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	fs := newActivityPhotoStorageTestFS(t, pool)
	h := &Handler{DB: db, ShadowFS: fs}
	routeID := insertTestRoute(t, h, userID)

	req := newRoutePhotoUploadRequest(t, userID, routeID, "with-derivatives.jpg", jpegPatternTestImage(t, 300, 300, 90))
	rr := httptest.NewRecorder()
	h.UploadRoutePhoto(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("upload route photo status = %d, want %d; body=%s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	var photoID, fileID string
	if err := pool.QueryRow(context.Background(),
		`SELECT id::text, COALESCE(file_id::text, '') FROM route_photos WHERE user_id = $1 AND route_id = $2`,
		userID, routeID,
	).Scan(&photoID, &fileID); err != nil {
		t.Fatalf("select route photo: %v", err)
	}
	if fileID == "" {
		t.Fatal("route photo file_id is empty")
	}
	if got := routePhotoDerivativeCount(t, pool, userID, photoID); got == 0 {
		t.Fatal("route photo upload created no derivatives to verify cleanup")
	}

	delReq := newRoutePhotoDeleteRequest(userID, routeID, photoID)
	delRR := httptest.NewRecorder()
	h.DeleteRoutePhoto(delRR, delReq)
	if delRR.Code != http.StatusNoContent {
		t.Fatalf("delete route photo status = %d, want %d; body=%s", delRR.Code, http.StatusNoContent, delRR.Body.String())
	}
	if got := userFileIDCount(t, pool, userID, fileID); got != 0 {
		t.Fatalf("original user_file rows after delete = %d, want 0", got)
	}
	if got := routePhotoDerivativeCount(t, pool, userID, photoID); got != 0 {
		t.Fatalf("route derivative rows after delete = %d, want 0", got)
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

func newRoutePhotoDeleteRequest(userID, routeID, photoID string) *http.Request {
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/routes/"+routeID+"/photos/"+photoID, nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", routeID)
	routeCtx.URLParams.Add("photoId", photoID)
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

func routePhotoDerivativeCount(t *testing.T, pool *pgxpool.Pool, userID, photoID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM user_files WHERE user_id = $1 AND parent_type = 'route_photo' AND parent_id = $2`,
		userID, photoID,
	).Scan(&count); err != nil {
		t.Fatalf("count route photo derivatives: %v", err)
	}
	return count
}

func userFileIDCount(t *testing.T, pool *pgxpool.Pool, userID, fileID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM user_files WHERE user_id = $1 AND id = $2`,
		userID, fileID,
	).Scan(&count); err != nil {
		t.Fatalf("count user file: %v", err)
	}
	return count
}
