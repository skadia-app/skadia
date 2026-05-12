package auth

import (
	"context"
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
	// Full artifact state-machine reconciliation is implemented in the later
	// cleanup-hardening phase. This startup pass intentionally keeps legacy
	// file_path cleanup active while artifact metadata is introduced.
	cleanupStaleExportJobs(ctx, h.DB.Pool)
	h.cleanupStaleImportJobs(ctx)
}

func (h *Handler) reconcileMissingArtifactsForStatus(ctx context.Context, jobID, userID string) {
	if h == nil || h.ArtifactStore == nil {
		return
	}
	if err := ctx.Err(); err != nil {
		slog.Warn("gdpr artifact status reconciliation cancelled", "job_id", jobID, "user_id", userID, "error", err)
	}
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
