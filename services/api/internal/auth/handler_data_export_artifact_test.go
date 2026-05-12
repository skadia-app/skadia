package auth

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/config"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestExecuteExportJobOnceWritesSingleZipToArtifactStore(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createArtifactExportUser(t, pool)
	jobID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO background_jobs (id, job_type, user_id, status, started_at, updated_at)
		 VALUES ($1, 'gdpr_export', $2, 'running', NOW(), NOW())`,
		jobID, userID); err != nil {
		t.Fatalf("insert background job: %v", err)
	}
	store, err := gdprartifacts.NewLocalStore(t.TempDir()+"/exports", t.TempDir()+"/imports")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	h := &Handler{
		DB:            &database.DB{Pool: pool},
		Config:        &config.Config{},
		ArtifactStore: store,
		shutdownCtx:   ctx,
	}
	if err := h.executeExportJobOnce(ctx, jobID, userID); err != nil {
		t.Fatalf("executeExportJobOnce: %v", err)
	}
	var metadata []byte
	if err := pool.QueryRow(ctx, `SELECT metadata FROM background_jobs WHERE id=$1`, jobID).Scan(&metadata); err != nil {
		t.Fatalf("metadata query: %v", err)
	}
	if json.Valid(metadata) == false {
		t.Fatalf("invalid metadata: %s", metadata)
	}
	if containsJSONKey(metadata, "file_path") {
		t.Fatalf("new artifact export wrote legacy file_path metadata: %s", metadata)
	}
	meta, err := gdprartifacts.DecodeJobMetadata(metadata)
	if err != nil {
		t.Fatalf("DecodeJobMetadata: %v", err)
	}
	if meta.Artifact == nil || meta.Artifact.Format != gdprartifacts.ArtifactFormatSingleZIP {
		t.Fatalf("artifact metadata = %#v", meta.Artifact)
	}
	if len(meta.Artifact.Objects) != 1 || meta.Artifact.Objects[0].Path == "" {
		t.Fatalf("artifact object missing local path: %#v", meta.Artifact.Objects)
	}
	stat, err := store.Stat(ctx, meta.Artifact.Objects[0])
	if err != nil {
		t.Fatalf("artifact Stat: %v", err)
	}
	if stat.State != gdprartifacts.ObjectPresent {
		t.Fatalf("artifact state = %s", stat.State)
	}
}

func TestDataExportStatusUsesArtifactSessionURL(t *testing.T) {
	expiresAt := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	metadata, err := exportStatusArtifactMetadata(expiresAt)
	if err != nil {
		t.Fatalf("metadata: %v", err)
	}
	resp := buildDataExportStatusResponse("job-1", "completed", time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC), nil, metadata, time.Date(2026, 5, 12, 12, 1, 0, 0, time.UTC))
	if resp["artifact_format"] != "single_zip" {
		t.Fatalf("artifact_format = %v", resp["artifact_format"])
	}
	if resp["download_session_url"] != "/api/v1/auth/data-export/job-1/download-session" {
		t.Fatalf("download_session_url = %v", resp["download_session_url"])
	}
	if _, ok := resp["download_url"]; ok {
		t.Fatalf("artifact status should not expose legacy download_url: %v", resp["download_url"])
	}
}

func TestMultipartExportWriterListsMetadataZipInParts(t *testing.T) {
	ctx := context.Background()
	store, artifact := executeMultipartExportForTest(t, ctx, []testExportFile{
		{zipPath: "activity/one.fit", fileName: "one.fit", fileType: "activity", data: []byte("one")},
	}, config.Config{
		GDPRExportMultipartThresholdBytes: 1,
		GDPRExportPartTargetBytes:         1024,
		GDPRMultipartMaxParts:             8,
	})

	manifest := readMultipartManifestArtifact(t, ctx, store, artifact)
	if !manifest.Multipart {
		t.Fatalf("manifest multipart = false")
	}
	if len(manifest.Parts) < 2 {
		t.Fatalf("manifest parts = %#v, want metadata.zip plus file parts", manifest.Parts)
	}
	part := manifestPartByName(manifest.Parts, "metadata.zip")
	if part == nil {
		t.Fatalf("manifest did not list metadata.zip: %#v", manifest.Parts)
	}
	if part.Role != "metadata" {
		t.Fatalf("metadata.zip role = %q, want metadata", part.Role)
	}
	if part.SHA256 == "" || part.SizeBytes <= 0 {
		t.Fatalf("metadata.zip checksum/size missing: %#v", part)
	}
}

func TestMultipartExportWriterSplitsFilesAtForcedThreshold(t *testing.T) {
	ctx := context.Background()
	store, artifact := executeMultipartExportForTest(t, ctx, []testExportFile{
		{zipPath: "activity/one.fit", fileName: "one.fit", fileType: "activity", data: []byte(strings.Repeat("a", 128))},
		{zipPath: "activity/two.fit", fileName: "two.fit", fileType: "activity", data: []byte(strings.Repeat("b", 128))},
	}, config.Config{
		GDPRExportMultipartThresholdBytes: 1,
		GDPRExportPartTargetBytes:         1,
		GDPRMultipartMaxParts:             8,
	})

	manifest := readMultipartManifestArtifact(t, ctx, store, artifact)
	if manifestPartByName(manifest.Parts, "files-0001.zip") == nil {
		t.Fatalf("manifest missing files-0001.zip: %#v", manifest.Parts)
	}
	if manifestPartByName(manifest.Parts, "files-0002.zip") == nil {
		t.Fatalf("manifest missing files-0002.zip: %#v", manifest.Parts)
	}
}

func TestMultipartExportWriterAlignsPartCountWithLimit(t *testing.T) {
	ctx := context.Background()
	store, artifact := executeMultipartExportForTest(t, ctx, []testExportFile{
		{zipPath: "activity/one.fit", fileName: "one.fit", fileType: "activity", data: []byte("one")},
		{zipPath: "activity/two.fit", fileName: "two.fit", fileType: "activity", data: []byte("two")},
		{zipPath: "activity/three.fit", fileName: "three.fit", fileType: "activity", data: []byte("three")},
	}, config.Config{
		GDPRExportMultipartThresholdBytes: 1,
		GDPRExportPartTargetBytes:         1,
		GDPRMultipartMaxParts:             3,
	})

	manifest := readMultipartManifestArtifact(t, ctx, store, artifact)
	if len(manifest.Parts) > 3 {
		t.Fatalf("manifest emitted %d parts, want <= 3: %#v", len(manifest.Parts), manifest.Parts)
	}
	if manifestPartByName(manifest.Parts, "files-0001.zip") == nil {
		t.Fatalf("manifest missing file part after target-size alignment: %#v", manifest.Parts)
	}
}

func TestMultipartExportKeepsFailedFilesInManifest(t *testing.T) {
	ctx := context.Background()
	store, artifact := executeMultipartExportForTest(t, ctx, []testExportFile{
		{zipPath: "activity/one.fit", fileName: "one.fit", fileType: "activity", data: []byte("one")},
		{zipPath: "activity/missing.fit", fileName: "missing.fit", fileType: "activity", failed: true},
	}, config.Config{
		GDPRExportMultipartThresholdBytes: 1,
		GDPRExportPartTargetBytes:         1024,
		GDPRMultipartMaxParts:             8,
	})

	manifest := readMultipartManifestArtifact(t, ctx, store, artifact)
	if len(manifest.FailedFiles) != 1 || manifest.FailedFiles[0] != "activity/missing.fit" {
		t.Fatalf("failed_files = %#v, want activity/missing.fit", manifest.FailedFiles)
	}
}

func createArtifactExportUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, username, email, display_name, timezone, email_verified, registration_source)
		VALUES ($1, $2, $3, 'Artifact Export Test', 'UTC', true, 'test')`,
		id, "artifact_"+id[:8], "artifact_"+id[:8]+"@example.com")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func exportStatusArtifactMetadata(expiresAt time.Time) ([]byte, error) {
	raw, err := gdprartifacts.EncodeArtifact(gdprartifacts.Artifact{
		Version:   1,
		Format:    gdprartifacts.ArtifactFormatSingleZIP,
		ExpiresAt: expiresAt,
		Objects: []gdprartifacts.ObjectRef{{
			Backend:     "local",
			Kind:        "export",
			UserID:      "11111111-1111-1111-1111-111111111111",
			JobID:       "22222222-2222-2222-2222-222222222222",
			Role:        gdprartifacts.RoleSingleZIP,
			Name:        "export.zip",
			Path:        "exports/11111111-1111-1111-1111-111111111111/22222222-2222-2222-2222-222222222222/export.zip",
			SHA256:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SizeBytes:   42,
			ContentType: "application/zip",
			ExpiresAt:   expiresAt,
		}},
	})
	if err != nil {
		return nil, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]json.RawMessage{"artifact": obj["artifact"]})
}

func containsJSONKey(raw []byte, key string) bool {
	var obj map[string]json.RawMessage
	_ = json.Unmarshal(raw, &obj)
	_, ok := obj[key]
	return ok
}

type testExportFile struct {
	zipPath  string
	fileName string
	fileType string
	data     []byte
	failed   bool
}

type testExportShadowFS struct {
	files []testExportFile
}

func (s *testExportShadowFS) StreamExportFiles(ctx context.Context, userID string, sink shadowfs.ExportFileSink) ([]string, error) {
	var failed []string
	for _, f := range s.files {
		if f.failed {
			failed = append(failed, f.zipPath)
			continue
		}
		if err := sink.AddExportFile(ctx, shadowfs.ExportFile{
			ZIPPath:   f.zipPath,
			FileName:  f.fileName,
			FileType:  f.fileType,
			SizeBytes: int64(len(f.data)),
			Modified:  time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC),
		}, bytes.NewReader(f.data)); err != nil {
			return failed, err
		}
	}
	return failed, nil
}

func (s *testExportShadowFS) StreamToZip(ctx context.Context, userID string, zw *zip.Writer) ([]string, error) {
	return s.StreamExportFiles(ctx, userID, shadowfs.NewZipExportFileSink(zw))
}

func (s *testExportShadowFS) DiskUsage(context.Context, string) (int64, error) {
	var total int64
	for _, f := range s.files {
		total += int64(len(f.data))
	}
	return total, nil
}

func (s *testExportShadowFS) Put(context.Context, shadowfs.FileEntry, io.Reader) (shadowfs.FileRecord, error) {
	return shadowfs.FileRecord{}, nil
}
func (s *testExportShadowFS) Get(context.Context, string, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (s *testExportShadowFS) Delete(context.Context, string, string) error   { return nil }
func (s *testExportShadowFS) DeleteAllForUser(context.Context, string) error { return nil }
func (s *testExportShadowFS) List(context.Context, string, *string) ([]shadowfs.FileRecord, error) {
	return nil, nil
}
func (s *testExportShadowFS) Resolve(context.Context, string, string) (shadowfs.ResolvedFile, error) {
	return shadowfs.ResolvedFile{}, nil
}
func (s *testExportShadowFS) Archive(context.Context, string, string, string) error { return nil }
func (s *testExportShadowFS) PutCompanion(context.Context, string, string, string, []byte) error {
	return nil
}
func (s *testExportShadowFS) GetCompanion(context.Context, string, string, string) ([]byte, error) {
	return nil, nil
}
func (s *testExportShadowFS) DeleteCompanions(context.Context, string, string) error {
	return nil
}
func (s *testExportShadowFS) PutCompanionAt(context.Context, string, []byte) error { return nil }
func (s *testExportShadowFS) GetCompanionAt(context.Context, string) ([]byte, error) {
	return nil, shadowfs.ErrNotFound
}
func (s *testExportShadowFS) DeleteCompanionAt(context.Context, string) error { return nil }

type multipartManifestForTest struct {
	Multipart   bool                   `json:"multipart"`
	Parts       []multipartPartForTest `json:"parts"`
	FailedFiles []string               `json:"failed_files"`
}

type multipartPartForTest struct {
	Name               string `json:"name"`
	Role               string `json:"role"`
	SHA256             string `json:"sha256"`
	SizeBytes          int64  `json:"size_bytes"`
	WebImportSupported *bool  `json:"web_import_supported,omitempty"`
}

func executeMultipartExportForTest(t *testing.T, ctx context.Context, files []testExportFile, cfg config.Config) (*gdprartifacts.LocalStore, gdprartifacts.Artifact) {
	t.Helper()
	pool := testdb.Open(t)
	userID := createArtifactExportUser(t, pool)
	jobID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO background_jobs (id, job_type, user_id, status, started_at, updated_at)
		 VALUES ($1, 'gdpr_export', $2, 'running', NOW(), NOW())`,
		jobID, userID); err != nil {
		t.Fatalf("insert background job: %v", err)
	}
	store, err := gdprartifacts.NewLocalStore(t.TempDir()+"/exports", t.TempDir()+"/imports")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	h := &Handler{
		DB:            &database.DB{Pool: pool},
		Config:        &cfg,
		ArtifactStore: store,
		ShadowFS:      &testExportShadowFS{files: files},
		shutdownCtx:   ctx,
	}
	if err := h.executeExportJobOnce(ctx, jobID, userID); err != nil {
		t.Fatalf("executeExportJobOnce: %v", err)
	}
	var metadata []byte
	if err := pool.QueryRow(ctx, `SELECT metadata FROM background_jobs WHERE id=$1`, jobID).Scan(&metadata); err != nil {
		t.Fatalf("metadata query: %v", err)
	}
	meta, err := gdprartifacts.DecodeJobMetadata(metadata)
	if err != nil {
		t.Fatalf("DecodeJobMetadata: %v", err)
	}
	if meta.Artifact == nil {
		t.Fatalf("artifact metadata missing: %s", metadata)
	}
	if meta.Artifact.Format != gdprartifacts.ArtifactFormatMultipart {
		t.Fatalf("artifact format = %q, want multipart; metadata=%s", meta.Artifact.Format, metadata)
	}
	return store, *meta.Artifact
}

func readMultipartManifestArtifact(t *testing.T, ctx context.Context, store gdprartifacts.Store, artifact gdprartifacts.Artifact) multipartManifestForTest {
	t.Helper()
	ref, err := artifactRefByName(artifact, "manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	reader, err := store.Open(ctx, ref, nil)
	if err != nil {
		t.Fatalf("open manifest artifact: %v", err)
	}
	defer reader.Close()
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read manifest artifact: %v", err)
	}
	var manifest multipartManifestForTest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode manifest artifact %q: %v", raw, err)
	}
	return manifest
}

func manifestPartByName(parts []multipartPartForTest, name string) *multipartPartForTest {
	for i := range parts {
		if parts[i].Name == name {
			return &parts[i]
		}
	}
	return nil
}

func artifactRefByName(artifact gdprartifacts.Artifact, name string) (gdprartifacts.ObjectRef, error) {
	for _, ref := range artifact.Objects {
		if ref.Name == name {
			return ref, nil
		}
	}
	return gdprartifacts.ObjectRef{}, fmt.Errorf("artifact object %q not found", name)
}
