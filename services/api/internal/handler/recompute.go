package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/activityutil"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
	"github.com/skadia-app/skadia/services/api/internal/tcximport"
)

// recomputeJobState tracks the in-memory state of an async recompute-all job.
type recomputeJobState struct {
	mu        sync.Mutex
	running   bool
	jobID     string
	total     int
	processed atomic.Int64
	errors    atomic.Int64
	startedAt time.Time
}

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

	// Multisport parent: reprocess all children asynchronously via background job.
	if activity.SportType == "multisport" {
		children, err := h.DB.GetChildActivities(r.Context(), id)
		if err != nil {
			slog.Error("reprocess multisport: get children failed", "id", id, "error", err)
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get child activities", r.URL.Path)
			return
		}
		parentID := id
		parentName := activity.Title
		if parentName == nil {
			auto := "Multisport"
			parentName = &auto
		}
		jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "recompute_multisport", &parentID, parentName, int64(len(children)+1))
		if err != nil {
			slog.Error("reprocess multisport: create job failed", "id", id, "error", err)
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create background job", r.URL.Path)
			return
		}

		go func() {
			ctx := context.Background()
			for i, child := range children {
				_ = h.DB.UpdateBackgroundJobPhase(ctx, jobID, fmt.Sprintf("Disziplin %d/%d", i+1, len(children)))
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(i))
				if err := h.reprocessActivity(ctx, child.ID, child.UserID, nil, 0); err != nil {
					slog.Error("reprocess multisport: child failed", "parent_id", parentID, "child_id", child.ID, "error", err)
					_ = h.DB.CompleteBackgroundJob(ctx, jobID, true)
					return
				}
			}
			_ = h.DB.UpdateBackgroundJobPhase(ctx, jobID, "Zusammenfassen")
			_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(len(children)))
			if aggrErr := h.DB.AggregateParentMetrics(ctx, parentID); aggrErr != nil {
				slog.Warn("reprocess multisport: aggregate failed", "id", parentID, "error", aggrErr)
			}
			_ = h.DB.CompleteBackgroundJob(ctx, jobID, false)
			slog.Info("reprocess multisport: done", "id", parentID, "children", len(children))
		}()

		writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
		return
	}

	// Parse optional scope from query string: ?scope=speed_smooth&scope=streams
	var scope map[string]bool
	if scopes := r.URL.Query()["scope"]; len(scopes) > 0 {
		scope = make(map[string]bool, len(scopes))
		for _, s := range scopes {
			scope[s] = true
		}
	}

	// Parse optional smooth_window as half-radius (1-3, default 2).
	// Total window = 2*smooth_window + 1 (always odd for clean median).
	smoothWindow := activityutil.DefaultSpeedSmoothingWindow
	if w := r.URL.Query().Get("smooth_window"); w != "" {
		if wn, err := strconv.Atoi(w); err == nil && wn >= 1 && wn <= 3 {
			smoothWindow = wn
		}
	}

	if err := h.reprocessActivity(r.Context(), id, userID, scope, smoothWindow); err != nil {
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

// RecomputeAllActivities starts an async background job that reprocesses all
// activities for the authenticated user. Returns immediately with 202 Accepted.
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

	// Prevent double execution.
	h.recomputeJob.mu.Lock()
	if h.recomputeJob.running {
		h.recomputeJob.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
		return
	}

	// List IDs while we still have the request context.
	ids, err := h.DB.ListActivityIDsFiltered(r.Context(), userID, filter)
	if err != nil {
		h.recomputeJob.mu.Unlock()
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

	// Initialize job state.
	jobID := uuid.New().String()
	h.recomputeJob.running = true
	h.recomputeJob.jobID = jobID
	h.recomputeJob.total = len(ids)
	h.recomputeJob.processed.Store(0)
	h.recomputeJob.errors.Store(0)
	h.recomputeJob.startedAt = time.Now()
	h.recomputeJob.mu.Unlock()

	// Job in DB anlegen für persistente Sichtbarkeit über Browser-Reload.
	dbJobID, dbJobErr := h.DB.InsertBackgroundJob(r.Context(), userID, "activity_recompute_all", nil, nil, int64(len(ids)))
	if dbJobErr != nil {
		slog.Warn("reprocess-all: failed to insert job", "error", dbJobErr)
		dbJobID = ""
	}

	// Launch background goroutine. Uses a detached context so it survives
	// after the HTTP response is sent.
	go func() {
		ctx := context.Background()

		for _, id := range ids {
			if err := h.reprocessActivity(ctx, id, userID, scope, 0); err != nil {
				slog.Error("reprocess-all: activity failed", "id", id, "error", err)
				h.recomputeJob.errors.Add(1)
			} else {
				h.recomputeJob.processed.Add(1)
			}

			// Fortschritt alle 20 Aktivitäten in DB schreiben.
			if dbJobID != "" {
				done := h.recomputeJob.processed.Load() + h.recomputeJob.errors.Load()
				if done%20 == 0 {
					_ = h.DB.UpdateBackgroundJobProgress(ctx, dbJobID, done)
				}
			}
		}

		h.recomputeJob.mu.Lock()
		h.recomputeJob.running = false
		h.recomputeJob.mu.Unlock()

		if dbJobID != "" {
			hasFailed := h.recomputeJob.errors.Load() > 0
			if err := h.DB.CompleteBackgroundJob(ctx, dbJobID, hasFailed); err != nil {
				slog.Warn("reprocess-all: complete job failed", "error", err)
			}
		}

		slog.Info("reprocess-all: finished",
			"job_id", jobID,
			"user_id", userID,
			"total", len(ids),
			"processed", h.recomputeJob.processed.Load(),
			"errors", h.recomputeJob.errors.Load(),
			"duration", time.Since(h.recomputeJob.startedAt).Round(time.Second),
		)
	}()

	slog.Info("reprocess-all: job started",
		"job_id", jobID,
		"user_id", userID,
		"total", len(ids),
	)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_id": jobID,
		"status": "running",
		"total":  len(ids),
	})
}

// RecomputeStatus returns the current state of the async recompute-all job.
//
// GET /api/v1/activities/recompute-status
func (h *Handler) RecomputeStatus(w http.ResponseWriter, r *http.Request) {
	h.recomputeJob.mu.Lock()
	running := h.recomputeJob.running
	jobID := h.recomputeJob.jobID
	total := h.recomputeJob.total
	h.recomputeJob.mu.Unlock()

	// No job has ever been started.
	if jobID == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "idle",
		})
		return
	}

	processed := h.recomputeJob.processed.Load()
	errors := h.recomputeJob.errors.Load()

	if running {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "running",
			"job_id":    jobID,
			"processed": processed,
			"total":     total,
			"errors":    errors,
		})
		return
	}

	// Job finished (completed).
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "completed",
		"job_id":    jobID,
		"processed": processed,
		"total":     total,
		"errors":    errors,
	})
}

// reprocessActivity is the shared core logic: download -> parse -> selectively replace data.
// scope controls which categories to reprocess (nil = all):
//   - "streams": streams + track
//   - "sensors": sensor records
//   - "details": laps, splits, split summaries, lengths, climb pro events
//   - "metrics": all computed metrics (times, distances, HR, power, etc.)
//   - "sport_type": sport_type + sub_type (only if not manually set)
//   - "tags": re-evaluate autotag rules and update tags
//   - "segments": segment efforts + lap detection
//   - "geo": Stage 2 geo enrichment (peaks, forests, water, stops, lifts, surfaces, borders)
func (h *Handler) reprocessActivity(ctx context.Context, activityID, userID string, scope map[string]bool, smoothWindow int) error {
	allScope := len(scope) == 0 // nil/empty = reprocess everything
	doStreams := allScope || scope["streams"]
	doSensors := allScope || scope["sensors"]
	doMetrics := allScope || scope["metrics"]
	doSportType := allScope || scope["sport_type"]
	doTags := allScope || scope["tags"]
	doDetails := allScope || scope["details"]
	doSegments := allScope || scope["segments"]
	doGeo := allScope || scope["geo"]
	doSpeedSmooth := scope["speed_smooth"] // opt-in only, never part of "all"

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
		if len(data.RRIntervals) > 0 {
			if err := h.DB.InsertRRIntervals(ctx, activityID, data.RRIntervals); err != nil {
				slog.Error("reprocess: insert rr intervals failed", "id", activityID, "error", err)
			}
		}
		if err := h.DB.InsertTrack(ctx, activityID, data.TrackPoints); err != nil {
			return fmt.Errorf("insert track: %w", err)
		}
		// Refresh heatmap line
		if err := h.DB.DeleteHeatmapLines(ctx, activityID); err != nil {
			slog.Error("reprocess: delete heatmap lines failed", "id", activityID, "error", err)
		}
		if err := h.DB.DeleteHeatmapPoints(ctx, activityID); err != nil {
			slog.Error("reprocess: delete heatmap points failed", "id", activityID, "error", err)
		}
		if len(data.TrackPoints) >= 2 {
			var subTypePtr *string
			if data.SubType != "" {
				subTypePtr = &data.SubType
			}
			if err := h.DB.InsertHeatmapLine(ctx, userID, activityID, data.SportType, subTypePtr, data.StartedAt); err != nil {
				slog.Error("reprocess: insert heatmap line failed", "id", activityID, "error", err)
			}
			h.DB.InvalidateTerraIncognitaCache(ctx, userID)
			interval, _ := h.DB.GetUserHeatmapInterval(ctx, userID)
			if err := h.DB.InsertHeatmapPoints(ctx, userID, activityID, data.SportType, subTypePtr, data.StartedAt, interval); err != nil {
				slog.Error("reprocess: insert heatmap points failed", "id", activityID, "error", err)
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

	// 5b. Delete + re-insert activity details (laps, splits, lengths, climb pro).
	if doDetails {
		if err := h.DB.DeleteLaps(ctx, activityID); err != nil {
			return fmt.Errorf("delete laps: %w", err)
		}
		if err := h.DB.DeleteSplits(ctx, activityID); err != nil {
			return fmt.Errorf("delete splits: %w", err)
		}
		if err := h.DB.DeleteSplitSummaries(ctx, activityID); err != nil {
			return fmt.Errorf("delete split summaries: %w", err)
		}
		if err := h.DB.DeleteLengths(ctx, activityID); err != nil {
			return fmt.Errorf("delete lengths: %w", err)
		}
		if err := h.DB.DeleteClimbProEvents(ctx, activityID); err != nil {
			return fmt.Errorf("delete climb pro events: %w", err)
		}
		if err := h.DB.InsertLaps(ctx, activityID, data.Laps); err != nil {
			return fmt.Errorf("insert laps: %w", err)
		}
		if err := h.DB.InsertSplits(ctx, activityID, data.Splits); err != nil {
			return fmt.Errorf("insert splits: %w", err)
		}
		if err := h.DB.InsertSplitSummaries(ctx, activityID, data.SplitSummaries); err != nil {
			return fmt.Errorf("insert split summaries: %w", err)
		}
		if err := h.DB.InsertLengths(ctx, activityID, data.Lengths); err != nil {
			return fmt.Errorf("insert lengths: %w", err)
		}
		if err := h.DB.InsertClimbProEvents(ctx, activityID, data.ClimbProEvents); err != nil {
			return fmt.Errorf("insert climb pro events: %w", err)
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

	// 7d. Geo Stage-2 data cleanup (peaks, forests, water, stops, lifts, surfaces, borders).
	// Stage 2 ML modules self-clean on re-run, but an explicit delete here ensures a clean
	// slate when "geo" scope is selected without a full recompute.
	if doGeo {
		if err := h.DB.DeleteActivityGeoTags(ctx, activityID); err != nil {
			slog.Error("reprocess: delete geo tags failed", "id", activityID, "error", err)
		}
		if err := h.DB.DeleteActivityStops(ctx, activityID); err != nil {
			slog.Error("reprocess: delete activity stops failed", "id", activityID, "error", err)
		}
		if err := h.DB.DeleteActivityLiftCandidates(ctx, activityID); err != nil {
			slog.Error("reprocess: delete lift candidates failed", "id", activityID, "error", err)
		}
		if err := h.DB.DeleteActivitySurfaces(ctx, activityID); err != nil {
			slog.Error("reprocess: delete activity surfaces failed", "id", activityID, "error", err)
		}
	}

	// 8. Stage 1 Enrichment (synchronous, ~50ms). Triggers Stage 2 via NATS at the end.
	if data.HasGPS && data.StartLat != 0 {
		enrichment.RunStage1(ctx, h.DB, userID, activityID, data, h.Events)
	}

	// 8b. Apply automation rules (all activities — not just GPS, since Frost/Nacht/etc. apply to indoor too).
	// On full recompute: always run. On tags-only: run if !allScope.
	if allScope || (doTags && !allScope) {
		if err := h.reprocessTags(ctx, activityID, userID); err != nil {
			slog.Error("reprocess: tags update failed", "id", activityID, "error", err)
		}
	}

	// 8c. Segment-Efforts + Detected Laps recompute (when streams changed or full recompute).
	if data.HasGPS && (doStreams || doSegments || allScope) {
		// Collect affected segment IDs before deletion (for rank updates)
		affectedSegIDs, _ := h.DB.GetAffectedSegmentIDs(ctx, activityID)

		// Delete existing efforts and detected laps
		if err := h.DB.DeleteSegmentEffortsByActivity(ctx, activityID); err != nil {
			slog.Error("reprocess: delete segment efforts failed", "id", activityID, "error", err)
		}
		if err := h.DB.DeleteDetectedLaps(ctx, activityID); err != nil {
			slog.Error("reprocess: delete detected laps failed", "id", activityID, "error", err)
		}

		// Re-run segment matching synchronously (includes lap detection)
		var gearIDPtr *string
		var activityStartedAt time.Time
		activity, aErr := h.DB.GetActivity(ctx, activityID)
		if aErr == nil && activity != nil {
			if activity.GearID != nil {
				gearIDPtr = activity.GearID
			}
			activityStartedAt = activity.StartedAt
		}
		MatchActivitySegments(ctx, h.DB, activityID, userID, data.SportType, data.SubType, gearIDPtr, activityStartedAt)

		// Lap detection
		if h.DB.IsLapDetectionEnabled(ctx, userID) {
			gpsStream, gpsErr := h.DB.GetActivityGPSStream(ctx, activityID)
			if gpsErr == nil && len(gpsStream) >= 20 {
				if laps := DetectLaps(ctx, h.DB, activityID, gpsStream); len(laps) > 0 {
					if err := h.DB.InsertDetectedLaps(ctx, activityID, laps); err != nil {
						slog.Error("reprocess: insert detected laps failed", "id", activityID, "error", err)
					}
				}
			}
		}

		// Update ranks for previously affected segments
		for _, segID := range affectedSegIDs {
			if err := h.DB.UpdateSegmentEffortRanks(ctx, segID); err != nil {
				slog.Warn("reprocess: update segment ranks failed", "segment_id", segID, "error", err)
			}
		}
	}

	// 8d. Speed smoothing — opt-in only (not part of "all").
	// Loads existing streams, applies a median filter to speed_ms, and writes
	// the smoothed values back. Does not re-parse the original file.
	if doSpeedSmooth {
		w := smoothWindow
		if w <= 0 {
			w = activityutil.DefaultSpeedSmoothingWindow
		}
		streamPts, sErr := h.DB.GetStreamPoints(ctx, activityID)
		if sErr != nil {
			slog.Error("reprocess: load streams for speed smoothing failed", "id", activityID, "error", sErr)
		} else if len(streamPts) >= 3 {
			streamPts = activityutil.SmoothSpeed(streamPts, w)
			if err := h.DB.UpdateStreamSpeeds(ctx, activityID, streamPts); err != nil {
				slog.Error("reprocess: update smoothed speeds failed", "id", activityID, "error", err)
			} else {
				slog.Info("reprocess: speed smoothing applied", "activity_id", activityID, "window", smoothWindow)
			}
		}
	}

	// 9. Publish NATS event (triggers PMC recalculation).
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
		// Trigger analytics recompute for the user (graceful — never blocks recompute)
		h.Events.PublishAnalyticsInvalidate(userID)
	}

	return nil
}

// reprocessTags re-evaluates automation rules for an activity using the shared helper.
func (h *Handler) reprocessTags(ctx context.Context, activityID, userID string) error {
	allRules, err := h.DB.ListRules(ctx, userID)
	if err != nil {
		return fmt.Errorf("list rules: %w", err)
	}
	return h.ApplyRulesToActivity(ctx, activityID, userID, allRules)
}
