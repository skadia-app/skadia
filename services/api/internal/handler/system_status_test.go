package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/go-chi/chi/v5"
)

func TestAdminSystemStatusHandlerReturnsSchemaVersion(t *testing.T) {
	h := newTestHandler(t, fusionTestPool(t))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system-status", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-test"))
	rec := httptest.NewRecorder()

	h.GetAdminSystemStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"schema_version":1`) {
		t.Fatalf("response missing schema_version: %s", rec.Body.String())
	}
}

func TestAdminSystemStatusRouteRequiresAdmin(t *testing.T) {
	h := newTestHandler(t, fusionTestPool(t))
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/admin", func(r chi.Router) {
			r.Use(middleware.RequireAdmin())
			r.Get("/system-status", h.GetAdminSystemStatus)
		})
	})

	nonAdminReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system-status", nil)
	nonAdminReq = nonAdminReq.WithContext(context.WithValue(nonAdminReq.Context(), middleware.IsAdminKey, false))
	nonAdminRec := httptest.NewRecorder()
	r.ServeHTTP(nonAdminRec, nonAdminReq)
	if nonAdminRec.Code != http.StatusForbidden {
		t.Fatalf("non-admin status = %d, want 403", nonAdminRec.Code)
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system-status", nil)
	adminReq = adminReq.WithContext(context.WithValue(adminReq.Context(), middleware.IsAdminKey, true))
	adminRec := httptest.NewRecorder()
	r.ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("admin status = %d, want 200; body=%s", adminRec.Code, adminRec.Body.String())
	}
}

func TestAdminSystemStatusExportRouteRequiresAdmin(t *testing.T) {
	h := newTestHandler(t, fusionTestPool(t))
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/admin", func(r chi.Router) {
			r.Use(middleware.RequireAdmin())
			r.Get("/system-status/export", h.GetAdminSystemStatusExport)
		})
	})

	nonAdminReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system-status/export", nil)
	nonAdminReq = nonAdminReq.WithContext(context.WithValue(nonAdminReq.Context(), middleware.IsAdminKey, false))
	nonAdminRec := httptest.NewRecorder()
	r.ServeHTTP(nonAdminRec, nonAdminReq)
	if nonAdminRec.Code != http.StatusForbidden {
		t.Fatalf("non-admin status = %d, want 403", nonAdminRec.Code)
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system-status/export", nil)
	adminReq = adminReq.WithContext(context.WithValue(adminReq.Context(), middleware.IsAdminKey, true))
	adminRec := httptest.NewRecorder()
	r.ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("admin status = %d, want 200; body=%s", adminRec.Code, adminRec.Body.String())
	}
	if adminRec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", adminRec.Header().Get("Cache-Control"))
	}
}

func TestPublicProbeDependencyFailureIsGeneric(t *testing.T) {
	cases := []struct {
		name string
		key  string
		err  error
		want string
	}{
		{name: "db", key: "db", err: errors.New("postgres://skadia:secret@db:5432/skadia path=/srv/skadia/data"), want: "unreachable"},
		{name: "s3", key: "s3", err: errors.New("bucket=private-bucket X-Amz-Signature=abc"), want: "unreachable"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := publicProbeDependencyValue(tt.key, tt.err)
			if got != tt.want {
				t.Fatalf("publicProbeDependencyValue = %q, want %q", got, tt.want)
			}
			if strings.Contains(got, "secret") || strings.Contains(got, "private-bucket") || strings.Contains(got, "/srv/") {
				t.Fatalf("public probe value leaked raw dependency detail: %q", got)
			}
		})
	}
}

func TestReadyzAndHealthzDoNotExposeRawProbeErrors(t *testing.T) {
	oldDBPing, oldS3Ping := probeDBPing, probeS3Ping
	t.Cleanup(func() { probeDBPing, probeS3Ping = oldDBPing, oldS3Ping })
	probeDBPing = func(context.Context, *Handler) error {
		return errors.New("postgres://skadia:secret@db:5432/skadia path=/srv/skadia/data")
	}
	probeS3Ping = func(context.Context, *Handler) error {
		return errors.New("bucket=private-bucket X-Amz-Signature=abc")
	}

	h := &Handler{}
	for _, path := range []string{"/readyz", "/healthz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		if path == "/readyz" {
			h.Readyz(rec, req)
		} else {
			h.Healthz(rec, req)
		}
		body := rec.Body.String()
		for _, forbidden := range []string{"secret", "/srv/skadia", "private-bucket", "X-Amz-Signature"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s leaked %q: %s", path, forbidden, body)
			}
		}
	}
}

func TestAdminSystemStatusIncludesDataPlaneComponents(t *testing.T) {
	h := newTestHandler(t, fusionTestPool(t))
	h.Config = &HandlerConfig{
		DeploymentProfile: "full",
		StorageBackend:    "local",
		StorageMode:       "local",
		APIReplicaCount:   1,
		SharedStorage:     "false",
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system-status", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-test"))
	rec := httptest.NewRecorder()

	h.GetAdminSystemStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	for _, key := range []string{`"db"`, `"timescale_compression"`, `"storage"`, `"jobs"`, `"gdpr_artifacts"`} {
		if !strings.Contains(rec.Body.String(), key) {
			t.Fatalf("response missing component %s: %s", key, rec.Body.String())
		}
	}
}

func TestAdminSystemStatusUsesInjectedS3StorageProbeResults(t *testing.T) {
	h := newTestHandler(t, fusionTestPool(t))
	h.Config = &HandlerConfig{
		DeploymentProfile: "full",
		StorageBackend:    "s3",
		StorageMode:       "s3-local-garage",
		APIReplicaCount:   1,
		SharedStorage:     "true",
	}
	h.StorageProbe = fakeAdminStatusStorageProbe{head: true, write: true, list: false, multipart: true}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system-status", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-test"))
	rec := httptest.NewRecorder()

	h.GetAdminSystemStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"reason_code":"s3_list_prefix_probe_failed"`) {
		t.Fatalf("response missing S3 list-prefix probe failure: %s", rec.Body.String())
	}
}

func TestAdminSystemStatusIncludesServiceAndGeodataComponents(t *testing.T) {
	h := newTestHandler(t, fusionTestPool(t))
	h.Config = &HandlerConfig{DeploymentProfile: "full", StorageBackend: "local", StorageMode: "local"}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system-status", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-test"))
	rec := httptest.NewRecorder()

	h.GetAdminSystemStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	for _, key := range []string{`"nats"`, `"ml"`, `"brouter"`, `"badrouter"`, `"osm"`, `"dem"`, `"coverage"`, `"tiles"`} {
		if !strings.Contains(rec.Body.String(), key) {
			t.Fatalf("response missing component %s: %s", key, rec.Body.String())
		}
	}
}

type fakeAdminStatusStorageProbe struct {
	head, write, list, multipart bool
}

func (f fakeAdminStatusStorageProbe) HeadBucket(context.Context) error {
	if f.head {
		return nil
	}
	return errors.New("head failed")
}

func (f fakeAdminStatusStorageProbe) PutGetDelete(context.Context) error {
	if f.write {
		return nil
	}
	return errors.New("write failed")
}

func (f fakeAdminStatusStorageProbe) ListPrefix(context.Context) error {
	if f.list {
		return nil
	}
	return errors.New("list failed")
}

func (f fakeAdminStatusStorageProbe) MultipartAbortDryRun(context.Context) error {
	if f.multipart {
		return nil
	}
	return errors.New("multipart failed")
}
