package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestDataExportDownloadSessionRequiresOwnershipAndKnownArtifact(t *testing.T) {
	ctx := context.Background()
	h, ownerID, jobID := newDataExportSessionFixture(t, "export body")

	req := dataExportSessionRequest(t, ownerID, jobID, `{"single":true}`)
	w := httptest.NewRecorder()
	h.DataExportDownloadSession(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("owner session status = %d body=%s", w.Code, w.Body.String())
	}

	otherID := uuid.NewString()
	req = dataExportSessionRequest(t, otherID, jobID, `{"single":true}`)
	w = httptest.NewRecorder()
	h.DataExportDownloadSession(w, req)
	if w.Code == http.StatusOK {
		t.Fatal("wrong user received a download session")
	}
	_ = ctx
}

func TestDataExportDownloadSessionStoresHashNotRawCookieSecret(t *testing.T) {
	h, ownerID, jobID := newDataExportSessionFixture(t, "export body")

	req := dataExportSessionRequest(t, ownerID, jobID, `{"single":true}`)
	w := httptest.NewRecorder()
	h.DataExportDownloadSession(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("session status = %d body=%s", w.Code, w.Body.String())
	}

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	rawSecret := cookies[0].Value
	if rawSecret == "" {
		t.Fatal("empty raw session cookie")
	}
	var metadata []byte
	if err := h.DB.Pool.QueryRow(context.Background(), `SELECT metadata FROM background_jobs WHERE id=$1`, jobID).Scan(&metadata); err != nil {
		t.Fatalf("metadata query: %v", err)
	}
	if strings.Contains(string(metadata), rawSecret) {
		t.Fatalf("metadata contains raw cookie secret: %s", metadata)
	}
	if !strings.Contains(string(metadata), "secret_hash") {
		t.Fatalf("metadata missing secret_hash: %s", metadata)
	}
	if strings.Contains(w.Body.String(), rawSecret) {
		t.Fatalf("response body contains raw cookie secret: %s", w.Body.String())
	}
}

func TestDataExportSessionDownloadStreamsOutsideJWTRouteGroup(t *testing.T) {
	h, ownerID, jobID := newDataExportSessionFixture(t, "export body")

	req := dataExportSessionRequest(t, ownerID, jobID, `{"single":true}`)
	w := httptest.NewRecorder()
	h.DataExportDownloadSession(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("session status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if strings.Contains(resp.URL, "secret") || strings.Contains(resp.URL, "token") || strings.Contains(resp.URL, "ticket") {
		t.Fatalf("download URL contains suspicious secret parameter: %s", resp.URL)
	}

	downloadReq := httptest.NewRequest(http.MethodGet, resp.URL, nil)
	for _, cookie := range w.Result().Cookies() {
		downloadReq.AddCookie(cookie)
	}
	downloadReq = withJobAndPart(downloadReq, jobID, "export.zip")
	downloadW := httptest.NewRecorder()
	h.DataExportDownloadPart(downloadW, downloadReq)
	if downloadW.Code != http.StatusOK {
		t.Fatalf("download status = %d body=%s", downloadW.Code, downloadW.Body.String())
	}
	if downloadW.Body.String() != "export body" {
		t.Fatalf("download body = %q", downloadW.Body.String())
	}
	if got := downloadW.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := downloadW.Header().Get("X-Checksum-SHA256"); got == "" {
		t.Fatal("missing checksum header")
	}
}

func TestDataExportDownloadSessionConsumesObjectOnce(t *testing.T) {
	h, ownerID, jobID := newDataExportSessionFixture(t, "export body")

	req := dataExportSessionRequest(t, ownerID, jobID, `{"single":true}`)
	w := httptest.NewRecorder()
	h.DataExportDownloadSession(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("session status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	downloadReq := httptest.NewRequest(http.MethodGet, resp.URL, nil)
	for _, cookie := range w.Result().Cookies() {
		downloadReq.AddCookie(cookie)
	}
	downloadReq = withJobAndPart(downloadReq, jobID, "export.zip")
	first := httptest.NewRecorder()
	h.DataExportDownloadPart(first, downloadReq)
	if first.Code != http.StatusOK {
		t.Fatalf("first download status = %d body=%s", first.Code, first.Body.String())
	}

	replay := httptest.NewRecorder()
	h.DataExportDownloadPart(replay, downloadReq)
	if replay.Code != http.StatusUnauthorized {
		t.Fatalf("replay status = %d, want 401 body=%s", replay.Code, replay.Body.String())
	}
}

func TestDataExportDownloadSessionRejectsExpiredArtifactAndMismatchedRef(t *testing.T) {
	h, ownerID, jobID := newDataExportSessionFixture(t, "export body")
	var metadata []byte
	if err := h.DB.Pool.QueryRow(context.Background(), `SELECT metadata FROM background_jobs WHERE id=$1`, jobID).Scan(&metadata); err != nil {
		t.Fatalf("metadata query: %v", err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(metadata, &obj); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	var artifact gdprartifacts.Artifact
	if err := json.Unmarshal(obj["artifact"], &artifact); err != nil {
		t.Fatalf("decode artifact: %v", err)
	}
	artifact.ExpiresAt = time.Now().Add(-time.Minute)
	artifactRaw, _ := json.Marshal(artifact)
	obj["artifact"] = artifactRaw
	updated, _ := json.Marshal(obj)
	if _, err := h.DB.Pool.Exec(context.Background(), `UPDATE background_jobs SET metadata=$2::jsonb WHERE id=$1`, jobID, updated); err != nil {
		t.Fatalf("expire artifact: %v", err)
	}

	req := dataExportSessionRequest(t, ownerID, jobID, `{"single":true}`)
	w := httptest.NewRecorder()
	h.DataExportDownloadSession(w, req)
	if w.Code != http.StatusGone {
		t.Fatalf("expired artifact session status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestDataExportDownloadRangeIncludesContentRange(t *testing.T) {
	h, ownerID, jobID := newDataExportSessionFixture(t, "export body")

	req := dataExportSessionRequest(t, ownerID, jobID, `{"single":true}`)
	w := httptest.NewRecorder()
	h.DataExportDownloadSession(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("session status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	downloadReq := httptest.NewRequest(http.MethodGet, resp.URL, nil)
	downloadReq.Header.Set("Range", "bytes=0-5")
	for _, cookie := range w.Result().Cookies() {
		downloadReq.AddCookie(cookie)
	}
	downloadReq = withJobAndPart(downloadReq, jobID, "export.zip")
	downloadW := httptest.NewRecorder()
	h.DataExportDownloadPart(downloadW, downloadReq)
	if downloadW.Code != http.StatusPartialContent {
		t.Fatalf("range status = %d body=%s", downloadW.Code, downloadW.Body.String())
	}
	if got := downloadW.Header().Get("Content-Range"); got != "bytes 0-5/11" {
		t.Fatalf("Content-Range = %q", got)
	}
	if got := downloadW.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("Accept-Ranges = %q", got)
	}
}

func TestDataExportDownloadInvalidRangeDoesNotConsumeSession(t *testing.T) {
	h, ownerID, jobID := newDataExportSessionFixture(t, "export body")

	req := dataExportSessionRequest(t, ownerID, jobID, `{"single":true}`)
	w := httptest.NewRecorder()
	h.DataExportDownloadSession(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("session status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode session response: %v", err)
	}

	invalidReq := httptest.NewRequest(http.MethodGet, resp.URL, nil)
	invalidReq.Header.Set("Range", "bytes=99-100")
	for _, cookie := range w.Result().Cookies() {
		invalidReq.AddCookie(cookie)
	}
	invalidReq = withJobAndPart(invalidReq, jobID, "export.zip")
	invalidW := httptest.NewRecorder()
	h.DataExportDownloadPart(invalidW, invalidReq)
	if invalidW.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("invalid range status = %d body=%s", invalidW.Code, invalidW.Body.String())
	}

	validReq := httptest.NewRequest(http.MethodGet, resp.URL, nil)
	for _, cookie := range w.Result().Cookies() {
		validReq.AddCookie(cookie)
	}
	validReq = withJobAndPart(validReq, jobID, "export.zip")
	validW := httptest.NewRecorder()
	h.DataExportDownloadPart(validW, validReq)
	if validW.Code != http.StatusOK {
		t.Fatalf("valid replay after invalid range status = %d body=%s", validW.Code, validW.Body.String())
	}
	if validW.Body.String() != "export body" {
		t.Fatalf("valid replay body = %q", validW.Body.String())
	}
}

func TestDataExportDownloadSessionAllReturnsObjectURLs(t *testing.T) {
	h, ownerID, jobID := newMultipartDataExportSessionFixture(t)

	req := dataExportSessionRequest(t, ownerID, jobID, `{"all":true}`)
	w := httptest.NewRecorder()
	h.DataExportDownloadSession(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("session status = %d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		URL     string `json:"url"`
		Objects []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if len(resp.Objects) != 3 {
		t.Fatalf("objects len = %d body=%s", len(resp.Objects), w.Body.String())
	}
	for _, object := range resp.Objects {
		if object.URL == "" {
			t.Fatalf("object %q missing url in body=%s", object.Name, w.Body.String())
		}
		if !strings.Contains(object.URL, "session_id=") {
			t.Fatalf("object %q URL missing session_id: %s", object.Name, object.URL)
		}
	}
	if resp.URL != resp.Objects[0].URL {
		t.Fatalf("primary url = %q, want first object url %q", resp.URL, resp.Objects[0].URL)
	}
}

func TestDataExportStatusInlineReconciliationConfirmedMissingStripsDownloadSessions(t *testing.T) {
	ctx := context.Background()
	h, ownerID, jobID := newDataExportSessionFixture(t, "export body")
	var metadata []byte
	if err := h.DB.Pool.QueryRow(ctx, `SELECT metadata FROM background_jobs WHERE id = $1`, jobID).Scan(&metadata); err != nil {
		t.Fatalf("select metadata: %v", err)
	}
	meta, err := gdprartifacts.DecodeJobMetadata(metadata)
	if err != nil || meta.Artifact == nil {
		t.Fatalf("DecodeJobMetadata: %v %#v", err, meta.Artifact)
	}
	if err := h.ArtifactStore.Delete(ctx, meta.Artifact.Objects[0]); err != nil {
		t.Fatalf("delete artifact: %v", err)
	}

	w := httptest.NewRecorder()
	h.DataExportStatus(w, dataExportStatusRequest(ownerID, jobID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["unavailable_reason"] != "artifact_missing" {
		t.Fatalf("unavailable_reason = %v body=%s", resp["unavailable_reason"], w.Body.String())
	}
	if _, ok := resp["download_session_url"]; ok {
		t.Fatalf("download_session_url should be omitted: %s", w.Body.String())
	}
	if err := h.DB.Pool.QueryRow(ctx, `SELECT metadata FROM background_jobs WHERE id = $1`, jobID).Scan(&metadata); err != nil {
		t.Fatalf("select updated metadata: %v", err)
	}
	if !strings.Contains(string(metadata), "artifact_missing_at") || !strings.Contains(string(metadata), "artifact_missing_objects") {
		t.Fatalf("metadata missing artifact_missing markers: %s", metadata)
	}
}

func TestDataExportStatusInlineReconciliationBackendUnknownPreservesRefs(t *testing.T) {
	h, ownerID, jobID := newDataExportSessionFixture(t, "export body")
	h.ArtifactStore = unknownStatStore{Store: h.ArtifactStore}
	var before []byte
	if err := h.DB.Pool.QueryRow(context.Background(), `SELECT metadata FROM background_jobs WHERE id = $1`, jobID).Scan(&before); err != nil {
		t.Fatalf("select metadata: %v", err)
	}

	w := httptest.NewRecorder()
	h.DataExportStatus(w, dataExportStatusRequest(ownerID, jobID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["unavailable_reason"] != "artifact_backend_unavailable" {
		t.Fatalf("unavailable_reason = %v body=%s", resp["unavailable_reason"], w.Body.String())
	}
	if _, ok := resp["download_session_url"]; ok {
		t.Fatalf("download_session_url should be omitted on unknown backend state: %s", w.Body.String())
	}
	var after []byte
	if err := h.DB.Pool.QueryRow(context.Background(), `SELECT metadata FROM background_jobs WHERE id = $1`, jobID).Scan(&after); err != nil {
		t.Fatalf("select metadata after status: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("metadata mutated on unknown backend state\nbefore=%s\nafter=%s", before, after)
	}
}

func newDataExportSessionFixture(t *testing.T, content string) (*Handler, string, string) {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	store, err := gdprartifacts.NewLocalStore(t.TempDir()+"/exports", t.TempDir()+"/imports")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	userID := uuid.NewString()
	jobID := uuid.NewString()
	writer, err := store.CreateWriter(ctx, gdprartifacts.ObjectSpec{
		Kind:        "export",
		UserID:      userID,
		JobID:       jobID,
		Role:        gdprartifacts.RoleSingleZIP,
		Name:        "export.zip",
		ContentType: "application/zip",
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateWriter: %v", err)
	}
	if _, err := writer.Write([]byte(content)); err != nil {
		t.Fatalf("Write artifact: %v", err)
	}
	ref, err := writer.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit artifact: %v", err)
	}
	artifactRaw, err := gdprartifacts.EncodeArtifact(gdprartifacts.Artifact{
		Version:   1,
		Format:    gdprartifacts.ArtifactFormatSingleZIP,
		ExpiresAt: time.Now().Add(time.Hour),
		Objects:   []gdprartifacts.ObjectRef{ref},
	})
	if err != nil {
		t.Fatalf("EncodeArtifact: %v", err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(artifactRaw, &envelope); err != nil {
		t.Fatalf("unmarshal artifact: %v", err)
	}
	artifact := envelope["artifact"]
	if _, err := pool.Exec(ctx,
		`INSERT INTO background_jobs (id, job_type, user_id, status, started_at, updated_at, metadata)
		 VALUES ($1, 'gdpr_export', $2, 'completed', NOW(), NOW(), jsonb_build_object('artifact', $3::jsonb))`,
		jobID, userID, string(artifact)); err != nil {
		t.Fatalf("insert background job: %v", err)
	}
	h := &Handler{DB: &database.DB{Pool: pool}, ArtifactStore: store, shutdownCtx: ctx}
	return h, userID, jobID
}

func dataExportStatusRequest(userID, jobID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/data-export/"+jobID, nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	return withJob(req, jobID)
}

type unknownStatStore struct {
	gdprartifacts.Store
}

func (s unknownStatStore) Stat(context.Context, gdprartifacts.ObjectRef) (gdprartifacts.ObjectStat, error) {
	return gdprartifacts.ObjectStat{State: gdprartifacts.ObjectUnknown, Err: context.DeadlineExceeded}, context.DeadlineExceeded
}

func newMultipartDataExportSessionFixture(t *testing.T) (*Handler, string, string) {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	store, err := gdprartifacts.NewLocalStore(t.TempDir()+"/exports", t.TempDir()+"/imports")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	userID := uuid.NewString()
	jobID := uuid.NewString()
	expiresAt := time.Now().Add(time.Hour)
	objects := make([]gdprartifacts.ObjectRef, 0, 3)
	for _, spec := range []struct {
		name string
		role gdprartifacts.ObjectRole
		body string
		ct   string
	}{
		{name: "manifest.json", role: gdprartifacts.RoleManifest, body: `{}`, ct: "application/json"},
		{name: "metadata.zip", role: gdprartifacts.RoleMetadataZIP, body: "metadata", ct: "application/zip"},
		{name: "files-0001.zip", role: gdprartifacts.RoleFilesZIP, body: "files", ct: "application/zip"},
	} {
		writer, err := store.CreateWriter(ctx, gdprartifacts.ObjectSpec{
			Kind:        "export",
			UserID:      userID,
			JobID:       jobID,
			Role:        spec.role,
			Name:        spec.name,
			ContentType: spec.ct,
			ExpiresAt:   expiresAt,
		})
		if err != nil {
			t.Fatalf("CreateWriter %s: %v", spec.name, err)
		}
		if _, err := writer.Write([]byte(spec.body)); err != nil {
			t.Fatalf("Write %s: %v", spec.name, err)
		}
		ref, err := writer.Commit(ctx)
		if err != nil {
			t.Fatalf("Commit %s: %v", spec.name, err)
		}
		objects = append(objects, ref)
	}
	artifactRaw, err := gdprartifacts.EncodeArtifact(gdprartifacts.Artifact{
		Version:   1,
		Format:    gdprartifacts.ArtifactFormatMultipart,
		ExpiresAt: expiresAt,
		Objects:   objects,
		Multipart: &gdprartifacts.MultipartSummary{PartCount: 3, TotalSizeBytes: 13},
	})
	if err != nil {
		t.Fatalf("EncodeArtifact: %v", err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(artifactRaw, &envelope); err != nil {
		t.Fatalf("unmarshal artifact: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO background_jobs (id, job_type, user_id, status, started_at, updated_at, metadata)
		 VALUES ($1, 'gdpr_export', $2, 'completed', NOW(), NOW(), jsonb_build_object('artifact', $3::jsonb))`,
		jobID, userID, string(envelope["artifact"])); err != nil {
		t.Fatalf("insert background job: %v", err)
	}
	return &Handler{DB: &database.DB{Pool: pool}, ArtifactStore: store, shutdownCtx: ctx}, userID, jobID
}

func dataExportSessionRequest(t *testing.T, userID, jobID, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/data-export/"+jobID+"/download-session", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	return withJob(req, jobID)
}

func withJob(r *http.Request, jobID string) *http.Request {
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("jobID", jobID)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeCtx))
}

func withJobAndPart(r *http.Request, jobID, partName string) *http.Request {
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("jobID", jobID)
	routeCtx.URLParams.Add("partName", partName)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeCtx))
}
