package auth

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/config"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
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
