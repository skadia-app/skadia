package handler

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/phash"
)

// phashBucketName returns the S3 bucket used for activity photos.
func phashBucketName() string {
	b := os.Getenv("GARAGE_MEDIA_BUCKET")
	if b == "" {
		return "skadia-media"
	}
	return b
}

// StartPhashBackfill computes pHash for all photos of the authenticated user
// that currently have no pHash stored.
//
// POST /api/v1/admin/phash-backfill
func (h *Handler) StartPhashBackfill(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	// Check whether a phash_backfill job is already running for this user.
	var alreadyRunning bool
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM background_jobs WHERE user_id = $1 AND job_type = 'phash_backfill' AND status IN ('pending', 'running'))`,
		userID).Scan(&alreadyRunning)
	if err != nil {
		slog.Error("phash backfill: check active job failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to check active jobs", r.URL.Path)
		return
	}
	if alreadyRunning {
		writeJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
		return
	}

	// Load photos without pHash.
	photos, err := h.DB.PhotosWithoutPHash(r.Context(), userID)
	if err != nil {
		slog.Error("phash backfill: failed to list photos", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list photos", r.URL.Path)
		return
	}

	if len(photos) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "processed": 0})
		return
	}

	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "phash_backfill", nil, nil, int64(len(photos)))
	if err != nil {
		slog.Error("phash backfill: failed to create background job", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create job", r.URL.Path)
		return
	}

	bucket := phashBucketName()

	go func() {
		ctx := context.Background()
		processed := int64(0)
		anyFailed := false

		for _, photo := range photos {
			// Download thumbnail (smaller, faster to hash). Fall back to original if thumb key is empty.
			key := photo.S3KeyThumb
			if key == "" {
				slog.Warn("phash backfill: photo has no thumb key, skipping", "photo_id", photo.ID)
				processed++
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
				continue
			}

			data, _, err := h.Storage.DownloadFromBucket(ctx, bucket, key)
			if err != nil {
				slog.Warn("phash backfill: s3 download failed", "photo_id", photo.ID, "key", key, "error", err)
				anyFailed = true
				processed++
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
				continue
			}

			var hashVal uint64
			var hashErr error

			if photo.IsVideo {
				durationS := 0.0
				if photo.DurationS != nil {
					durationS = *photo.DurationS
				}
				hashVal, hashErr = phash.ComputeVideoHash(data, durationS)
			} else {
				hashVal, hashErr = phash.ComputeFromBytes(data)
				if hashErr != nil {
					// Fallback: try via ffmpeg (handles HEIC, JXL, etc.)
					hashVal, hashErr = phash.ComputeViaFFmpeg(data)
				}
			}

			if hashErr != nil {
				slog.Warn("phash backfill: compute failed", "photo_id", photo.ID, "is_video", photo.IsVideo, "error", hashErr)
				anyFailed = true
				processed++
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
				continue
			}

			if err := h.DB.UpdateActivityPhotoPHash(ctx, photo.ID, int64(hashVal)); err != nil {
				slog.Warn("phash backfill: db update failed", "photo_id", photo.ID, "error", err)
				anyFailed = true
			}

			processed++
			_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
		}

		_ = h.DB.CompleteBackgroundJob(ctx, jobID, anyFailed)
		slog.Info("phash backfill: completed", "user_id", userID, "job_id", jobID, "processed", processed, "any_failed", anyFailed)
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_id": jobID,
		"queued": len(photos),
	})
}
