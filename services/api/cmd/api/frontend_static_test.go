package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMountFrontendStaticServesSPAWithoutMaskingAPI404(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "index.html"), "<!doctype html><div id=\"root\">app</div>")
	writeTestFile(t, filepath.Join(dir, "assets", "app.js"), "console.log('app')")

	router := chi.NewRouter()
	router.Get("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	if !mountFrontendStatic(router, dir) {
		t.Fatal("mountFrontendStatic returned false for an existing frontend directory")
	}

	assertStaticResponse(t, router, "/", http.StatusOK, "text/html", "app")
	assertStaticResponse(t, router, "/dashboard", http.StatusOK, "text/html", "app")
	assertStaticResponse(t, router, "/assets/app.js", http.StatusOK, "text/javascript", "console.log")
	assertStaticHeader(t, router, "/assets/app.js", "Cache-Control", "public, max-age=31536000, immutable")
	assertStaticResponse(t, router, "/api/v1/health", http.StatusNoContent, "", "")
	assertStaticResponse(t, router, "/api/v1/missing", http.StatusNotFound, "", "")
	assertStaticResponse(t, router, "/assets/missing.js", http.StatusNotFound, "", "")
}

func TestMountFrontendStaticRejectsSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "index.html"), "<!doctype html><div id=\"root\">app</div>")
	writeTestFile(t, filepath.Join(outside, "secret.js"), "secret")
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.js"), filepath.Join(dir, "assets", "leak.js")); err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	if !mountFrontendStatic(router, dir) {
		t.Fatal("mountFrontendStatic returned false for an existing frontend directory")
	}

	assertStaticResponse(t, router, "/assets/leak.js", http.StatusNotFound, "", "")
}

func TestMountFrontendStaticSkipsMissingDirectory(t *testing.T) {
	router := chi.NewRouter()
	if mountFrontendStatic(router, filepath.Join(t.TempDir(), "missing")) {
		t.Fatal("mountFrontendStatic returned true for a missing directory")
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func assertStaticHeader(t *testing.T, handler http.Handler, path, header, want string) {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if got := rec.Header().Get(header); got != want {
		t.Fatalf("%s %s = %q, want %q", path, header, got, want)
	}
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertStaticResponse(t *testing.T, handler http.Handler, path string, wantStatus int, wantContentType, wantBody string) {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != wantStatus {
		t.Fatalf("%s status = %d, want %d; body=%q", path, rec.Code, wantStatus, rec.Body.String())
	}
	if wantContentType != "" && !strings.Contains(rec.Header().Get("Content-Type"), wantContentType) {
		t.Fatalf("%s content-type = %q, want contains %q", path, rec.Header().Get("Content-Type"), wantContentType)
	}
	if wantBody != "" && !strings.Contains(rec.Body.String(), wantBody) {
		t.Fatalf("%s body = %q, want contains %q", path, rec.Body.String(), wantBody)
	}
}
