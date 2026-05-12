package auth

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/config"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDataImportStoresSingleZipInArtifactStoreNotTmp(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	clearActiveDataImportJobsForTest(t, pool)
	userID := createArtifactExportUser(t, pool)
	store, err := gdprartifacts.NewLocalStore(t.TempDir()+"/exports", t.TempDir()+"/imports")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	h := &Handler{
		DB:            &database.DB{Pool: pool},
		Config:        &config.Config{},
		ArtifactStore: store,
		shutdownCtx:   canceledContext(),
	}

	req := newDataImportUploadRequest(t, userID, []byte("not a zip"))
	w := httptest.NewRecorder()
	h.DataImport(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("DataImport status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	var metadata []byte
	if err := pool.QueryRow(ctx, `SELECT metadata FROM background_jobs WHERE id = $1`, resp.JobID).Scan(&metadata); err != nil {
		t.Fatalf("select metadata: %v", err)
	}
	if containsJSONKey(metadata, "file_path") {
		t.Fatalf("new import job wrote legacy file_path metadata: %s", metadata)
	}
	meta, err := gdprartifacts.DecodeJobMetadata(metadata)
	if err != nil {
		t.Fatalf("DecodeJobMetadata: %v", err)
	}
	if meta.Artifact == nil || meta.Artifact.Format != gdprartifacts.ArtifactFormatSingleZIP {
		t.Fatalf("artifact metadata = %#v", meta.Artifact)
	}
	if len(meta.Artifact.Objects) != 1 || meta.Artifact.Objects[0].Kind != "import" {
		t.Fatalf("import artifact object missing: %#v", meta.Artifact.Objects)
	}
	stat, err := store.Stat(ctx, meta.Artifact.Objects[0])
	if err != nil {
		t.Fatalf("artifact Stat: %v", err)
	}
	if stat.State != gdprartifacts.ObjectPresent {
		t.Fatalf("artifact state = %s, want present", stat.State)
	}
}

func TestRunImportProgressMergesImportProgressWithoutReplacingArtifact(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	clearActiveDataImportJobsForTest(t, pool)
	userID := createArtifactExportUser(t, pool)
	store, artifact, zipPath := stageSingleImportArtifactForTest(t, ctx, userID, minimalImportZip(t), false)
	jobID := artifact.Objects[0].JobID
	h := &Handler{
		DB:            &database.DB{Pool: pool},
		Config:        &config.Config{},
		ArtifactStore: store,
		shutdownCtx:   ctx,
	}

	h.runImportJob(jobID, userID, zipPath)

	var metadata []byte
	if err := pool.QueryRow(ctx, `SELECT metadata FROM background_jobs WHERE id = $1`, jobID).Scan(&metadata); err != nil {
		t.Fatalf("select metadata: %v", err)
	}
	meta, err := gdprartifacts.DecodeJobMetadata(metadata)
	if err != nil {
		t.Fatalf("DecodeJobMetadata: %v", err)
	}
	if meta.Artifact == nil {
		t.Fatalf("artifact metadata was replaced: %s", metadata)
	}
	if meta.ImportProgress == nil {
		t.Fatalf("import_progress missing after import: %s", metadata)
	}
	if meta.ImportResult == nil {
		t.Fatalf("import_result missing after import: %s", metadata)
	}
}

func TestSingleZipImportOpensArtifactSourceForS3Backend(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	clearActiveDataImportJobsForTest(t, pool)
	userID := createArtifactExportUser(t, pool)
	store, artifact, zipPath := stageSingleImportArtifactForTest(t, ctx, userID, minimalImportZip(t), true)
	jobID := artifact.Objects[0].JobID
	h := &Handler{
		DB:            &database.DB{Pool: pool},
		Config:        &config.Config{},
		ArtifactStore: store,
		shutdownCtx:   ctx,
	}

	h.runImportJob(jobID, userID, zipPath)

	if !store.openReaderAtCalled {
		t.Fatal("single ZIP import did not open artifact through OpenReaderAt")
	}
}

func TestCompletedImportDeletesStagedArtifact(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	clearActiveDataImportJobsForTest(t, pool)
	userID := createArtifactExportUser(t, pool)
	store, artifact, zipPath := stageSingleImportArtifactForTest(t, ctx, userID, minimalImportZip(t), false)
	jobID := artifact.Objects[0].JobID
	h := &Handler{
		DB:            &database.DB{Pool: pool},
		Config:        &config.Config{},
		ArtifactStore: store.LocalStore,
		shutdownCtx:   ctx,
	}

	h.runImportJob(jobID, userID, zipPath)

	stat, err := store.Stat(ctx, artifact.Objects[0])
	if err != nil {
		t.Fatalf("artifact Stat: %v", err)
	}
	if stat.State != gdprartifacts.ObjectMissing {
		t.Fatalf("artifact state = %s, want missing after completed import", stat.State)
	}
}

func TestMultipartImportCreateSessionValidatesManifest(t *testing.T) {
	h, userID, _ := newMultipartImportHandlerForTest(t)

	badReq := newJSONAuthRequest(t, userID, http.MethodPost, "/api/v1/auth/data-import/multipart", `{"format_version":1,"multipart":false}`)
	badW := httptest.NewRecorder()
	h.DataImportMultipartCreate(badW, badReq)
	if badW.Code != http.StatusBadRequest {
		t.Fatalf("non-multipart manifest status = %d body=%s", badW.Code, badW.Body.String())
	}

	body := multipartImportManifestJSON(t, []multipartExpectedPart{
		{name: "metadata.zip", role: "metadata", data: minimalImportZip(t)},
		{name: "files-0001.zip", role: "files", data: zipWithEntry(t, "files/activity/a.fit", []byte("a"))},
	})
	req := newJSONAuthRequest(t, userID, http.MethodPost, "/api/v1/auth/data-import/multipart", body)
	w := httptest.NewRecorder()
	h.DataImportMultipartCreate(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("valid manifest status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestMultipartImportOldManifestImpliedMetadataZip(t *testing.T) {
	h, userID, _ := newMultipartImportHandlerForTest(t)
	body := multipartImportManifestJSON(t, []multipartExpectedPart{
		{name: "files-0001.zip", role: "files", data: zipWithEntry(t, "files/activity/a.fit", []byte("a"))},
	})

	w := httptest.NewRecorder()
	h.DataImportMultipartCreate(w, newJSONAuthRequest(t, userID, http.MethodPost, "/api/v1/auth/data-import/multipart", body))
	if w.Code != http.StatusAccepted {
		t.Fatalf("legacy manifest status = %d body=%s", w.Code, w.Body.String())
	}
	jobID := decodeJobIDFromResponse(t, w.Body.Bytes())
	var metadata []byte
	if err := h.DB.Pool.QueryRow(context.Background(), `SELECT metadata FROM background_jobs WHERE id = $1`, jobID).Scan(&metadata); err != nil {
		t.Fatalf("select metadata: %v", err)
	}
	if !bytes.Contains(metadata, []byte(`"metadata.zip"`)) || !bytes.Contains(metadata, []byte(`"legacy_checksum_missing"`)) {
		t.Fatalf("metadata.zip legacy marker missing: %s", metadata)
	}
}

func TestMultipartImportRejectsTooManyParts(t *testing.T) {
	h, userID, _ := newMultipartImportHandlerForTest(t)
	h.Config.GDPRMultipartMaxParts = 1
	body := multipartImportManifestJSON(t, []multipartExpectedPart{
		{name: "metadata.zip", role: "metadata", data: minimalImportZip(t)},
		{name: "files-0001.zip", role: "files", data: zipWithEntry(t, "files/activity/a.fit", []byte("a"))},
	})

	w := httptest.NewRecorder()
	h.DataImportMultipartCreate(w, newJSONAuthRequest(t, userID, http.MethodPost, "/api/v1/auth/data-import/multipart", body))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("too many parts status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestMultipartImportPartUploadVerifiesChecksum(t *testing.T) {
	h, userID, _ := newMultipartImportHandlerForTest(t)
	jobID := createMultipartImportSessionForTest(t, h, userID, []multipartExpectedPart{
		{name: "metadata.zip", role: "metadata", data: minimalImportZip(t)},
	})

	w := httptest.NewRecorder()
	req := newPartUploadRequest(t, userID, jobID, "metadata.zip", []byte("wrong"))
	h.DataImportMultipartPart(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("checksum mismatch status = %d body=%s", w.Code, w.Body.String())
	}
	var metadata []byte
	if err := h.DB.Pool.QueryRow(context.Background(), `SELECT metadata FROM background_jobs WHERE id = $1`, jobID).Scan(&metadata); err != nil {
		t.Fatalf("select metadata: %v", err)
	}
	if bytes.Contains(metadata, []byte(`"received":true`)) {
		t.Fatalf("part marked received after checksum mismatch: %s", metadata)
	}
}

func TestMultipartImportStartRequiresAllParts(t *testing.T) {
	h, userID, _ := newMultipartImportHandlerForTest(t)
	jobID := createMultipartImportSessionForTest(t, h, userID, []multipartExpectedPart{
		{name: "metadata.zip", role: "metadata", data: minimalImportZip(t)},
		{name: "files-0001.zip", role: "files", data: zipWithEntry(t, "files/activity/a.fit", []byte("a"))},
	})

	w := httptest.NewRecorder()
	h.DataImportMultipartStart(w, newJobActionRequest(t, userID, http.MethodPost, "/api/v1/auth/data-import/"+jobID+"/start", jobID))
	if w.Code != http.StatusConflict {
		t.Fatalf("start with missing parts status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestMultipartImportStartUsesArtifactMultiPartSource(t *testing.T) {
	h, userID, store := newMultipartImportHandlerForTest(t)
	metaZip := minimalImportZip(t)
	filesZip := zipWithEntry(t, "files/activity/a.fit", []byte("a"))
	jobID := createMultipartImportSessionForTest(t, h, userID, []multipartExpectedPart{
		{name: "metadata.zip", role: "metadata", data: metaZip},
		{name: "files-0001.zip", role: "files", data: filesZip},
	})
	uploadMultipartPartForTest(t, h, userID, jobID, "metadata.zip", metaZip, http.StatusAccepted)
	uploadMultipartPartForTest(t, h, userID, jobID, "files-0001.zip", filesZip, http.StatusAccepted)

	w := httptest.NewRecorder()
	h.DataImportMultipartStart(w, newJobActionRequest(t, userID, http.MethodPost, "/api/v1/auth/data-import/"+jobID+"/start", jobID))
	if w.Code != http.StatusAccepted {
		t.Fatalf("start status = %d body=%s", w.Code, w.Body.String())
	}
	if !store.openReaderAtCalled {
		t.Fatal("multipart import start did not use artifact-backed source")
	}
}

func TestUploadingSessionSurvivesThirtyMinuteStaleRule(t *testing.T) {
	h, userID, _ := newMultipartImportHandlerForTest(t)
	jobID := createMultipartImportSessionForTest(t, h, userID, []multipartExpectedPart{{name: "metadata.zip", role: "metadata", data: minimalImportZip(t)}})
	if _, err := h.DB.Pool.Exec(context.Background(), `UPDATE background_jobs SET updated_at = NOW() - INTERVAL '31 minutes' WHERE id = $1`, jobID); err != nil {
		t.Fatalf("age job: %v", err)
	}

	h.cleanupStaleImportJobs(context.Background())

	var status string
	if err := h.DB.Pool.QueryRow(context.Background(), `SELECT status FROM background_jobs WHERE id = $1`, jobID).Scan(&status); err != nil {
		t.Fatalf("select status: %v", err)
	}
	if status != "uploading" {
		t.Fatalf("status = %q, want uploading", status)
	}
}

func TestExpiredMultipartUploadSessionDeletesArtifacts(t *testing.T) {
	h, userID, store := newMultipartImportHandlerForTest(t)
	data := minimalImportZip(t)
	jobID := createMultipartImportSessionForTest(t, h, userID, []multipartExpectedPart{{name: "metadata.zip", role: "metadata", data: data}})
	uploadMultipartPartForTest(t, h, userID, jobID, "metadata.zip", data, http.StatusAccepted)
	if _, err := h.DB.Pool.Exec(context.Background(), `
		UPDATE background_jobs
		SET metadata = jsonb_set(metadata, '{import_upload,expires_at}', to_jsonb((NOW() - INTERVAL '1 minute')::text), true)
		WHERE id = $1`, jobID); err != nil {
		t.Fatalf("expire job: %v", err)
	}

	h.cleanupStaleImportJobs(context.Background())

	if err := assertImportArtifactPrefixMissing(t, store.LocalStore, userID, jobID); err != nil {
		t.Fatal(err)
	}
}

func TestUploadingSessionBlocksOtherGDPRTransfers(t *testing.T) {
	h, userID, _ := newMultipartImportHandlerForTest(t)
	createMultipartImportSessionForTest(t, h, userID, []multipartExpectedPart{{name: "metadata.zip", role: "metadata", data: minimalImportZip(t)}})

	w := httptest.NewRecorder()
	h.DataImport(w, newDataImportUploadRequest(t, userID, minimalImportZip(t)))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("single import while multipart uploading status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestMultipartImportCancelDeletesArtifactsAndReleasesReservation(t *testing.T) {
	h, userID, store := newMultipartImportHandlerForTest(t)
	data := minimalImportZip(t)
	jobID := createMultipartImportSessionForTest(t, h, userID, []multipartExpectedPart{{name: "metadata.zip", role: "metadata", data: data}})
	uploadMultipartPartForTest(t, h, userID, jobID, "metadata.zip", data, http.StatusAccepted)

	w := httptest.NewRecorder()
	h.DataImportMultipartCancel(w, newJobActionRequest(t, userID, http.MethodPost, "/api/v1/auth/data-import/"+jobID+"/cancel", jobID))
	if w.Code != http.StatusOK {
		t.Fatalf("cancel status = %d body=%s", w.Code, w.Body.String())
	}
	if err := assertImportArtifactPrefixMissing(t, store.LocalStore, userID, jobID); err != nil {
		t.Fatal(err)
	}
}

func TestMultipartImportAuditEvents(t *testing.T) {
	h, userID, _ := newMultipartImportHandlerForTest(t)
	data := minimalImportZip(t)
	jobID := createMultipartImportSessionForTest(t, h, userID, []multipartExpectedPart{{name: "metadata.zip", role: "metadata", data: data}})
	uploadMultipartPartForTest(t, h, userID, jobID, "metadata.zip", data, http.StatusAccepted)

	var count int
	if err := h.DB.Pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM auth_audit_log
		WHERE user_id = $1::uuid
		  AND event_type IN ('data_import_multipart_session_created', 'data_import_multipart_part_received')`,
		userID).Scan(&count); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if count != 2 {
		t.Fatalf("audit event count = %d, want 2", count)
	}
}

func newDataImportUploadRequest(t *testing.T, userID string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "skadia-export.zip")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write multipart: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/data-import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
}

type multipartExpectedPart struct {
	name string
	role string
	data []byte
}

func newMultipartImportHandlerForTest(t *testing.T) (*Handler, string, *recordingArtifactStore) {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	clearActiveDataImportJobsForTest(t, pool)
	userID := createArtifactExportUser(t, pool)
	local, err := gdprartifacts.NewLocalStore(t.TempDir()+"/exports", t.TempDir()+"/imports")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	store := &recordingArtifactStore{LocalStore: local}
	h := &Handler{
		DB:            &database.DB{Pool: pool},
		Config:        &config.Config{GDPRMultipartMaxParts: 256},
		ArtifactStore: store,
		shutdownCtx:   ctx,
	}
	return h, userID, store
}

func multipartImportManifestJSON(t *testing.T, parts []multipartExpectedPart) string {
	t.Helper()
	rawParts := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		sum := sha256.Sum256(part.data)
		rawParts = append(rawParts, map[string]any{
			"name":       part.name,
			"role":       part.role,
			"sha256":     fmt.Sprintf("%x", sum[:]),
			"size_bytes": len(part.data),
		})
	}
	raw, err := json.Marshal(map[string]any{
		"format_version":   1,
		"schema_migration": 1,
		"user_id":          uuid.NewString(),
		"multipart":        true,
		"parts":            rawParts,
	})
	if err != nil {
		t.Fatalf("marshal multipart manifest: %v", err)
	}
	return string(raw)
}

func newJSONAuthRequest(t *testing.T, userID, method, target, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
}

func newPartUploadRequest(t *testing.T, userID, jobID, partName string, content []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/data-import/"+jobID+"/parts/"+partName, bytes.NewReader(content))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	return withRouteParam(req, map[string]string{"jobID": jobID, "partName": partName})
}

func newJobActionRequest(t *testing.T, userID, method, target, jobID string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	return withRouteParam(req, map[string]string{"jobID": jobID})
}

func withRouteParam(r *http.Request, params map[string]string) *http.Request {
	routeCtx := chi.NewRouteContext()
	for key, value := range params {
		routeCtx.URLParams.Add(key, value)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeCtx))
}

func createMultipartImportSessionForTest(t *testing.T, h *Handler, userID string, parts []multipartExpectedPart) string {
	t.Helper()
	w := httptest.NewRecorder()
	h.DataImportMultipartCreate(w, newJSONAuthRequest(t, userID, http.MethodPost, "/api/v1/auth/data-import/multipart", multipartImportManifestJSON(t, parts)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("create multipart session status = %d body=%s", w.Code, w.Body.String())
	}
	return decodeJobIDFromResponse(t, w.Body.Bytes())
}

func uploadMultipartPartForTest(t *testing.T, h *Handler, userID, jobID, partName string, content []byte, wantStatus int) {
	t.Helper()
	w := httptest.NewRecorder()
	h.DataImportMultipartPart(w, newPartUploadRequest(t, userID, jobID, partName, content))
	if w.Code != wantStatus {
		t.Fatalf("upload %s status = %d, want %d body=%s", partName, w.Code, wantStatus, w.Body.String())
	}
}

func decodeJobIDFromResponse(t *testing.T, raw []byte) string {
	t.Helper()
	var resp struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.JobID == "" {
		t.Fatalf("response missing job_id: %s", raw)
	}
	return resp.JobID
}

func zipWithEntry(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func assertImportArtifactPrefixMissing(t *testing.T, store gdprartifacts.Store, userID, jobID string) error {
	t.Helper()
	_, err := store.Open(context.Background(), gdprartifacts.ObjectRef{
		Backend: "local",
		Kind:    "import",
		UserID:  userID,
		JobID:   jobID,
		Role:    gdprartifacts.RoleSingleZIP,
		Name:    "metadata.zip",
		Path:    "imports/" + userID + "/" + jobID + "/metadata.zip",
	}, nil)
	if err == nil {
		return fmt.Errorf("import artifact prefix still readable for job %s", jobID)
	}
	return nil
}

func clearActiveDataImportJobsForTest(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `DELETE FROM background_jobs WHERE job_type = 'data_import' AND status IN ('queued', 'running')`); err != nil {
		t.Fatalf("clear active data_import jobs: %v", err)
	}
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

type recordingArtifactStore struct {
	*gdprartifacts.LocalStore
	openReaderAtCalled bool
}

func (s *recordingArtifactStore) OpenReaderAt(ctx context.Context, ref gdprartifacts.ObjectRef) (gdprartifacts.SizedReaderAt, error) {
	s.openReaderAtCalled = true
	ref.Backend = "local"
	return s.LocalStore.OpenReaderAt(ctx, ref)
}

func stageSingleImportArtifactForTest(t *testing.T, ctx context.Context, userID string, content []byte, recordOpenReaderAt bool) (*recordingArtifactStore, gdprartifacts.Artifact, string) {
	t.Helper()
	local, err := gdprartifacts.NewLocalStore(t.TempDir()+"/exports", t.TempDir()+"/imports")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	store := &recordingArtifactStore{LocalStore: local}
	jobID := uuid.NewString()
	expiresAt := time.Now().UTC().Add(24 * time.Hour)
	writer, err := store.CreateWriter(ctx, gdprartifacts.ObjectSpec{
		Kind:        "import",
		UserID:      userID,
		JobID:       jobID,
		Role:        gdprartifacts.RoleSingleZIP,
		Name:        "import.zip",
		ContentType: "application/zip",
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateWriter: %v", err)
	}
	if _, err := writer.Write(content); err != nil {
		t.Fatalf("artifact write: %v", err)
	}
	ref, err := writer.Commit(ctx)
	if err != nil {
		t.Fatalf("artifact commit: %v", err)
	}
	if recordOpenReaderAt {
		ref.Backend = "s3"
		ref.Bucket = "skadia-imports"
		ref.Key = "gdpr/imports/" + userID + "/" + jobID + "/import.zip"
	}
	artifact := gdprartifacts.Artifact{
		Version:   1,
		Format:    gdprartifacts.ArtifactFormatSingleZIP,
		ExpiresAt: expiresAt,
		Objects:   []gdprartifacts.ObjectRef{ref},
	}
	metadata, err := gdprartifacts.MergeArtifactJSON(nil, artifact)
	if err != nil {
		t.Fatalf("MergeArtifactJSON: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(metadata, &obj); err != nil {
		t.Fatalf("metadata unmarshal: %v", err)
	}
	obj["import_upload"] = map[string]any{"state": "committed", "size_bytes": ref.SizeBytes, "sha256": ref.SHA256}
	metadata, _ = json.Marshal(obj)
	if _, err := testdb.Open(t).Exec(ctx,
		`INSERT INTO background_jobs (id, job_type, user_id, status, started_at, updated_at, metadata)
		 VALUES ($1, 'data_import', $2, 'queued', NOW(), NOW(), $3::jsonb)`,
		jobID, userID, metadata); err != nil {
		t.Fatalf("insert background job: %v", err)
	}
	zipPath := writeTempZipForLegacySignature(t, content)
	return store, artifact, zipPath
}

func minimalImportZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatalf("create manifest: %v", err)
	}
	_, _ = io.WriteString(w, `{"format_version":1,"schema_migration":1,"user_id":"`+uuid.NewString()+`"}`)
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func writeTempZipForLegacySignature(t *testing.T, content []byte) string {
	t.Helper()
	path := t.TempDir() + "/import.zip"
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write legacy zip: %v", err)
	}
	return path
}
