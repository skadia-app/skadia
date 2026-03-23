package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"github.com/skadia-app/skadia/services/api/internal/activityutil"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
	"github.com/skadia-app/skadia/services/api/internal/tcximport"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
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

// segCollector accumulates segment IDs that need rank updates across concurrent
// reprocessActivity calls. When non-nil it is passed to reprocessActivity to
// defer ranking until after all workers complete.
type segCollector struct {
	m sync.Map // key: segmentID (string) → struct{}
}

// add records a segment ID for deferred ranking.
func (c *segCollector) add(segID string) { c.m.Store(segID, struct{}{}) }

// forEach iterates over all collected segment IDs.
func (c *segCollector) forEach(fn func(segID string)) {
	c.m.Range(func(k, _ any) bool {
		fn(k.(string))
		return true
	})
}

// RecomputeActivity re-downloads the original file from S3, parses it again,
// and replaces all derived data (streams, track, sensors, metrics). User-defined
// fields (title, description, tags, visibility) are preserved.
//
// POST /api/v1/activities/{id}/recompute
func (h *Handler) RecomputeActivity(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	id := chi.URLParam(r, "id")

	// Verify ownership.
	activity, err := h.DB.GetActivity(r.Context(), id)
	if err != nil {
		slog.Error("reprocess: get activity failed", "id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	// Multisport parent: reprocess all children asynchronously via background job.
	if activity.SportType == "multisport" {
		children, err := h.DB.GetChildActivities(r.Context(), id)
		if err != nil {
			slog.Error("reprocess multisport: get children failed", "id", id, "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get child activities", r.URL.Path)
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
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create background job", r.URL.Path)
			return
		}

		go func() {
			ctx := context.Background()
			for i, child := range children {
				_ = h.DB.UpdateBackgroundJobPhase(ctx, jobID, fmt.Sprintf("Disziplin %d/%d", i+1, len(children)))
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(i))
				if err := h.reprocessActivity(ctx, child.ID, child.UserID, nil, 0, false, nil); err != nil {
					slog.Error("reprocess multisport: child failed", "parent_id", parentID, "child_id", child.ID, "error", err)
					_ = h.DB.CompleteBackgroundJob(ctx, jobID, true, err.Error())
					return
				}
			}
			_ = h.DB.UpdateBackgroundJobPhase(ctx, jobID, "Zusammenfassen")
			_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(len(children)))
			if aggrErr := h.DB.AggregateParentMetrics(ctx, parentID); aggrErr != nil {
				slog.Warn("reprocess multisport: aggregate failed", "id", parentID, "error", aggrErr)
			}
			_ = h.DB.CompleteBackgroundJob(ctx, jobID, false, "")
			slog.Info("reprocess multisport: done", "id", parentID, "children", len(children))
		}()

		httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
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

	// Run in background goroutine with context.Background — client disconnect
	// must NOT cancel the work. Returns 202 immediately with job_id for polling.
	activityName := activity.Title
	if activityName == nil {
		auto := activity.SportType
		activityName = &auto
	}
	jobID, jobErr := h.DB.InsertBackgroundJob(r.Context(), userID, "activity_recompute_all", &id, activityName, 1)
	if jobErr != nil {
		slog.Warn("reprocess: failed to insert background job", "id", id, "error", jobErr)
	}

	go func() {
		ctx := context.Background()
		if err := h.reprocessActivity(ctx, id, userID, scope, smoothWindow, false, nil); err != nil {
			slog.Error("reprocess: failed", "id", id, "error", err)
			if jobID != "" {
				_ = h.DB.CompleteBackgroundJob(ctx, jobID, true, err.Error())
			}
			return
		}
		slog.Info("reprocess: activity updated", "id", id)
		if jobID != "" {
			_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, 1)
			_ = h.DB.CompleteBackgroundJob(ctx, jobID, false, "")
		}
	}()

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"status": "running",
		"job_id": jobID,
	})
}

// RecomputeAllActivities starts an async background job that reprocesses all
// activities for the authenticated user. Returns immediately with 202 Accepted.
//
// POST /api/v1/activities/recompute-all
func (h *Handler) RecomputeAllActivities(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	// Parse optional filter from body.
	var filter *database.ReprocessFilter
	if r.Body != nil && r.ContentLength > 0 {
		var f database.ReprocessFilter
		if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid filter JSON", r.URL.Path)
			return
		}
		filter = &f
	}

	// Prevent double execution.
	h.recomputeJob.mu.Lock()
	if h.recomputeJob.running {
		h.recomputeJob.mu.Unlock()
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
		return
	}

	// List IDs while we still have the request context.
	ids, err := h.DB.ListActivityIDsFiltered(r.Context(), userID, filter)
	if err != nil {
		h.recomputeJob.mu.Unlock()
		slog.Error("reprocess-all: list ids failed", "user_id", userID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
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

	workers := recomputeWorkerCount()

	// Separater Done-Counter für korrekte Modulo-Checks unter Parallelität.
	// (processed + errors sind zwei separate atomics; wenn beide parallel inkrementiert
	// werden kann processed+errors%20 mehrfach denselben Wert liefern)
	var doneCounter atomic.Int64

	// Fehlersammlung (thread-safe, max. 5 Texte)
	var (
		errMu    sync.Mutex
		errTexts []string
	)

	// Shutdown-aware ID for shutdown.Go registry.
	shutdownID := dbJobID
	if shutdownID == "" {
		shutdownID = "recompute-all-" + jobID
	}

	shutdown.Go(shutdownID, func(ctx context.Context) {
		sem := make(chan struct{}, workers)
		var wg sync.WaitGroup

		// Collect all affected segment IDs across workers; rank once after wg.Wait().
		var coll segCollector

		for _, id := range ids {
			// Shutdown-Check: keine neuen Aktivitäten dispatchen wenn Shutdown läuft
			select {
			case <-ctx.Done():
				wg.Wait()
				h.recomputeJob.mu.Lock()
				h.recomputeJob.running = false
				h.recomputeJob.mu.Unlock()
				if dbJobID != "" {
					_ = h.DB.CompleteBackgroundJob(ctx, dbJobID, true, "aborted: server shutdown")
				}
				return
			case sem <- struct{}{}:
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()

				if err := h.reprocessActivity(ctx, id, userID, scope, 0, true, &coll); err != nil {
					slog.Error("reprocess-all: activity failed", "id", id, "error", err)
					h.recomputeJob.errors.Add(1)
					errMu.Lock()
					if len(errTexts) < 5 {
						errTexts = append(errTexts, fmt.Sprintf("%s: %v", id, err))
					}
					errMu.Unlock()
				} else {
					h.recomputeJob.processed.Add(1)
				}

				// Fortschritt in DB: eigener Counter verhindert Race beim Modulo-Check.
				if dbJobID != "" {
					n := doneCounter.Add(1)
					if n%5 == 0 {
						_ = h.DB.UpdateBackgroundJobProgress(ctx, dbJobID, n)
					}
				}
			}()
		}

		wg.Wait()

		// Deferred segment ranking: rank each affected segment exactly once,
		// regardless of how many activities matched it during bulk recompute.
		coll.forEach(func(segID string) {
			if err := h.DB.UpdateSegmentEffortRanks(ctx, segID); err != nil {
				slog.Warn("reprocess-all: deferred segment ranking failed", "segment_id", segID, "error", err)
			}
		})

		h.recomputeJob.mu.Lock()
		h.recomputeJob.running = false
		h.recomputeJob.mu.Unlock()

		processed := h.recomputeJob.processed.Load()
		errors := h.recomputeJob.errors.Load()
		total := int64(len(ids))

		if dbJobID != "" {
			// Finaler Progress-Flush (unabhängig von Modulo).
			_ = h.DB.UpdateBackgroundJobProgress(ctx, dbJobID, processed+errors)

			// Failed = mehr als 10% Aktivitäten hatten fatale Fehler.
			hasFailed := total > 0 && errors*100/total > 10

			var errorSummary string
			if errors > 0 {
				errMu.Lock()
				msgs := make([]string, len(errTexts))
				copy(msgs, errTexts)
				errMu.Unlock()
				remaining := errors - int64(len(msgs))
				if remaining > 0 {
					errorSummary = strings.Join(msgs, "; ") + fmt.Sprintf(" ... und %d weitere", remaining)
				} else {
					errorSummary = strings.Join(msgs, "; ")
				}
			}

			if err := h.DB.CompleteBackgroundJob(ctx, dbJobID, hasFailed, errorSummary); err != nil {
				slog.Warn("reprocess-all: complete job failed", "error", err)
			}
		}

		// Einmalige Analytics-Invalidierung nach Abschluss aller Workers
		// (per-activity wurde mit skipAnalyticsEvent=true unterdrückt).
		if h.Events != nil {
			h.Events.PublishAnalyticsInvalidate(userID)
		}

		slog.Info("reprocess-all: finished",
			"job_id", jobID,
			"user_id", userID,
			"total", len(ids),
			"processed", processed,
			"errors", errors,
			"workers", workers,
			"duration", time.Since(h.recomputeJob.startedAt).Round(time.Second),
		)
	})

	slog.Info("reprocess-all: job started",
		"job_id", jobID,
		"user_id", userID,
		"total", len(ids),
	)

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
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
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"status": "idle",
		})
		return
	}

	processed := h.recomputeJob.processed.Load()
	errors := h.recomputeJob.errors.Load()

	if running {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"status":    "running",
			"job_id":    jobID,
			"processed": processed,
			"total":     total,
			"errors":    errors,
		})
		return
	}

	// Job finished (completed).
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
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
//   - "geo": Stage 2 geo enrichment (summits, forests, water, stops, lifts, surfaces, borders)
//
// skipAnalyticsEvent suppresses PublishAnalyticsInvalidate when true (used in bulk recompute
// to avoid flooding — the caller publishes a single invalidation after all workers complete).
//
// segColl, when non-nil, causes segment ranking to be deferred: affected segment
// IDs are collected into segColl instead of ranked immediately. The caller is
// responsible for ranking them after all workers complete.
func (h *Handler) reprocessActivity(ctx context.Context, activityID, userID string, scope map[string]bool, smoothWindow int, skipAnalyticsEvent bool, segColl *segCollector) error {
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
		// Refresh heatmap data (lines + points)
		if err := h.DB.DeleteHeatmapData(ctx, activityID); err != nil {
			slog.Error("reprocess: delete heatmap data failed", "id", activityID, "error", err)
		}
		if len(data.TrackPoints) >= 2 {
			var subTypePtr *string
			if data.SubType != "" {
				subTypePtr = &data.SubType
			}
			interval, _ := h.DB.GetUserHeatmapInterval(ctx, userID)
			if err := h.DB.InsertHeatmapData(ctx, userID, activityID, data.SportType, subTypePtr, data.StartedAt, interval); err != nil {
				slog.Error("reprocess: insert heatmap data failed", "id", activityID, "error", err)
			}
			h.DB.InvalidateTerraIncognitaCache(ctx, userID)
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

		// Register Recording Device FK.
		if data.RecordingDevice != nil {
			data.RecordingDevice.UserID = userID
			data.RecordingDevice.SeenAt = data.StartedAt
			deviceID, err := h.DB.UpsertHWDevice(ctx, *data.RecordingDevice)
			if err != nil {
				slog.Error("recompute: upsert recording device failed", "error", err)
			} else if deviceID != "" {
				if err := h.DB.SetRecordingDeviceID(ctx, activityID, deviceID); err != nil {
					slog.Error("recompute: set recording device id failed", "error", err)
				}
			}
		}

		// Register Sensor Device FKs.
		for _, sensor := range data.Sensors {
			if sensor.DeviceUpsert != nil {
				sensor.DeviceUpsert.UserID = userID
				sensor.DeviceUpsert.SeenAt = data.StartedAt
				deviceID, err := h.DB.UpsertHWDevice(ctx, *sensor.DeviceUpsert)
				if err != nil {
					slog.Error("recompute: upsert sensor device failed", "error", err)
					continue
				}
				if deviceID != "" {
					if err := h.DB.SetSensorDeviceID(ctx, activityID, sensor, deviceID); err != nil {
						slog.Error("recompute: set sensor device id failed", "error", err)
					}
				}
			}
		}
	}

	// 5b. Delete + re-insert activity details (laps, splits, lengths, climb pro).
	if doDetails {
		// Batch all 5 deletes in one round-trip.
		detailBatch := &pgx.Batch{}
		detailBatch.Queue("DELETE FROM activity_laps WHERE activity_id = $1", activityID)
		detailBatch.Queue("DELETE FROM activity_splits WHERE activity_id = $1", activityID)
		detailBatch.Queue("DELETE FROM activity_split_summaries WHERE activity_id = $1", activityID)
		detailBatch.Queue("DELETE FROM activity_lengths WHERE activity_id = $1", activityID)
		detailBatch.Queue("DELETE FROM activity_climb_pro_events WHERE activity_id = $1", activityID)
		br := h.DB.Pool.SendBatch(ctx, detailBatch)
		for range 5 {
			if _, err := br.Exec(); err != nil {
				br.Close()
				return fmt.Errorf("batch delete details: %w", err)
			}
		}
		br.Close()

		// Parallel inserts via errgroup.
		g, gCtx := errgroup.WithContext(ctx)
		g.Go(func() error { return h.DB.InsertLaps(gCtx, activityID, data.Laps) })
		g.Go(func() error { return h.DB.InsertSplits(gCtx, activityID, data.Splits) })
		g.Go(func() error { return h.DB.InsertSplitSummaries(gCtx, activityID, data.SplitSummaries) })
		g.Go(func() error { return h.DB.InsertLengths(gCtx, activityID, data.Lengths) })
		g.Go(func() error { return h.DB.InsertClimbProEvents(gCtx, activityID, data.ClimbProEvents) })
		if err := g.Wait(); err != nil {
			return fmt.Errorf("insert details: %w", err)
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

	// 7d. Geo Stage-2 data cleanup (summits, forests, water, stops, lifts, surfaces, borders).
	// Stage 2 ML modules self-clean on re-run, but an explicit delete here ensures a clean
	// slate when "geo" scope is selected without a full recompute.
	if doGeo {
		geoBatch := &pgx.Batch{}
		geoBatch.Queue("DELETE FROM activity_geo_tags WHERE activity_id = $1", activityID)
		geoBatch.Queue("DELETE FROM activity_stops WHERE activity_id = $1", activityID)
		geoBatch.Queue("DELETE FROM activity_lift_candidates WHERE activity_id = $1", activityID)
		geoBatch.Queue("DELETE FROM activity_surfaces WHERE activity_id = $1", activityID)
		gbr := h.DB.Pool.SendBatch(ctx, geoBatch)
		for range 4 {
			if _, err := gbr.Exec(); err != nil {
				slog.Error("reprocess: batch delete geo data failed", "id", activityID, "error", err)
			}
		}
		gbr.Close()
	}

	// 8. Stage 1 Enrichment (synchronous, ~50ms). Triggers Stage 2 via NATS at the end.
	if data.HasGPS && data.StartLat != 0 {
		var gr routing.GraphReader
		if h.GraphManager != nil {
			gr = h.GraphManager.Reader()
		}
		enrichment.RunStage1(ctx, h.DB, userID, activityID, data, h.Events, false, false, gr)
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
		// Collect affected segment IDs before deletion (for rank updates).
		affectedSegIDs, _ := h.DB.GetAffectedSegmentIDs(ctx, activityID)

		// Delete existing efforts and detected laps.
		if err := h.DB.DeleteSegmentEffortsByActivity(ctx, activityID); err != nil {
			slog.Error("reprocess: delete segment efforts failed", "id", activityID, "error", err)
		}
		if err := h.DB.DeleteDetectedLaps(ctx, activityID); err != nil {
			slog.Error("reprocess: delete detected laps failed", "id", activityID, "error", err)
		}

		// Re-run segment matching synchronously (includes lap detection).
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

		// Lap detection.
		if h.DB.IsLapDetectionEnabled(ctx, userID) {
			gpsStream, gpsErr := h.DB.GetActivityGPSStream(ctx, activityID)
			if gpsErr == nil && len(gpsStream) >= 20 {
				if laps := DetectLaps(activityID, gpsStream); len(laps) > 0 {
					if err := h.DB.InsertDetectedLaps(ctx, activityID, laps); err != nil {
						slog.Error("reprocess: insert detected laps failed", "id", activityID, "error", err)
					}
				}
			}
		}

		// Segment rank updates: either defer into the collector (bulk recompute)
		// or update immediately (single-activity recompute).
		if segColl != nil {
			// Deferred: add all previously-affected IDs to the collector.
			// Newly-matched segment IDs are handled by MatchActivitySegments
			// which calls UpdateSegmentEffortRanks directly; here we ensure
			// segments that *lost* efforts also get re-ranked.
			for _, segID := range affectedSegIDs {
				segColl.add(segID)
			}
		} else {
			for _, segID := range affectedSegIDs {
				if err := h.DB.UpdateSegmentEffortRanks(ctx, segID); err != nil {
					slog.Warn("reprocess: update segment ranks failed", "segment_id", segID, "error", err)
				}
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
	// Im Bulk-Recompute (skipAnalyticsEvent=true) werden ALLE Events unterdrückt:
	// PublishActivityUploaded löst eine vollständige PMC-Neuberechnung aus — bei 700+
	// Aktivitäten parallel würde das die DB überlasten. Ein einziges Event wird am Ende
	// vom Caller (RecomputeAllActivities) via PublishAnalyticsInvalidate gefeuert.
	if h.Events != nil && !skipAnalyticsEvent {
		evt := event.ActivityUploadedEvent{
			ActivityID: activityID,
			UserID:     userID,
			SportType:  data.SportType,
			HasGPS:     data.HasGPS,
		}
		if err := h.Events.PublishActivityUploaded(evt); err != nil {
			slog.Error("reprocess: event publish failed", "id", activityID, "error", err)
		}
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

// recomputeWorkerCount liest RECOMPUTE_WORKERS aus der Umgebung (Default: 4).
// Gültige Werte: 1–32.
func recomputeWorkerCount() int {
	if s := os.Getenv("RECOMPUTE_WORKERS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= 32 {
			return n
		}
	}
	return 4
}
