package handler

import (
	"context"
	"encoding/json"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

func TestRoutePhotoUploadAndGetRouteExposeMediaURLs(t *testing.T) {
	for _, target := range []string{"local", "s3"} {
		t.Run(target, func(t *testing.T) {
			pool := fusionTestPool(t)
			db := &database.DB{Pool: pool}
			userID := insertTestUser(t, pool)
			fs := newRoutePhotoStorageTestFS(t, pool, target)
			h := &Handler{DB: db, ShadowFS: fs}
			routeID := insertTestRoute(t, h, userID)

			uploadReq := newRoutePhotoUploadRequest(t, userID, routeID, "synthetic-route-photo-"+target+".jpg", jpegPatternTestImage(t, 300, 300, 90))
			uploadRR := httptest.NewRecorder()
			h.UploadRoutePhoto(uploadRR, uploadReq)
			if uploadRR.Code != http.StatusCreated {
				t.Fatalf("UploadRoutePhoto status = %d; body=%s", uploadRR.Code, uploadRR.Body.String())
			}
			var uploaded model.RoutePhoto
			if err := json.NewDecoder(uploadRR.Body).Decode(&uploaded); err != nil {
				t.Fatalf("decode upload: %v", err)
			}
			if uploaded.FullURL == "" || uploaded.PreviewURL == "" || uploaded.ThumbURL == "" {
				t.Fatalf("uploaded URLs missing: %#v", uploaded)
			}

			for _, variant := range []string{"full", "preview", "thumb"} {
				req := newRoutePhotoStreamRequest(userID, routeID, uploaded.ID, variant)
				rr := httptest.NewRecorder()
				switch variant {
				case "full":
					h.StreamRoutePhotoFull(rr, req)
				case "preview":
					h.StreamRoutePhotoPreview(rr, req)
				case "thumb":
					h.StreamRoutePhotoThumb(rr, req)
				}
				if rr.Code != http.StatusOK {
					t.Fatalf("%s stream status = %d, want 200; body=%s", variant, rr.Code, rr.Body.String())
				}
			}

			getReq := httptest.NewRequest("GET", "/api/v1/routes/"+routeID, nil)
			getReq = getReq.WithContext(context.WithValue(getReq.Context(), middleware.UserIDKey, userID))
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", routeID)
			getReq = getReq.WithContext(context.WithValue(getReq.Context(), chi.RouteCtxKey, rctx))
			getRR := httptest.NewRecorder()
			h.GetRoute(getRR, getReq)
			if getRR.Code != http.StatusOK {
				t.Fatalf("GetRoute status = %d; body=%s", getRR.Code, getRR.Body.String())
			}
			var route model.PlannedRoute
			if err := json.NewDecoder(getRR.Body).Decode(&route); err != nil {
				t.Fatalf("decode route: %v", err)
			}
			if len(route.Photos) != 1 {
				t.Fatalf("route photos len = %d, want 1", len(route.Photos))
			}
			if !strings.HasPrefix(route.Photos[0].PreviewURL, "/routes/"+routeID+"/photos/"+route.Photos[0].ID+"/preview") {
				t.Fatalf("route preview_url = %q", route.Photos[0].PreviewURL)
			}
		})
	}
}

func TestRoutePhotoStreamEndpointsRequireRouteOwnership(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	ownerID := insertTestUser(t, pool)
	otherID := insertTestUser(t, pool)
	fs := newActivityPhotoStorageTestFS(t, pool)
	h := &Handler{DB: db, ShadowFS: fs}
	routeID := insertTestRoute(t, h, ownerID)

	uploadReq := newRoutePhotoUploadRequest(t, ownerID, routeID, "synthetic-route-photo.jpg", jpegTestImage(t, 48, 48, color.RGBA{R: 80, G: 120, B: 200, A: 255}, 90))
	uploadRR := httptest.NewRecorder()
	h.UploadRoutePhoto(uploadRR, uploadReq)
	if uploadRR.Code != http.StatusCreated {
		t.Fatalf("UploadRoutePhoto status = %d; body=%s", uploadRR.Code, uploadRR.Body.String())
	}
	var uploaded model.RoutePhoto
	if err := json.NewDecoder(uploadRR.Body).Decode(&uploaded); err != nil {
		t.Fatalf("decode upload: %v", err)
	}

	req := newRoutePhotoStreamRequest(otherID, routeID, uploaded.ID, "full")
	rr := httptest.NewRecorder()
	h.StreamRoutePhotoFull(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("foreign StreamRoutePhotoFull status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

func TestDeleteRoutePhotoRemovesDerivativesForLocalAndS3(t *testing.T) {
	for _, target := range []string{"local", "s3"} {
		t.Run(target, func(t *testing.T) {
			pool := fusionTestPool(t)
			db := &database.DB{Pool: pool}
			userID := insertTestUser(t, pool)
			fs := newRoutePhotoStorageTestFS(t, pool, target)
			h := &Handler{DB: db, ShadowFS: fs}
			routeID := insertTestRoute(t, h, userID)

			req := newRoutePhotoUploadRequest(t, userID, routeID, "synthetic-route-derivatives-"+target+".jpg", jpegPatternTestImage(t, 300, 300, 90))
			rr := httptest.NewRecorder()
			h.UploadRoutePhoto(rr, req)
			if rr.Code != http.StatusCreated {
				t.Fatalf("upload route photo status = %d, want 201; body=%s", rr.Code, rr.Body.String())
			}
			var uploaded model.RoutePhoto
			if err := json.NewDecoder(rr.Body).Decode(&uploaded); err != nil {
				t.Fatalf("decode upload: %v", err)
			}

			var fileID string
			if err := pool.QueryRow(context.Background(),
				`SELECT COALESCE(file_id::text, '') FROM route_photos WHERE user_id = $1 AND route_id = $2 AND id = $3`,
				userID, routeID, uploaded.ID,
			).Scan(&fileID); err != nil {
				t.Fatalf("select route photo file_id: %v", err)
			}
			if fileID == "" {
				t.Fatal("route photo file_id is empty")
			}
			if got := routePhotoDerivativeCount(t, pool, userID, uploaded.ID); got == 0 {
				t.Fatal("route photo upload created no derivatives to verify cleanup")
			}

			delReq := newRoutePhotoDeleteRequest(userID, routeID, uploaded.ID)
			delRR := httptest.NewRecorder()
			h.DeleteRoutePhoto(delRR, delReq)
			if delRR.Code != http.StatusNoContent {
				t.Fatalf("delete route photo status = %d, want 204; body=%s", delRR.Code, delRR.Body.String())
			}
			if got := userFileIDCount(t, pool, userID, fileID); got != 0 {
				t.Fatalf("original user_file rows after delete = %d, want 0", got)
			}
			if got := routePhotoDerivativeCount(t, pool, userID, uploaded.ID); got != 0 {
				t.Fatalf("route derivative rows after delete = %d, want 0", got)
			}
		})
	}
}

func newRoutePhotoStorageTestFS(t *testing.T, pool *pgxpool.Pool, target string) shadowfs.ShadowFS {
	t.Helper()
	dir := t.TempDir()
	backend := shadowfs.NewLocalBackend(dir)
	var local, s3 shadowfs.StorageBackend
	switch target {
	case "local":
		local = backend
	case "s3":
		s3 = backend
	default:
		t.Fatalf("unknown target backend %q", target)
	}
	return shadowfs.NewUnifiedShadowFS(pool, local, s3, target, dir, shadowfs.NewSchemaRenderer(), nil, 1, 0, "skadia-originals", "", "skadia-originals")
}

func newRoutePhotoStreamRequest(userID, routeID, photoID, variant string) *http.Request {
	req := httptest.NewRequest("GET", "/api/v1/routes/"+routeID+"/photos/"+photoID+"/"+variant, nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", routeID)
	rctx.URLParams.Add("photoId", photoID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}
