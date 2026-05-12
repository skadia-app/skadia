package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
)

type artifactMaintenanceHooks struct {
	startupSweep func(context.Context)
}

func (h *Handler) StartGDPRArtifactMaintenance(ctx context.Context) {
	if h.artifactMaintenanceHooks.startupSweep != nil {
		h.artifactMaintenanceHooks.startupSweep(ctx)
	} else {
		h.reconcileExpiredArtifacts(ctx)
	}
	h.cleanupIncompleteS3MultipartUploads(ctx)

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.reconcileExpiredArtifacts(ctx)
				h.cleanupIncompleteS3MultipartUploads(ctx)
			}
		}
	}()
}

func (h *Handler) reconcileExpiredArtifacts(ctx context.Context) {
	if h == nil || h.DB == nil || h.ArtifactStore == nil {
		return
	}
	if err := ctx.Err(); err != nil {
		return
	}
	h.cleanupExpiredExportArtifacts(ctx)
	cleanupStaleExportJobs(ctx, h.DB.Pool)
	h.cleanupStaleImportJobs(ctx)
}

func (h *Handler) cleanupExpiredExportArtifacts(ctx context.Context) {
	rows, err := h.DB.Pool.Query(ctx, `
		SELECT id, user_id, metadata
		FROM background_jobs
		WHERE job_type = 'gdpr_export'
		  AND status IN ('completed', 'failed')
		  AND metadata ? 'artifact'`)
	if err != nil {
		slog.Error("gdpr artifact cleanup: query expired export artifacts failed", "error", err)
		return
	}
	type expiredArtifactJob struct {
		id       string
		userID   string
		metadata []byte
		artifact gdprartifacts.Artifact
	}
	var expired []expiredArtifactJob
	now := time.Now()
	for rows.Next() {
		var job expiredArtifactJob
		if err := rows.Scan(&job.id, &job.userID, &job.metadata); err != nil {
			continue
		}
		meta, err := gdprartifacts.DecodeJobMetadata(job.metadata)
		if err != nil || meta.Artifact == nil || now.Before(meta.Artifact.ExpiresAt) {
			continue
		}
		job.artifact = *meta.Artifact
		expired = append(expired, job)
	}
	rows.Close()

	for _, job := range expired {
		deletedAll := true
		for _, ref := range job.artifact.Objects {
			if err := h.ArtifactStore.Delete(ctx, ref); err != nil {
				deletedAll = false
				slog.Warn("gdpr artifact cleanup: delete expired export object failed", "job_id", job.id, "object", ref.Name, "error", err)
			}
		}
		if !deletedAll {
			continue
		}
		updated, err := stripExpiredExportArtifactJSON(job.metadata, job.artifact)
		if err != nil {
			slog.Warn("gdpr artifact cleanup: encode expired export metadata failed", "job_id", job.id, "error", err)
			continue
		}
		if _, err := h.DB.Pool.Exec(ctx, `UPDATE background_jobs SET metadata=$2::jsonb, updated_at=updated_at WHERE id=$1`, job.id, updated); err != nil {
			slog.Warn("gdpr artifact cleanup: update expired export metadata failed", "job_id", job.id, "error", err)
		}
	}
}

func stripExpiredExportArtifactJSON(metadata []byte, artifact gdprartifacts.Artifact) ([]byte, error) {
	obj := make(map[string]json.RawMessage)
	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &obj); err != nil {
			return nil, err
		}
	}
	delete(obj, "artifact")
	delete(obj, "download_sessions")
	if len(artifact.Objects) > 0 {
		summary := artifact.Objects[0]
		if raw, err := json.Marshal(summary.SHA256); err == nil {
			obj["sha256"] = raw
		}
		if raw, err := json.Marshal(summary.SizeBytes); err == nil {
			obj["size_bytes"] = raw
		}
	}
	if raw, err := json.Marshal(artifact.Format); err == nil {
		obj["artifact_format"] = raw
	}
	if raw, err := json.Marshal(time.Now().UTC().Format(time.RFC3339)); err == nil {
		obj["artifact_expired_at"] = raw
	}
	return json.Marshal(obj)
}

func (h *Handler) reconcileMissingArtifactsForStatus(ctx context.Context, jobID, userID string) {
	h.reconcileExportArtifactForStatus(ctx, jobID, userID, nil)
}

func (h *Handler) reconcileExportArtifactForStatus(ctx context.Context, jobID, userID string, metadata []byte) ([]byte, string) {
	if h == nil || h.DB == nil || h.ArtifactStore == nil || len(metadata) == 0 {
		return metadata, ""
	}
	if err := ctx.Err(); err != nil {
		slog.Warn("gdpr artifact status reconciliation cancelled", "job_id", jobID, "user_id", userID, "error", err)
		return metadata, "artifact_backend_unavailable"
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(metadata, &raw); err != nil || len(raw["artifact_missing_at"]) > 0 {
		return metadata, ""
	}
	meta, err := gdprartifacts.DecodeJobMetadata(metadata)
	if err != nil || meta.Artifact == nil {
		return metadata, ""
	}
	missing := make([]string, 0)
	for _, ref := range meta.Artifact.Objects {
		stat, err := h.ArtifactStore.Stat(ctx, ref)
		if err != nil || stat.State == gdprartifacts.ObjectUnknown {
			slog.Warn("gdpr artifact status reconciliation backend unavailable", "job_id", jobID, "user_id", userID, "object", ref.Name, "error", err)
			return metadata, "artifact_backend_unavailable"
		}
		if stat.State == gdprartifacts.ObjectMissing {
			missing = append(missing, ref.Name)
		}
	}
	if len(missing) == 0 {
		return metadata, ""
	}
	missingRaw, err := json.Marshal(missing)
	if err != nil {
		return metadata, "artifact_missing"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var updated []byte
	err = h.DB.Pool.QueryRow(ctx, `
		UPDATE background_jobs
		SET metadata = (jsonb_set(
				jsonb_set(COALESCE(metadata, '{}'::jsonb), '{artifact_missing_at}', to_jsonb($3::text), true),
				'{artifact_missing_objects}', $4::jsonb, true
			) - 'download_sessions'),
		    updated_at = updated_at
		WHERE id = $1 AND user_id = $2 AND job_type = 'gdpr_export'
		RETURNING metadata`,
		jobID, userID, now, string(missingRaw)).Scan(&updated)
	if err != nil {
		slog.Warn("gdpr artifact status reconciliation metadata update failed", "job_id", jobID, "user_id", userID, "error", err)
		return metadata, "artifact_missing"
	}
	return updated, "artifact_missing"
}

func (h *Handler) cleanupIncompleteS3MultipartUploads(ctx context.Context) {
	aborter, ok := h.ArtifactStore.(gdprartifacts.IncompleteMultipartAborter)
	if !ok {
		return
	}
	if err := aborter.AbortIncompleteUploads(ctx, gdprartifacts.ArtifactPrefix{Kind: "export"}); err != nil {
		slog.Warn("gdpr artifact cleanup: incomplete export multipart abort skipped", "error", err)
	}
	if err := aborter.AbortIncompleteUploads(ctx, gdprartifacts.ArtifactPrefix{Kind: "import"}); err != nil {
		slog.Warn("gdpr artifact cleanup: incomplete import multipart abort skipped", "error", err)
	}
}
