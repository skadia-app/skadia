package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
	"github.com/skadia-app/skadia/services/api/internal/tcximport"
)

// RecomputeActivity re-downloads the original file from S3, parses it again,
// and replaces all derived data (streams, track, sensors, metrics). User-defined
// fields (title, description, tags, visibility) are preserved.
//
// POST /api/v1/activities/{id}/recompute
func (h *Handler) RecomputeActivity(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	id := chi.URLParam(r, "id")

	// Verify ownership.
	activity, err := h.DB.GetActivity(r.Context(), id)
	if err != nil {
		slog.Error("reprocess: get activity failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	if err := h.reprocessActivity(r.Context(), id, userID, nil); err != nil {
		slog.Error("reprocess: failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "Reprocess Error", err.Error(), r.URL.Path)
		return
	}

	// Return updated activity.
	updated, err := h.DB.GetActivity(r.Context(), id)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to reload activity", r.URL.Path)
		return
	}

	slog.Info("reprocess: activity updated", "id", id)
	writeJSON(w, http.StatusOK, updated)
}

// RecomputeAllActivities reprocesses activities for the authenticated user.
// Accepts optional filter criteria in the request body.
//
// POST /api/v1/activities/recompute-all
func (h *Handler) RecomputeAllActivities(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	// Parse optional filter from body.
	var filter *database.ReprocessFilter
	if r.Body != nil && r.ContentLength > 0 {
		var f database.ReprocessFilter
		if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
			writeError(w, http.StatusBadRequest, "Bad Request", "invalid filter JSON", r.URL.Path)
			return
		}
		filter = &f
	}

	ids, err := h.DB.ListActivityIDsFiltered(r.Context(), userID, filter)
	if err != nil {
		slog.Error("reprocess-all: list ids failed", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
		return
	}

	// Build scope set from filter.
	var scope map[string]bool
	if filter != nil && len(filter.Scope) > 0 {
		scope = make(map[string]bool, len(filter.Scope))
		for _, s := range filter.Scope {
			scope[s] = true
		}
	}

	var processed, errors int

	for _, id := range ids {
		if err := h.reprocessActivity(r.Context(), id, userID, scope); err != nil {
			slog.Error("reprocess-all: activity failed", "id", id, "error", err)
			errors++
			continue
		}
		processed++
	}

	slog.Info("reprocess-all: finished",
		"user_id", userID,
		"total", len(ids),
		"processed", processed,
		"errors", errors,
	)

	writeJSON(w, http.StatusOK, map[string]int{
		"processed": processed,
		"errors":    errors,
		"total":     len(ids),
	})
}

// reprocessActivity is the shared core logic: download → parse → selectively replace data.
// scope controls which categories to reprocess (nil = all):
//   - "streams": streams + track
//   - "sensors": sensor records
//   - "metrics": all computed metrics (times, distances, HR, power, etc.)
//   - "sport_type": sport_type + sub_type (only if not manually set)
func (h *Handler) reprocessActivity(ctx context.Context, activityID, userID string, scope map[string]bool) error {
	allScope := len(scope) == 0 // nil/empty = reprocess everything
	doStreams := allScope || scope["streams"]
	doSensors := allScope || scope["sensors"]
	doMetrics := allScope || scope["metrics"]
	doSportType := allScope || scope["sport_type"]

	// 1. Load reprocess info (S3 key, format, manual sport flag).
	info, err := h.DB.GetActivityReprocessInfo(ctx, activityID)
	if err != nil {
		return fmt.Errorf("load reprocess info: %w", err)
	}
	if info == nil || info.FileKey == "" {
		return fmt.Errorf("no original file stored for activity %s", activityID)
	}

	// 2. Download original file from Garage.
	fileBytes, err := h.Storage.Download(ctx, info.FileKey)
	if err != nil {
		return fmt.Errorf("download original file: %w", err)
	}

	// 3. Re-parse the file.
	var data *model.ActivityData
	switch info.FileFormat {
	case "fit":
		data, err = fitimport.ParseFIT(bytes.NewReader(fileBytes), info.OriginalFilename)
	case "gpx":
		data, err = gpximport.ParseGPX(bytes.NewReader(fileBytes), info.OriginalFilename)
	case "tcx":
		data, err = tcximport.ParseTCX(bytes.NewReader(fileBytes), info.OriginalFilename)
	default:
		return fmt.Errorf("unknown format %q for activity %s", info.FileFormat, activityID)
	}
	if err != nil {
		return fmt.Errorf("parse %s: %w", info.FileFormat, err)
	}

	// 4. Delete + re-insert streams & track.
	if doStreams {
		if err := h.DB.DeleteStreams(ctx, activityID); err != nil {
			return fmt.Errorf("delete streams: %w", err)
		}
		if err := h.DB.DeleteTrack(ctx, activityID); err != nil {
			return fmt.Errorf("delete track: %w", err)
		}
		if err := h.DB.InsertStreams(ctx, activityID, data.Streams); err != nil {
			return fmt.Errorf("insert streams: %w", err)
		}
		if err := h.DB.InsertTrack(ctx, activityID, data.TrackPoints); err != nil {
			return fmt.Errorf("insert track: %w", err)
		}
		// Refresh heatmap line
		if err := h.DB.DeleteHeatmapLines(ctx, activityID); err != nil {
			slog.Error("reprocess: delete heatmap lines failed", "id", activityID, "error", err)
		}
		if len(data.TrackPoints) >= 2 {
			if err := h.DB.InsertHeatmapLine(ctx, userID, activityID, data.SportType, data.StartedAt); err != nil {
				slog.Error("reprocess: insert heatmap line failed", "id", activityID, "error", err)
			}
		}
	}

	// 5. Delete + re-insert sensors.
	if doSensors {
		if err := h.DB.DeleteSensors(ctx, activityID); err != nil {
			return fmt.Errorf("delete sensors: %w", err)
		}
		if err := h.DB.InsertSensors(ctx, activityID, data.Sensors); err != nil {
			return fmt.Errorf("insert sensors: %w", err)
		}
	}

	// 6. Update metrics. skipSportType = manual OR user didn't select sport_type scope.
	if doMetrics || doSportType {
		skipSport := info.SportTypeIsManual || !doSportType
		if err := h.DB.UpdateActivityMetrics(ctx, activityID, data, skipSport); err != nil {
			return fmt.Errorf("update metrics: %w", err)
		}
	}

	// 7. Rewrite sidecar metadata.
	metaKey := info.FileKey + ".meta"
	meta := sidecar.GenerateMeta(activityID, data)
	metaBytes, mErr := meta.Marshal()
	if mErr == nil {
		if err := h.Storage.Upload(ctx, metaKey, metaBytes, "application/json"); err != nil {
			slog.Error("reprocess: meta upload failed", "id", activityID, "error", err)
		}
	}

	// 8. Publish NATS event (triggers PMC recalculation).
	if h.Events != nil {
		evt := event.ActivityUploadedEvent{
			ActivityID: activityID,
			UserID:     userID,
			SportType:  data.SportType,
			HasGPS:     data.HasGPS,
		}
		if err := h.Events.PublishActivityUploaded(evt); err != nil {
			slog.Error("reprocess: event publish failed", "id", activityID, "error", err)
		}
	}

	return nil
}
