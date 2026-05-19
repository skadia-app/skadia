package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/systemstatus"
	"github.com/go-chi/chi/v5"
	natsgo "github.com/nats-io/nats.go"
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
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
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

func TestAdminSystemStatusSkipsWriteDeleteProbeDuringMaintenance(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	h.Config = &HandlerConfig{
		DeploymentProfile: "full",
		StorageBackend:    "s3",
		StorageMode:       "s3-local-garage",
		APIReplicaCount:   1,
		SharedStorage:     "true",
	}
	probe := &countingAdminStatusStorageProbe{head: true, write: true, list: true, multipart: true}
	h.StorageProbe = probe
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO system_flags (key, value, expires_at)
		VALUES ('maintenance_mode', 'storage migration in progress', 'infinity'::timestamptz)
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value, set_at = NOW(), expires_at = EXCLUDED.expires_at`); err != nil {
		t.Fatalf("set maintenance_mode: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM system_flags WHERE key = 'maintenance_mode'`)
	})

	got := h.collectStorageComponent(context.Background())

	if probe.writeCalls != 0 {
		t.Fatalf("PutGetDelete calls during maintenance = %d, want 0", probe.writeCalls)
	}
	if got.State != systemstatus.StatusOK {
		t.Fatalf("storage status = %+v, want OK when read-only probes pass during maintenance", got)
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

func TestCollectNATSComponentTreatsConfiguredMissingConnectionAsUnavailable(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{NatsURL: "nats://nats:4222"}}
	got := h.collectNATSComponent(context.Background(), false)
	if got.State != "unavailable" || got.ReasonCode != "nats_unavailable" {
		t.Fatalf("NATS status = %+v, want unavailable nats_unavailable", got)
	}
}

func TestCollectNATSJetStreamSnapshotAcceptsWildcardSkadiaStream(t *testing.T) {
	js := fakeNATSInspector{
		subjectStreams: map[string]string{
			"skadia.photo.embed":          "SKADIA",
			"skadia.stage2.high":          "SKADIA",
			"skadia.document.uploaded":    "SKADIA",
			"skadia.analytics.invalidate": "SKADIA",
		},
		streams: map[string]bool{"SKADIA": true},
		consumers: map[string]map[string]natsgo.ConsumerInfo{
			"SKADIA": {
				"ml-stage2-high":          {NumPending: 2},
				"ml-stage2-normal":        {},
				"ml-stage2-low":           {},
				"ml-document-processor":   {},
				"ml-analytics-invalidate": {NumPending: 3},
				"ml-photo-embed":          {},
			},
		},
	}
	snap := systemstatus.NATSSnapshot{Configured: true, Connected: true}

	collectNATSJetStreamSnapshot(&snap, js, true)
	got := systemstatus.NATSStatusFromSnapshot(snap)

	if got.State != systemstatus.StatusOK {
		t.Fatalf("NATS status = %+v, want ok for consumers on wildcard SKADIA stream", got)
	}
	if got.Details["pending_message_count"] != "5" {
		t.Fatalf("pending_message_count = %q, want 5", got.Details["pending_message_count"])
	}
}

func TestCollectOSMComponentDegradesOnFailedRegion(t *testing.T) {
	h := newTestHandler(t, fusionTestPool(t))
	h.Config = &HandlerConfig{DeploymentProfile: "full"}
	_, err := h.DB.Pool.Exec(context.Background(), `
		INSERT INTO osm_import_regions (region_id, label, continent, status)
		VALUES ('ok-region', 'OK Region', 'Test', 'imported'),
		       ('failed-region', 'Failed Region', 'Test', 'failed')`)
	if err != nil {
		t.Fatalf("insert osm_import_regions: %v", err)
	}
	caps := capabilities.Capabilities{Features: map[string]capabilities.FeatureState{
		capabilities.KeyOSMImport: {Configured: true, Available: true},
	}}

	got := h.collectOSMComponent(context.Background(), caps)

	if got.State != systemstatus.StatusDegraded || got.ReasonCode != "osm_regions_failed" {
		t.Fatalf("OSM status = %+v, want degraded osm_regions_failed", got)
	}
	if got.Details["failed_region_count"] != "1" || got.Details["imported_region_count"] != "1" {
		t.Fatalf("OSM status details = %+v, want imported=1 failed=1", got.Details)
	}
}

func TestCollectMLSnapshotUsesInternalStatusForBuildAndRoles(t *testing.T) {
	ml := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/livez", "/readyz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/status":
			if r.Header.Get("X-ML-Callback-Secret") != "status-secret" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"service":"ml","git_commit":"abc123","commit_time":"2026-05-17T09:58:00Z","build_time":"2026-05-17T10:00:00Z","config_version":"1","metrics":{"processed":2},"llm_broker":{"status":"degraded","roles":{"document_extract":"configured","vision":"not_configured"}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ml.Close)
	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "full", MLServiceURL: ml.URL, MLCallbackSecret: "status-secret"}}

	snap, build := h.collectMLSnapshot(context.Background())
	got := systemstatus.MLStatusFromSnapshot(snap)

	if build == nil || build.GitCommit != "abc123" || snap.MissingRoles == nil {
		t.Fatalf("ML snapshot/build incomplete: snap=%+v build=%+v", snap, build)
	}
	if got.State != "degraded" || got.ReasonCode != "ml_roles_unavailable" {
		t.Fatalf("ML status = %+v, want degraded ml_roles_unavailable", got)
	}
}

func TestAdminSystemStatusKeepsMLBuildRowWhenInternalStatusUnavailable(t *testing.T) {
	ml := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/livez", "/readyz":
			w.WriteHeader(http.StatusOK)
		case "/status":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ml.Close)
	h := newTestHandler(t, fusionTestPool(t))
	h.Config = &HandlerConfig{DeploymentProfile: "full", MLServiceURL: ml.URL, MLCallbackSecret: "status-secret"}

	got := h.collectAdminSystemStatus(context.Background())

	if got.Build["ml"].Service != "ml" {
		t.Fatalf("ML build row = %+v, want placeholder service=ml", got.Build["ml"])
	}
	if got.Components["ml"].State != systemstatus.StatusDegraded || got.Components["ml"].ReasonCode != "ml_status_unavailable" {
		t.Fatalf("ML component = %+v, want degraded ml_status_unavailable", got.Components["ml"])
	}
}

type fakeNATSInspector struct {
	subjectStreams map[string]string
	streams        map[string]bool
	consumers      map[string]map[string]natsgo.ConsumerInfo
}

func (f fakeNATSInspector) StreamInfo(stream string, opts ...natsgo.JSOpt) (*natsgo.StreamInfo, error) {
	if f.streams[stream] {
		return &natsgo.StreamInfo{Config: natsgo.StreamConfig{Name: stream}}, nil
	}
	return nil, errors.New("stream not found")
}

func (f fakeNATSInspector) StreamNameBySubject(subject string, opts ...natsgo.JSOpt) (string, error) {
	if stream := f.subjectStreams[subject]; stream != "" {
		return stream, nil
	}
	return "", errors.New("stream not found")
}

func (f fakeNATSInspector) ConsumerInfo(stream, consumer string, opts ...natsgo.JSOpt) (*natsgo.ConsumerInfo, error) {
	byConsumer := f.consumers[stream]
	if byConsumer == nil {
		return nil, errors.New("consumer not found")
	}
	info, ok := byConsumer[consumer]
	if !ok {
		return nil, errors.New("consumer not found")
	}
	return &info, nil
}

func TestCollectMLSnapshotDegradesWhenInternalStatusAuthUnavailable(t *testing.T) {
	ml := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/livez", "/readyz":
			w.WriteHeader(http.StatusOK)
		case "/status":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ml.Close)
	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "full", MLServiceURL: ml.URL, MLCallbackSecret: "status-secret"}}

	snap, build := h.collectMLSnapshot(context.Background())
	got := systemstatus.MLStatusFromSnapshot(snap)

	if build != nil {
		t.Fatalf("build = %+v, want nil when /status is unavailable", build)
	}
	if got.State != "degraded" || got.ReasonCode != "ml_status_unavailable" {
		t.Fatalf("ML status = %+v, want degraded ml_status_unavailable", got)
	}
}

func TestShadowFSSystemStatusProbeChecksDistinctS3Buckets(t *testing.T) {
	backend := &recordingStatusBackend{}
	probe := newShadowFSSystemStatusProbe(backend, func(fileType string) string {
		switch fileType {
		case "activity":
			return "activity-bucket"
		case "media", "document":
			return "media-bucket"
		default:
			return "default-bucket"
		}
	})

	if err := probe.HeadBucket(context.Background()); err != nil {
		t.Fatalf("HeadBucket: %v", err)
	}
	if err := probe.PutGetDelete(context.Background()); err != nil {
		t.Fatalf("PutGetDelete: %v", err)
	}
	if err := probe.ListPrefix(context.Background()); err != nil {
		t.Fatalf("ListPrefix: %v", err)
	}
	if err := probe.MultipartAbortDryRun(context.Background()); err != nil {
		t.Fatalf("MultipartAbortDryRun: %v", err)
	}

	for _, bucket := range []string{"activity-bucket", "media-bucket", "default-bucket"} {
		if backend.pinged[bucket] == 0 || backend.written[bucket] == 0 || backend.listed[bucket] == 0 || backend.multipart[bucket] == 0 {
			t.Fatalf("bucket %q was not fully probed: %+v", bucket, backend)
		}
	}
}

func TestCollectGDPRArtifactsDegradesOnCounterError(t *testing.T) {
	h := &Handler{GDPRArtifactCounter: failingArtifactCounter{}}
	got := h.collectGDPRArtifactComponent(context.Background())
	if got.State != systemstatus.StatusDegraded || got.ReasonCode != "artifact_store_probe_failed" {
		t.Fatalf("GDPR artifact status = %+v, want degraded artifact_store_probe_failed", got)
	}
}

func TestCoreProfileMarksFullOnlyGeodataComponentsDisabled(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "core", DeploymentProfileSource: "env"}}
	caps := h.capabilitiesForStatus(context.Background())
	for name, got := range map[string]systemstatus.ComponentStatus{
		"badrouter": h.collectBadRouterComponent(context.Background(), caps),
		"osm":       h.collectOSMComponent(context.Background(), caps),
		"dem":       h.collectDEMComponent(context.Background(), caps),
		"coverage":  h.collectCoverageComponent(context.Background(), caps),
		"tiles":     h.collectTilesComponent(context.Background(), caps),
	} {
		if got.State != systemstatus.StatusDisabled || got.ReasonCode != "core_profile" {
			t.Fatalf("%s status = %+v, want disabled core_profile", name, got)
		}
	}
}

type fakeAdminStatusStorageProbe struct {
	head, write, list, multipart bool
}

type countingAdminStatusStorageProbe struct {
	head, write, list, multipart bool
	writeCalls                   int
}

type failingArtifactCounter struct{}

func (failingArtifactCounter) CountIncompleteUploads(context.Context, gdprartifacts.ArtifactPrefix) (int, error) {
	return 0, errors.New("bucket=private-artifacts permission denied")
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

func (f *countingAdminStatusStorageProbe) HeadBucket(context.Context) error {
	if f.head {
		return nil
	}
	return errors.New("head failed")
}

func (f *countingAdminStatusStorageProbe) PutGetDelete(context.Context) error {
	f.writeCalls++
	if f.write {
		return nil
	}
	return errors.New("write failed")
}

func (f *countingAdminStatusStorageProbe) ListPrefix(context.Context) error {
	if f.list {
		return nil
	}
	return errors.New("list failed")
}

func (f *countingAdminStatusStorageProbe) MultipartAbortDryRun(context.Context) error {
	if f.multipart {
		return nil
	}
	return errors.New("multipart failed")
}

type recordingStatusBackend struct {
	pinged    map[string]int
	written   map[string]int
	listed    map[string]int
	multipart map[string]int
}

func (r *recordingStatusBackend) ensure() {
	if r.pinged == nil {
		r.pinged = map[string]int{}
		r.written = map[string]int{}
		r.listed = map[string]int{}
		r.multipart = map[string]int{}
	}
}

func (r *recordingStatusBackend) Read(context.Context, string, *shadowfs.BackendOpts) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("ok")), nil
}

func (r *recordingStatusBackend) Write(_ context.Context, _ string, data io.Reader, _ string, _ bool, opts *shadowfs.BackendOpts) (int64, string, error) {
	r.ensure()
	bucket := bucketFromOpts(opts)
	r.written[bucket]++
	n, _ := io.Copy(io.Discard, data)
	return n, "hash", nil
}

func (r *recordingStatusBackend) Delete(context.Context, string, *shadowfs.BackendOpts) error {
	return nil
}

func (r *recordingStatusBackend) Exists(context.Context, string, *shadowfs.BackendOpts) (bool, error) {
	return true, nil
}

func (r *recordingStatusBackend) DeletePrefix(context.Context, string, *shadowfs.BackendOpts) error {
	return nil
}

func (r *recordingStatusBackend) ListFunc(_ context.Context, _ string, opts *shadowfs.BackendOpts, fn func(string) error) error {
	r.ensure()
	r.listed[bucketFromOpts(opts)]++
	return fn(".skadia/status-probe/test")
}

func (r *recordingStatusBackend) PingBucket(_ context.Context, bucket string) error {
	r.ensure()
	r.pinged[bucket]++
	return nil
}

func (r *recordingStatusBackend) ListMultipartUploads(_ context.Context, bucket, _ string) error {
	r.ensure()
	r.multipart[bucket]++
	return nil
}

func bucketFromOpts(opts *shadowfs.BackendOpts) string {
	if opts == nil {
		return ""
	}
	return opts.Bucket
}
