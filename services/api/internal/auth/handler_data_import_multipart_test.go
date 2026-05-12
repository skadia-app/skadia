package auth

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
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
