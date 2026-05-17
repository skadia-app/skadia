package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/go-chi/chi/v5"
)

func TestCoreBlocksFullOnlySegmentActionsBeforeDBWork(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "core"}, DB: &database.DB{Pool: nil}}
	r := chi.NewRouter()
	r.Post("/api/v1/segments/import-lifts", h.ImportLiftsFromOSM)
	r.Post("/api/v1/segments/{id}/recompute", h.RecomputeSegment)
	r.Post("/api/v1/segments/{id}/publish", h.PublishSegment)

	for _, tc := range []struct {
		name string
		path string
	}{
		{"import lifts", "/api/v1/segments/import-lifts"},
		{"recompute", "/api/v1/segments/00000000-0000-0000-0000-000000000010/recompute"},
		{"publish", "/api/v1/segments/00000000-0000-0000-0000-000000000010/publish"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", tc.path, nil)
			req = req.WithContext(withUserAndAdmin(req.Context(), "00000000-0000-0000-0000-000000000004", false))
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", rec.Code)
			}
			if body := rec.Body.String(); !strings.Contains(body, "feature_unavailable") {
				t.Fatalf("response body %q does not contain feature_unavailable", body)
			}
		})
	}
}
