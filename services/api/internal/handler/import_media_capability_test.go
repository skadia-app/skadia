package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/go-chi/chi/v5"
)

func TestCoreBlocksPhotoEmbeddingEndpointsBeforeDBWork(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "core"}, DB: &database.DB{Pool: nil}}
	r := chi.NewRouter()
	r.Get("/api/v1/photos/search", h.SearchPhotos)
	r.Post("/api/v1/photos/recompute-embeddings", h.RecomputePhotoEmbeddings)

	for _, tc := range []struct {
		name   string
		method string
		path   string
	}{
		{"semantic search", http.MethodGet, "/api/v1/photos/search?q=forest"},
		{"embedding backfill", http.MethodPost, "/api/v1/photos/recompute-embeddings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req = req.WithContext(withUserAndAdmin(req.Context(), "00000000-0000-0000-0000-000000000005", false))
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", rec.Code)
			}
			if body := rec.Body.String(); !strings.Contains(body, `"code":"feature_unavailable"`) {
				t.Fatalf("response body %q does not contain feature_unavailable", body)
			}
		})
	}
}

func TestS3StorageBlocksLocalPathImportSurfaces(t *testing.T) {
	h := &Handler{
		Config: &HandlerConfig{
			DeploymentProfile: "full",
			StorageMode:       "s3-external",
			StorageBackend:    "s3",
			ImportAllowedDirs: []string{"/srv/imports"},
		},
		DB: &database.DB{Pool: nil},
	}
	r := chi.NewRouter()
	r.Post("/api/v1/import/garmin/detect", h.DetectGarminModules)
	r.Post("/api/v1/import/strava/detect", h.DetectStravaModules)

	for _, path := range []string{"/api/v1/import/garmin/detect", "/api/v1/import/strava/detect"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"source_path":"/srv/imports/export"}`))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(withUserAndAdmin(req.Context(), "00000000-0000-0000-0000-000000000006", false))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s status = %d, want 503", path, rec.Code)
		}
		if body := rec.Body.String(); !strings.Contains(body, `"code":"feature_unavailable"`) || !strings.Contains(body, "s3_storage") {
			t.Fatalf("%s response body %q missing feature_unavailable/s3_storage", path, body)
		}
	}
}

func TestFullDeploymentBlocksUnavailableGeodataReadsBeforeDBWork(t *testing.T) {
	h := &Handler{
		Config: &HandlerConfig{DeploymentProfile: "full"},
		DB:     &database.DB{Pool: nil},
	}
	r := chi.NewRouter()
	r.Get("/api/v1/search/pois", h.SearchPOIs)
	r.Get("/api/v1/activities/{id}/stops", h.GetActivityStops)

	for _, path := range []string{
		"/api/v1/search/pois?q=hut",
		"/api/v1/activities/00000000-0000-0000-0000-000000000007/stops",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(withUserAndAdmin(req.Context(), "00000000-0000-0000-0000-000000000007", false))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s status = %d, want 503; body=%s", path, rec.Code, rec.Body.String())
		}
		if body := rec.Body.String(); !strings.Contains(body, `"code":"feature_unavailable"`) || !strings.Contains(body, "missing_artifacts") {
			t.Fatalf("%s response body %q missing feature_unavailable/missing_artifacts", path, body)
		}
	}
}
