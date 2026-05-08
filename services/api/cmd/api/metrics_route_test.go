package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

func TestMountMetricsRouteRequiresAdminContext(t *testing.T) {
	router := chi.NewRouter()
	mountMetricsRoute(router, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated metrics status = %d, want 403", rec.Code)
	}

	adminReq := req.WithContext(context.WithValue(req.Context(), middleware.IsAdminKey, true))
	adminRec := httptest.NewRecorder()
	router.ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusNoContent {
		t.Fatalf("admin metrics status = %d, want 204", adminRec.Code)
	}
}
