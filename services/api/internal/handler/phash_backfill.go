package handler

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
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
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	// Check whether a phash_backfill job is already running for this user.
	var alreadyRunning bool
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM background_jobs WHERE user_id = $1 AND job_type = 'phash_backfill' AND status IN ('pending', 'running'))`,
		userID).Scan(&alreadyRunning)
	if err != nil {
		slog.Error("phash backfill: check active job failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to check active jobs", r.URL.Path)
		return
	}
	if alreadyRunning {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
		return
	}

	// Load photos without pHash.
	photos, err := h.DB.PhotosWithoutPHash(r.Context(), userID)
	if err != nil {
		slog.Error("phash backfill: failed to list photos", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list photos", r.URL.Path)
		return
	}

	if len(photos) == 0 {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "processed": 0})
		return
	}

	if h.Storage == nil && h.ShadowFS == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "processed": 0})
		return
	}

	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "phash_backfill", nil, nil, int64(len(photos)))
	if err != nil {
		slog.Error("phash backfill: failed to create background job", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create job", r.URL.Path)
		return
	}

	bucket := phashBucketName()

	go func() {
		ctx := context.Background()
		processed := int64(0)
		anyFailed := false

		// downloadPhoto reads photo data: ShadowFS (preferred) then S3 fallback.
		downloadPhoto := func(photo database.BackfillPhotoRow, key string) ([]byte, error) {
			// ShadowFS path (preferred).
			if h.ShadowFS != nil && photo.FileID != "" {
				rc, getErr := h.ShadowFS.Get(ctx, userID, photo.FileID)
				if getErr == nil {
					d, readErr := io.ReadAll(rc)
					rc.Close()
					if readErr == nil {
						return d, nil
					}
					slog.Warn("phash backfill: ShadowFS read failed, falling back to S3",
						"photo_id", photo.ID, "file_id", photo.FileID, "error", readErr)
				} else {
					slog.Warn("phash backfill: ShadowFS.Get failed, falling back to S3",
						"photo_id", photo.ID, "file_id", photo.FileID, "error", getErr)
				}
			}
			// Legacy S3 fallback.
			if h.Storage != nil && key != "" {
				data, _, dlErr := h.Storage.DownloadFromBucket(ctx, bucket, key)
				return data, dlErr
			}
			return nil, fmt.Errorf("no storage available for photo %s", photo.ID)
		}

		for _, photo := range photos {
			// Download thumbnail for pHash (smaller, faster to hash).
			key := photo.S3KeyThumb
			if key == "" {
				key = photo.S3Key // fallback to original
			}
			if key == "" && photo.FileID == "" {
				slog.Warn("phash backfill: photo has no S3 key or file_id, skipping", "photo_id", photo.ID)
				processed++
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
				continue
			}

			data, err := downloadPhoto(photo, key)
			if err != nil {
				slog.Warn("phash backfill: download failed", "photo_id", photo.ID, "key", key, "error", err)
				anyFailed = true
				processed++
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
				continue
			}

			// pHash computation from thumbnail data.
			// For videos with a thumbnail, the downloaded data is already a JPEG
			// image — use ComputeFromBytes, not ComputeVideoHash.
			// Only fall back to ComputeVideoHash when we had to download the
			// original video file (no thumbnail available).
			var hashVal uint64
			var hashErr error

			usedThumb := photo.S3KeyThumb != "" && key == photo.S3KeyThumb
			if photo.IsVideo && !usedThumb {
				durationS := 0.0
				if photo.DurationS != nil {
					durationS = *photo.DurationS
				}
				hashVal, hashErr = phash.ComputeVideoHash(data, durationS)
			} else {
				hashVal, hashErr = phash.ComputeFromBytes(data)
				if hashErr != nil {
					hashVal, hashErr = phash.ComputeViaFFmpeg(data)
				}
			}

			if hashErr != nil {
				slog.Warn("phash backfill: compute failed", "photo_id", photo.ID, "is_video", photo.IsVideo, "error", hashErr)
				anyFailed = true
			} else {
				if err := h.DB.UpdateActivityPhotoPHash(ctx, photo.ID, int64(hashVal)); err != nil {
					slog.Warn("phash backfill: db update pHash failed", "photo_id", photo.ID, "error", err)
					anyFailed = true
				}
			}

			// Dimensions backfill: download original if width is NULL
			if photo.Width == nil && !photo.IsVideo {
				origData, dlErr := downloadPhoto(photo, photo.S3Key)
				if dlErr != nil {
					slog.Warn("phash backfill: download original failed", "photo_id", photo.ID, "error", dlErr)
				} else if origData != nil {
					if img, _, decErr := image.Decode(bytes.NewReader(origData)); decErr == nil {
						w := img.Bounds().Dx()
						ht := img.Bounds().Dy()
						if err := h.DB.UpdateActivityPhotoDimensions(ctx, photo.ID, w, ht); err != nil {
							slog.Warn("phash backfill: db update dimensions failed", "photo_id", photo.ID, "error", err)
							anyFailed = true
						}
					} else {
						slog.Warn("phash backfill: image decode failed for dimensions", "photo_id", photo.ID, "error", decErr)
					}
				}
			}

			processed++
			_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
		}

		_ = h.DB.CompleteBackgroundJob(ctx, jobID, anyFailed, "")
		slog.Info("phash backfill: completed", "user_id", userID, "job_id", jobID, "processed", processed, "any_failed", anyFailed)
	}()

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"job_id": jobID,
		"queued": len(photos),
	})
}
