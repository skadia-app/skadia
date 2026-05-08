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

func TestNewInternalMetricsServerDisabledWithoutAddr(t *testing.T) {
	if srv := newInternalMetricsServer("", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})); srv != nil {
		t.Fatalf("server = %#v, want nil", srv)
	}
}

func TestNewInternalMetricsServerServesMetricsWithoutAdminMiddleware(t *testing.T) {
	srv := newInternalMetricsServer(" 127.0.0.1:0 ", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	if srv == nil {
		t.Fatal("server = nil, want configured server")
	}
	if srv.Addr != "127.0.0.1:0" {
		t.Fatalf("server addr = %q, want 127.0.0.1:0", srv.Addr)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("internal metrics status = %d, want 204", rec.Code)
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/activities", nil)
	missingRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("internal non-metrics status = %d, want 404", missingRec.Code)
	}
}
