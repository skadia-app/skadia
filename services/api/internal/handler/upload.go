package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/garmin"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/importutil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/tcximport"
)

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	maxUploadSize := h.Config.MaxUploadSize
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "file too large or invalid multipart form", r.URL.Path)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "missing 'file' field", r.URL.Path)
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to read file", r.URL.Path)
		return
	}

	// Detect format + decompress .gz if needed
	maxSize := h.Config.MaxUploadSize
	if maxSize <= 0 {
		maxSize = 50 << 20
	}
	decompressed, format, decompErr := detectFormatAndDecompress(fileBytes, header.Filename, maxSize*10)
	if decompErr != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", fmt.Sprintf("unsupported or unreadable file: %s", decompErr), r.URL.Path)
		return
	}

	// Calculate SHA256 hash on decompressed bytes
	hash := sha256.Sum256(decompressed)
	hashStr := hex.EncodeToString(hash[:])

	// Resolve user's conflict strategy for duplicate handling
	var conflictStrategy importer.ConflictStrategy
	if importSettings, settErr := h.DB.GetImportSettings(r.Context(), userID); settErr == nil && importSettings != nil {
		conflictStrategy = importer.ConflictStrategy(importSettings.ConflictStrategy)
	}

	filename := header.Filename

	// Route monitoring FIT files (health data, no activity created)
	if format == "fit" {
		ftype := garmin.ReadFITFileType(decompressed)
		if ftype == 68 || ftype == 44 {
			sz := int64(len(decompressed))
			tracker := h.DB.NewImportTracker(r.Context(), userID, "upload", header.Filename, &sz, nil)
			tracker.Processing(r.Context())
			tracker.SetFileHash(r.Context(), hashStr)
			monErr := garmin.ProcessMonitoringFIT(r.Context(), h.DB, h.ShadowFS, decompressed, userID)
			if monErr != nil {
				tracker.Error(r.Context(), monErr)
				httputil.WriteError(w, http.StatusUnprocessableEntity, "Process Error",
					fmt.Sprintf("failed to process monitoring FIT: %s", monErr), r.URL.Path)
				return
			}
			tracker.Completed(r.Context(), "")
			httputil.WriteJSON(w, http.StatusOK, map[string]any{
				"status":    "completed",
				"data_type": "monitoring",
				"fit_type":  ftype,
			})
			return
		}
		// ftype == 4 or unknown → continue to activity import below
	}

	// For FIT files try multi-session parsing to detect multisport activities.
	// GPX and TCX always produce a single activity.
	if format == "fit" {
		activities, parseErr := fitimport.ParseMultiSessionFIT(decompressed, filename)
		if parseErr != nil || len(activities) == 0 {
			// Fall back to single-session parse for error reporting
			singleData, singleErr := fitimport.ParseFIT(bytes.NewReader(decompressed), filename)
			if singleErr != nil {
				slog.Error("parse FIT failed", "error", singleErr)
				httputil.WriteError(w, http.StatusUnprocessableEntity, "Parse Error",
					fmt.Sprintf("failed to parse FIT file: %s", singleErr), r.URL.Path)
				return
			}
			activities = []*model.ActivityData{singleData}
		}

		if len(activities) > 1 {
			// Import log for multisport FIT activity
			msSz := int64(len(decompressed))
			msTracker := h.DB.NewImportTracker(r.Context(), userID, "upload", filename, &msSz, nil)
			msTracker.Processing(r.Context())
			msTracker.SetFileHash(r.Context(), hashStr)

			// Upload file to ShadowFS.
			var msFileID string
			rec, putErr := h.ShadowFS.Put(r.Context(), shadowfs.FileEntry{
				UserID:      userID,
				FileType:    "activity",
				ContentType: "application/octet-stream",
				FileName:    filename,
				SizeBytes:   int64(len(decompressed)),
				FileHash:    hashStr,
				Tags:        map[string]string{"format": format},
			}, bytes.NewReader(decompressed))
			if putErr != nil {
				slog.Error("shadowfs upload for multisport failed", "error", putErr)
				msTracker.Error(r.Context(), fmt.Errorf("ShadowFS upload: %w", putErr))
				httputil.WriteError(w, http.StatusInternalServerError, "Upload Error", "failed to store file", r.URL.Path)
				return
			}
			msFileID = rec.ID

			msResult, msErr := h.ImportService.ProcessMultisport(
				r.Context(), userID, activities, decompressed, hashStr,
				msFileID, format, filename,
				importer.ImportMetadata{SourcePlatform: "webui", ConflictStrategy: conflictStrategy},
			)
			if msErr != nil {
				msTracker.Error(r.Context(), msErr)
				httputil.WriteError(w, http.StatusInternalServerError, "Import Error", msErr.Error(), r.URL.Path)
				return
			}

			msTracker.Completed(r.Context(), msResult.ParentID)

			// Return parent activity
			parentAct, _ := h.DB.GetActivity(r.Context(), msResult.ParentID)
			if parentAct != nil {
				httputil.WriteJSON(w, http.StatusCreated, parentAct)
			} else {
				httputil.WriteJSON(w, http.StatusCreated, map[string]string{"parent_id": msResult.ParentID})
			}
			return
		}

		// Single-session FIT — fall through with the parsed data
		data := activities[0]

		data.FileHash = hashStr
		if data.SportType == "unknown" {
			slog.Warn("sport type unknown", "filename", filename, "format", format)
		}

		// Time overlap check
		var warnings []string
		if !data.StartedAt.IsZero() && !data.EndedAt.IsZero() {
			if overlap, oErr := h.DB.CheckTimeOverlap(r.Context(), userID, data.StartedAt, data.EndedAt); oErr != nil {
				slog.Error("time overlap check failed", "error", oErr)
			} else if overlap != nil {
				warnings = append(warnings, "time_overlap:"+overlap.ID)
			}
		}

		// Import log for single-session FIT activity
		sz := int64(len(decompressed))
		tracker := h.DB.NewImportTracker(r.Context(), userID, "upload", filename, &sz, nil)
		tracker.Processing(r.Context())
		tracker.SetFileHash(r.Context(), hashStr)

		fitResult, processErr := h.ImportService.ProcessActivity(r.Context(), userID, data, decompressed, importer.ImportMetadata{SourcePlatform: "webui", ConflictStrategy: conflictStrategy})
		if processErr != nil {
			slog.Error("process activity failed", "error", processErr)
			tracker.Error(r.Context(), processErr)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to process activity", r.URL.Path)
			return
		}

		if fitResult.Status == "skipped" {
			tracker.Skipped(r.Context(), "duplicate", nil)
			httputil.WriteJSON(w, http.StatusOK, map[string]any{
				"status":      "duplicate",
				"activity_id": fitResult.ActivityID,
			})
			return
		}

		// For created/enriched/replaced: upload to ShadowFS.
		// Look up existing file_id before Put so we can archive it on replace/enrich.
		var fitOldFileID string
		if fitResult.Status != "created" {
			_ = h.DB.Pool.QueryRow(r.Context(),
				`SELECT COALESCE(file_id, '') FROM activities WHERE id = $1`,
				fitResult.ActivityID,
			).Scan(&fitOldFileID)
		}

		var fitFileID string
		{
			rec, putErr := h.ShadowFS.Put(r.Context(), shadowfs.FileEntry{
				UserID:      userID,
				FileType:    "activity",
				ContentType: "application/octet-stream",
				FileName:    filename,
				SizeBytes:   int64(len(decompressed)),
				FileHash:    hashStr,
				Tags: map[string]string{
					"format": format,
					"year":   data.StartedAt.Format("2006"),
					"month":  data.StartedAt.Format("01"),
					"day":    data.StartedAt.Format("02"),
					"time":   data.StartedAt.Format("15-04"),
					"sport":  data.SportType,
				},
			}, bytes.NewReader(decompressed))
			if putErr != nil {
				slog.Error("shadowfs upload failed", "error", putErr)
				tracker.Error(r.Context(), fmt.Errorf("ShadowFS upload: %w", putErr))
				httputil.WriteError(w, http.StatusInternalServerError, "Storage Error", "failed to store original file", r.URL.Path)
				return
			}
			fitFileID = rec.ID
			// Archive the old file now that the new one is stored.
			// T31 (Reconciliation) will enforce retention limits.
			if fitOldFileID != "" {
				if archErr := h.ShadowFS.Archive(r.Context(), userID, fitOldFileID, fitFileID); archErr != nil {
					slog.Warn("archive old activity file failed", "old_file_id", fitOldFileID, "new_file_id", fitFileID, "err", archErr)
				}
			}
		}

		fitActivity, _ := h.DB.GetActivity(r.Context(), fitResult.ActivityID)
		if fitActivity == nil {
			tracker.Error(r.Context(), fmt.Errorf("failed to fetch inserted activity"))
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to fetch inserted activity", r.URL.Path)
			return
		}

		h.postInsertActivity(r.Context(), userID, fitActivity, data, fitFileID, format, filename)

		tracker.Completed(r.Context(), fitResult.ActivityID)

		switch fitResult.Status {
		case "enriched", "replaced":
			httputil.WriteJSON(w, http.StatusOK, map[string]any{
				"status":      fitResult.Status,
				"activity_id": fitResult.ActivityID,
			})
			return
		}

		// "created": physiological profile + full response
		if data.UserProfileImport != nil {
			if profErr := h.DB.SetUserProfileFieldsIfNull(r.Context(), userID, data.UserProfileImport); profErr != nil {
				slog.Warn("set user profile fields failed", "error", profErr)
			}
		}

		full, _ := h.DB.GetActivity(r.Context(), fitResult.ActivityID)
		if full == nil {
			full = fitActivity
		}
		httputil.WriteJSON(w, http.StatusCreated, model.UploadResponse{Activity: full, Warnings: warnings})
		return
	}

	// GPX / TCX path
	var data *model.ActivityData
	switch format {
	case "gpx":
		data, err = gpximport.ParseGPX(bytes.NewReader(decompressed), filename)
	case "tcx":
		data, err = tcximport.ParseTCX(bytes.NewReader(decompressed), filename)
	}
	if err != nil {
		slog.Error("parse failed", "format", format, "error", err)
		httputil.WriteError(w, http.StatusUnprocessableEntity, "Parse Error",
			fmt.Sprintf("failed to parse %s file: %s", format, err), r.URL.Path)
		return
	}

	data.FileHash = hashStr

	// Warn if sport type could not be determined
	if data.SportType == "unknown" {
		slog.Warn("sport type unknown, manual classification required",
			"filename", filename, "format", format)
	}

	// Time overlap check
	var warnings []string
	if !data.StartedAt.IsZero() && !data.EndedAt.IsZero() {
		overlap, overlapErr := h.DB.CheckTimeOverlap(r.Context(), userID, data.StartedAt, data.EndedAt)
		if overlapErr != nil {
			slog.Error("time overlap check failed", "error", overlapErr)
		} else if overlap != nil {
			warnings = append(warnings, "time_overlap:"+overlap.ID)
			slog.Warn("activity time overlaps with existing",
				"existing_id", overlap.ID, "filename", filename)
		}
	}

	// Import log for GPX/TCX activity
	sz := int64(len(decompressed))
	tracker := h.DB.NewImportTracker(r.Context(), userID, "upload", filename, &sz, nil)
	tracker.Processing(r.Context())
	tracker.SetFileHash(r.Context(), hashStr)

	gpxResult, processErr := h.ImportService.ProcessActivity(r.Context(), userID, data, decompressed, importer.ImportMetadata{SourcePlatform: "webui", ConflictStrategy: conflictStrategy})
	if processErr != nil {
		slog.Error("process activity failed", "error", processErr)
		tracker.Error(r.Context(), processErr)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to process activity", r.URL.Path)
		return
	}

	if gpxResult.Status == "skipped" {
		tracker.Skipped(r.Context(), "duplicate", nil)
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"status":      "duplicate",
			"activity_id": gpxResult.ActivityID,
		})
		return
	}

	// For created/enriched/replaced: upload to ShadowFS.
	// Look up existing file_id before Put so we can archive it on replace/enrich.
	var gpxOldFileID string
	if gpxResult.Status != "created" {
		_ = h.DB.Pool.QueryRow(r.Context(),
			`SELECT COALESCE(file_id, '') FROM activities WHERE id = $1`,
			gpxResult.ActivityID,
		).Scan(&gpxOldFileID)
	}

	contentType := "application/xml"

	var gpxFileID string
	{
		rec, putErr := h.ShadowFS.Put(r.Context(), shadowfs.FileEntry{
			UserID:      userID,
			FileType:    "activity",
			ContentType: contentType,
			FileName:    filename,
			SizeBytes:   int64(len(decompressed)),
			FileHash:    hashStr,
			Tags: map[string]string{
				"format": format,
				"year":   data.StartedAt.Format("2006"),
				"month":  data.StartedAt.Format("01"),
				"day":    data.StartedAt.Format("02"),
				"time":   data.StartedAt.Format("15-04"),
				"sport":  data.SportType,
			},
		}, bytes.NewReader(decompressed))
		if putErr != nil {
			slog.Error("shadowfs upload failed", "error", putErr)
			tracker.Error(r.Context(), fmt.Errorf("ShadowFS upload: %w", putErr))
			httputil.WriteError(w, http.StatusInternalServerError, "Storage Error", "failed to store original file", r.URL.Path)
			return
		}
		gpxFileID = rec.ID
		// Archive the old file now that the new one is stored.
		// T31 (Reconciliation) will enforce retention limits.
		if gpxOldFileID != "" {
			if archErr := h.ShadowFS.Archive(r.Context(), userID, gpxOldFileID, gpxFileID); archErr != nil {
				slog.Warn("archive old activity file failed", "old_file_id", gpxOldFileID, "new_file_id", gpxFileID, "err", archErr)
			}
		}
	}

	gpxActivity, _ := h.DB.GetActivity(r.Context(), gpxResult.ActivityID)
	if gpxActivity == nil {
		tracker.Error(r.Context(), fmt.Errorf("failed to fetch inserted activity"))
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to fetch inserted activity", r.URL.Path)
		return
	}

	h.postInsertActivity(r.Context(), userID, gpxActivity, data, gpxFileID, format, filename)

	tracker.Completed(r.Context(), gpxResult.ActivityID)

	if gpxResult.Status == "enriched" || gpxResult.Status == "replaced" {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"status":      gpxResult.Status,
			"activity_id": gpxResult.ActivityID,
		})
		return
	}

	// "created": re-fetch full activity for response
	full, err := h.DB.GetActivity(r.Context(), gpxResult.ActivityID)
	if err != nil || full == nil {
		full = gpxActivity
	}

	resp := model.UploadResponse{
		Activity: full,
		Warnings: warnings,
	}
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

// postInsertActivity runs all post-insert steps by delegating to the shared
// ImportService.PostProcess pipeline. This ensures that the upload handler and
// the cross-platform import pipeline use identical post-processing logic.
func (h *Handler) postInsertActivity(
	ctx context.Context,
	userID string,
	activity *model.Activity,
	data *model.ActivityData,
	fileID string,
	format string,
	filename string,
) {
	if err := h.ImportService.PostProcess(ctx, userID, activity.ID, data, fileID, format, filename); err != nil {
		slog.Error("post-process activity failed", "id", activity.ID, "error", err)
	}
}

func detectFormat(data []byte, filename string) string {
	return importutil.DetectFormat(data, filename)
}
