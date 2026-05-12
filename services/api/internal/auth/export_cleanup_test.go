package auth

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
	"github.com/google/uuid"
)

func TestStartArtifactCleanupRunsStartupSweepOnce(t *testing.T) {
	var sweeps int32
	h := &Handler{
		shutdownCtx: context.Background(),
		artifactMaintenanceHooks: artifactMaintenanceHooks{
			startupSweep: func(context.Context) { atomic.AddInt32(&sweeps, 1) },
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.StartGDPRArtifactMaintenance(ctx)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&sweeps) == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("startup sweep count = %d, want 1", sweeps)
}

func TestStartupSweepDeletesExpiredArtifactsAndKeepsHistorySummary(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	store, err := gdprartifacts.NewLocalStore(t.TempDir()+"/exports", t.TempDir()+"/imports")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	userID := createArtifactExportUser(t, pool)
	jobID := uuid.NewString()
	writer, err := store.CreateWriter(ctx, gdprartifacts.ObjectSpec{
		Kind:        "export",
		UserID:      userID,
		JobID:       jobID,
		Role:        gdprartifacts.RoleSingleZIP,
		Name:        "export.zip",
		ContentType: "application/zip",
		ExpiresAt:   time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateWriter: %v", err)
	}
	if _, err := writer.Write([]byte("expired export")); err != nil {
		t.Fatalf("write: %v", err)
	}
	ref, err := writer.Commit(ctx)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	metadata, err := gdprartifacts.MergeArtifactJSON(nil, gdprartifacts.Artifact{
		Version:   1,
		Format:    gdprartifacts.ArtifactFormatSingleZIP,
		ExpiresAt: time.Now().Add(-time.Hour),
		Objects:   []gdprartifacts.ObjectRef{ref},
	})
	if err != nil {
		t.Fatalf("MergeArtifactJSON: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO background_jobs (id, job_type, user_id, status, started_at, updated_at, metadata)
		 VALUES ($1, 'gdpr_export', $2, 'completed', NOW() - INTERVAL '2 hours', NOW() - INTERVAL '2 hours', $3::jsonb)`,
		jobID, userID, metadata); err != nil {
		t.Fatalf("insert job: %v", err)
	}

	h := &Handler{DB: &database.DB{Pool: pool}, ArtifactStore: store}
	h.reconcileExpiredArtifacts(ctx)

	stat, err := store.Stat(ctx, ref)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if stat.State != gdprartifacts.ObjectMissing {
		t.Fatalf("artifact state = %s, want missing", stat.State)
	}
	var updated []byte
	if err := pool.QueryRow(ctx, `SELECT metadata FROM background_jobs WHERE id=$1`, jobID).Scan(&updated); err != nil {
		t.Fatalf("select metadata: %v", err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(updated, &obj); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if len(obj["artifact"]) > 0 || len(obj["download_sessions"]) > 0 {
		t.Fatalf("expired metadata kept live artifact/session refs: %s", updated)
	}
	if len(obj["sha256"]) == 0 || len(obj["size_bytes"]) == 0 || len(obj["artifact_format"]) == 0 {
		t.Fatalf("expired metadata missing history summary: %s", updated)
	}
}
