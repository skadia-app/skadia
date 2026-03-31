package importer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/sync/errgroup"

	"github.com/skadia-app/skadia/services/api/internal/automation"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/pmc"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
)

// SegmentMatcherFunc is a callback that runs background segment matching and lap detection.
// The handler package provides the concrete implementation.
type SegmentMatcherFunc func(activityID, userID, sportType, subType string, gearID *string, activityDate time.Time)

// Service holds shared dependencies needed to run post-insert processing steps
// after an activity has been written to the database. It is used by the upload
// handler and will be reused by the cross-platform import pipeline so that all
// import paths go through the same post-processing logic.
type Service struct {
	DB       *database.DB
	ShadowFS shadowfs.ShadowFS
	Events   *event.Publisher

	// Callback for segment matching that lives in the handler package to
	// avoid circular imports. Optional (nil is safe – the step is skipped).
	SegmentMatcher SegmentMatcherFunc

	// GraphReader provides tile-level graph access for surface enrichment.
	// Optional — nil when no tiles are loaded.
	GraphReader routing.GraphReader

	// PMC is the PMC service used to immediately compute TSS after import.
	// Optional — nil is safe; TSS will be populated on the next recompute.
	PMC *pmc.Service

	// PostImportHook is called once after a successful activity import.
	// Used to trigger fusion queue recompute. Optional — nil is safe.
	PostImportHook func(ctx context.Context, userID string)
}

// NewService constructs a Service with the provided dependencies.
// segmentMatcher may be nil; the step will be skipped.
func NewService(
	db *database.DB,
	store shadowfs.ShadowFS,
	events *event.Publisher,
	segmentMatcher SegmentMatcherFunc,
) *Service {
	return &Service{
		DB:             db,
		ShadowFS:       store,
		Events:         events,
		SegmentMatcher: segmentMatcher,
	}
}

// PostProcess runs all post-insert steps for a newly stored activity:
//  1. Phase 0 (sequential): Stage 1 Enrichment + ApplyRulesToActivity
//  2. Phase 1 (parallel):
//     Group A (sequential): InsertStreams → InsertTrack → InsertHeatmapData
//     Group C (parallel): RR-Intervals, Sensors+Devices, Laps, Splits, SplitSummaries,
//     Lengths, ClimbPro, TimeInZones, Events, Sets, Jumps, HrData, SegmentLaps
//     Group D (parallel): UpdateActivityFileInfo + S3 Meta Upload
//  3. DeveloperFieldDefs, Workout + Link, Stream-Enrichment, NATS Events, Shadow FS, Segment-Matching
func (s *Service) PostProcess(
	ctx context.Context,
	userID, activityID string,
	data *model.ActivityData,
	fileID, format, filename string,
) error {
	// Resolve activity from DB to get full model (GearID etc.)
	activity, err := s.DB.GetActivity(ctx, activityID)
	if err != nil || activity == nil {
		slog.Error("postprocess: failed to fetch activity", "id", activityID, "error", err)
		return err
	}

	// ── Phase 0 (sequential, MUST run first) ──────────────────────────────
	// RunStage1 + ApplyRulesToActivity set metadata used by subsequent steps.

	// Run for all activities (GPS and non-GPS): Weather/TimeOfDay/Sunrise have internal GPS guards.
	// Non-GPS activities still benefit from TRIMP, Intensity, EF, VI etc.
	// skipStreamEnrichment=true: activity_streams is written in Phase 1 (InsertStreams).
	// RunStreamEnrichment is called explicitly in Phase 2 after g.Wait().
	// skipGeo=true: activity_tracks is written in Phase 1 (InsertTrack).
	// RunGeoAndPublishStage2 is called explicitly in Phase 2 after g.Wait().
	enrichment.RunStage1(ctx, s.DB, userID, activityID, data, s.Events, true, true, s.GraphReader, true)

	if allRules, err := s.DB.ListRules(ctx, userID); err == nil {
		_ = automation.ApplyRulesToActivity(ctx, s.DB, activityID, userID, allRules, "on_import")
	}

	// ── Phase 1 (parallel groups) ─────────────────────────────────────────
	g, gCtx := errgroup.WithContext(ctx)

	// Group A (sequential): Streams → Track → Heatmap
	g.Go(func() error {
		// Streams (fatal on error — downstream depends on it)
		if err := s.DB.InsertStreams(gCtx, activityID, data.Streams); err != nil {
			slog.Error("insert streams failed", "error", err)
			return fmt.Errorf("insert streams: %w", err)
		}

		// Weather stream refinement (background, non-blocking)
		if enrichment.NeedsStreamRefinement(data.HasGPS, data.MinElevationM, data.MaxElevationM, data.ElapsedTimeS) {
			go s.runWeatherStreamRefinement(activityID, userID, data)
		}

		// Track (fatal on error)
		if err := s.DB.InsertTrack(gCtx, activityID, data.TrackPoints); err != nil {
			slog.Error("insert track failed", "error", err)
			return fmt.Errorf("insert track: %w", err)
		}

		// Heatmap (best-effort, logged only; skip for heatmap-excluded and group-secondary activities)
		if len(data.TrackPoints) >= 2 && !activity.ExcludeFromHeatmap && !activity.IsGroupSecondary {
			var subTypePtr *string
			if data.SubType != "" {
				subTypePtr = &data.SubType
			}
			interval, _ := s.DB.GetUserHeatmapInterval(gCtx, userID)
			if err := s.DB.InsertHeatmapData(gCtx, userID, activityID, data.SportType, subTypePtr, data.StartedAt, interval); err != nil {
				slog.Error("insert heatmap data failed", "error", err)
			}
			s.DB.InvalidateTerraIncognitaCache(gCtx, userID)
		}

		return nil
	})

	// Group C: independent inserts (all best-effort, logged only)
	g.Go(func() error {
		if len(data.RRIntervals) > 0 {
			if err := s.DB.InsertRRIntervals(gCtx, activityID, data.RRIntervals); err != nil {
				slog.Error("insert rr intervals failed", "error", err)
			}
		}
		return nil
	})

	g.Go(func() error {
		if err := s.DB.InsertSensors(gCtx, activityID, data.Sensors); err != nil {
			slog.Error("insert sensors failed", "error", err)
		}
		// Register Recording Device
		if data.RecordingDevice != nil {
			data.RecordingDevice.UserID = userID
			data.RecordingDevice.SeenAt = data.StartedAt
			deviceID, err := s.DB.UpsertHWDevice(gCtx, *data.RecordingDevice)
			if err != nil {
				slog.Error("upsert recording device failed", "activity", activityID, "error", err)
			} else if deviceID != "" {
				if err := s.DB.SetRecordingDeviceID(gCtx, activityID, deviceID); err != nil {
					slog.Error("set recording device id failed", "activity", activityID, "error", err)
				}
			}
		}
		// Register Sensor Devices
		for _, sensor := range data.Sensors {
			if sensor.DeviceUpsert == nil {
				continue
			}
			sensor.DeviceUpsert.UserID = userID
			sensor.DeviceUpsert.SeenAt = data.StartedAt
			deviceID, err := s.DB.UpsertHWDevice(gCtx, *sensor.DeviceUpsert)
			if err != nil {
				slog.Error("upsert sensor device failed", "activity", activityID, "error", err)
				continue
			}
			if deviceID != "" {
				if err := s.DB.SetSensorDeviceID(gCtx, activityID, sensor, deviceID); err != nil {
					slog.Error("set sensor device id failed", "activity", activityID, "error", err)
				}
			}
		}
		return nil
	})

	g.Go(func() error {
		if err := s.DB.InsertLaps(gCtx, activityID, data.Laps); err != nil {
			slog.Error("insert laps failed", "error", err)
		}
		return nil
	})

	g.Go(func() error {
		if err := s.DB.InsertSplits(gCtx, activityID, data.Splits); err != nil {
			slog.Error("insert splits failed", "error", err)
		}
		return nil
	})

	g.Go(func() error {
		if err := s.DB.InsertSplitSummaries(gCtx, activityID, data.SplitSummaries); err != nil {
			slog.Error("insert split summaries failed", "error", err)
		}
		return nil
	})

	g.Go(func() error {
		if err := s.DB.InsertLengths(gCtx, activityID, data.Lengths); err != nil {
			slog.Error("insert lengths failed", "error", err)
		}
		return nil
	})

	g.Go(func() error {
		if err := s.DB.InsertClimbProEvents(gCtx, activityID, data.ClimbProEvents); err != nil {
			slog.Error("insert climb pro events failed", "error", err)
		}
		return nil
	})

	g.Go(func() error {
		if len(data.TimeInZoneData) > 0 {
			if err := s.DB.SaveTimeInZones(gCtx, activityID, nil, nil, data.TimeInZoneData); err != nil {
				slog.Error("save time in zones failed", "error", err)
			}
		}
		for lapID, zones := range data.LapTimeInZones {
			id := lapID
			if err := s.DB.SaveTimeInZones(gCtx, activityID, &id, nil, zones); err != nil {
				slog.Warn("save lap time-in-zone", "err", err, "lap_id", id)
			}
		}
		for slID, zones := range data.SegmentLapTimeInZones {
			id := slID
			if err := s.DB.SaveTimeInZones(gCtx, activityID, nil, &id, zones); err != nil {
				slog.Warn("save segment-lap time-in-zone", "err", err, "segment_lap_id", id)
			}
		}
		return nil
	})

	g.Go(func() error {
		if err := s.DB.InsertActivityEvents(gCtx, activityID, data.Events); err != nil {
			slog.Error("insert activity events failed", "error", err)
		}
		return nil
	})

	g.Go(func() error {
		if err := s.DB.InsertActivitySets(gCtx, activityID, data.Sets); err != nil {
			slog.Error("insert activity sets failed", "error", err)
		}
		return nil
	})

	g.Go(func() error {
		if err := s.DB.InsertActivityJumps(gCtx, activityID, data.Jumps); err != nil {
			slog.Error("insert activity jumps failed", "error", err)
		}
		return nil
	})

	g.Go(func() error {
		if err := s.DB.InsertActivityHrData(gCtx, activityID, data.HrData); err != nil {
			slog.Error("insert activity hr data failed", "error", err)
		}
		return nil
	})

	g.Go(func() error {
		if err := s.DB.InsertActivitySegmentLaps(gCtx, activityID, data.SegmentLaps); err != nil {
			slog.Error("insert activity segment laps failed", "error", err)
		}
		return nil
	})

	g.Go(func() error {
		if data.DiveSettings != nil {
			data.DiveSettings.ActivityID = activityID
			if err := s.DB.InsertDiveSettings(gCtx, *data.DiveSettings); err != nil {
				slog.Error("insert dive settings failed", "error", err)
			}
		}
		if len(data.DiveGases) > 0 {
			for i := range data.DiveGases {
				data.DiveGases[i].ActivityID = activityID
			}
			if err := s.DB.InsertDiveGases(gCtx, data.DiveGases); err != nil {
				slog.Error("insert dive gases failed", "error", err)
			}
		}
		return nil
	})

	// Group D: file info + file_id linkage (parallel to everything else)
	g.Go(func() error {
		// Link file_id from ShadowFS.Put (set by caller) if not already set.
		// Multisport children must NOT get file_id — only the parent owns the file.
		isChild := activity != nil && activity.ParentID != nil
		if s.ShadowFS != nil && fileID != "" && !isChild {
			if _, execErr := s.DB.Pool.Exec(gCtx,
				`UPDATE activities SET file_id = $1 WHERE id = $2`,
				fileID, activityID,
			); execErr != nil {
				slog.Error("set file_id failed", "error", execErr)
			}
		}

		// Legacy S3 file info (kept until full migration to ShadowFS)
		if fileID == "" {
			// Update format/filename even without ShadowFS.
			if err := s.DB.UpdateActivityFileInfo(gCtx, activityID, format, filename); err != nil {
				slog.Error("update file info failed", "error", err)
			}
		}
		return nil
	})

	// Wait for all parallel groups. Group A errors (streams, track) are fatal.
	if err := g.Wait(); err != nil {
		return err
	}

	// ── Phase 2 (post-parallel) ───────────────────────────────────────────

	// Developer field definitions (idempotent upsert, no FK dependency)
	if len(data.DeveloperFieldDefs) > 0 {
		if err := s.DB.UpsertDeveloperFieldDefs(ctx, data.DeveloperFieldDefs); err != nil {
			slog.Error("upsert developer field defs failed", "error", err)
		}
	}

	// Workout + link to activity
	if data.Workout != nil {
		workoutID, err := s.DB.UpsertWorkout(ctx, userID, *data.Workout, data.WorkoutSteps)
		if err != nil {
			slog.Error("upsert workout failed", "error", err)
		} else if workoutID != "" {
			if err := s.DB.SetActivityWorkoutID(ctx, activityID, workoutID); err != nil {
				slog.Error("set activity workout id failed", "error", err)
			}
		}
	}

	// Stream-Enrichment runs AFTER InsertStreams (Phase 1) to avoid race condition
	// where activity_streams is still empty.
	if data.HasGPS {
		var streamSettings enrichment.StreamEnrichmentSettings
		if s, err := s.DB.GetEnrichmentSettings(ctx, userID); err != nil {
			slog.Warn("postprocess: could not load enrichment settings for stream enrichment", "user_id", userID, "error", err)
			streamSettings = enrichment.AllStreamEnabled()
		} else {
			streamSettings = enrichment.StreamSettingsFromModel(s)
		}
		if err := enrichment.RunStreamEnrichment(ctx, s.DB, userID, activityID, streamSettings); err != nil {
			slog.Warn("postprocess: stream enrichment failed (non-fatal)", "activity_id", activityID, "error", err)
		}
	}

	// Geo-Enrichment + Stage-2 NATS event run AFTER InsertTrack (Phase 1) because
	// all geo queries read from activity_tracks.track_full.
	// For non-GPS activities, geo queries are skipped internally but Stage-2 NATS event (embeddings) is still published.
	enrichment.RunGeoAndPublishStage2(ctx, s.DB, userID, activityID, data.HasGPS, data.SportType, data.IsVirtual, s.Events, s.GraphReader)

	// Trigger on_enrichment_complete rules after all synchronous enrichment steps finish.
	if allRules, err := s.DB.ListRules(ctx, userID); err == nil {
		if rulesErr := automation.ApplyRulesToActivity(ctx, s.DB, activityID, userID, allRules, "on_enrichment_complete"); rulesErr != nil {
			slog.Warn("on_enrichment_complete rules failed", "activity_id", activityID, "error", rulesErr)
		}
	}

	// Compute TSS immediately after import so computed_tss is populated
	// without requiring a manual recompute. Runs after InsertStreams (Phase 1)
	// so power/HR/speed streams are available for stream-based TSS methods.
	if s.PMC != nil {
		sport := data.FitSportID
		sub := data.FitSubSportID
		var avgPower, avgHR, distM *float64
		if data.AvgPowerW > 0 {
			v := data.AvgPowerW
			avgPower = &v
		}
		if data.AvgHeartRate > 0 {
			v := data.AvgHeartRate
			avgHR = &v
		}
		if data.DistanceM > 0 {
			v := data.DistanceM
			distM = &v
		}
		if err := s.PMC.ComputeActivityTSS(ctx, userID, activityID,
			sport, sub, data.MovingTimeS, avgPower, avgHR, distM, data.StartedAt); err != nil {
			slog.Warn("postprocess: ComputeActivityTSS failed (non-fatal)", "activity_id", activityID, "error", err)
		}
	}

	// Publish NATS events
	if s.Events != nil {
		evt := event.ActivityUploadedEvent{
			ActivityID: activityID,
			UserID:     userID,
			SportType:  data.SportType,
			HasGPS:     data.HasGPS,
		}
		if err := s.Events.PublishActivityUploaded(evt); err != nil {
			slog.Error("event publish failed", "error", err)
		}
		s.Events.PublishAnalyticsInvalidate(userID)
	}

	// Segment matching + lap detection (non-blocking background goroutine)
	if data.HasGPS && s.SegmentMatcher != nil {
		var gearIDPtr *string
		if activity.GearID != nil {
			gearIDPtr = activity.GearID
		}
		sportType := data.SportType
		subType := data.SubType
		startedAt := activity.StartedAt
		matcher := s.SegmentMatcher
		go matcher(activityID, userID, sportType, subType, gearIDPtr, startedAt)
	}

	// Write .meta v3 companion sidecar (backup/restore without DB).
	// Skip for multisport children — they share the parent's file.
	isChild := activity != nil && activity.ParentID != nil
	if !isChild {
		effectiveFileID := fileID
		if effectiveFileID == "" && activity != nil && activity.FileID != nil {
			effectiveFileID = *activity.FileID
		}
		sidecar.WriteMetaCompanion(ctx, s.ShadowFS, s.DB, userID, activityID, effectiveFileID, data, activity)
	}

	slog.Info("activity post-processed",
		"id", activityID,
		"sport", data.SportType,
		"format", format,
		"streams", len(data.Streams),
		"sensors", len(data.Sensors),
		"has_gps", data.HasGPS,
	)
	return nil
}

// runWeatherStreamRefinement re-fetches weather with summit/midpoint GPS after streams are inserted.
// Runs as a fire-and-forget goroutine with its own background context.
func (s *Service) runWeatherStreamRefinement(activityID, userID string, data *model.ActivityData) {
	ctx2, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prefs := model.DefaultWeatherPrefs()
	var minPtr, maxPtr *float64
	if data.MinElevationM != 0 {
		v := data.MinElevationM
		minPtr = &v
	}
	if data.MaxElevationM != 0 {
		v := data.MaxElevationM
		maxPtr = &v
	}

	var summitLat, summitLng *float64
	var summitAlt float64
	var summitAt *time.Time
	var midLat, midLng, midMeanAlt float64
	var midAt *time.Time

	c := enrichment.DetermineWeatherCase(data.ElapsedTimeS, data.MinElevationM, data.MaxElevationM, prefs)
	if c == enrichment.CaseMountain {
		if slat, slng, salt, sat, ok := enrichment.SummitPoint(ctx2, s.DB.Pool, activityID); ok {
			summitLat, summitLng, summitAlt = &slat, &slng, salt
			summitAt = &sat
		}
	}
	if c == enrichment.CaseMountain || c == enrichment.CaseLong {
		if mlat, mlng, malt, mat, ok := enrichment.MidpointGPS(ctx2, s.DB.Pool, activityID); ok {
			midLat, midLng, midMeanAlt = mlat, mlng, malt
			midAt = &mat
		}
	}

	reqs := enrichment.BuildWeatherRequests(
		data.StartLat, data.StartLng, data.StartedAt,
		data.ElapsedTimeS, minPtr, maxPtr,
		summitLat, summitLng, summitAlt,
		midLat, midLng, midMeanAlt,
		summitAt, midAt,
		prefs,
	)
	readings, err := enrichment.FetchWeatherReadings(ctx2, reqs)
	if err != nil {
		slog.Warn("stream weather refinement fetch failed", "activity_id", activityID, "error", err)
		return
	}
	if err := s.DB.InsertWeatherReadings(ctx2, activityID, readings); err != nil {
		slog.Warn("stream weather refinement insert failed", "activity_id", activityID, "error", err)
	} else {
		weatherPrefs, _ := s.DB.GetWeatherPrefs(ctx2, userID)
		if weatherPrefs.AutoTags {
			var allWeatherTags []string
			for _, r := range readings {
				allWeatherTags = append(allWeatherTags, r.Tags...)
			}
			if err := s.DB.SyncWeatherTagsToActivityTags(ctx2, activityID, userID, allWeatherTags); err != nil {
				slog.Warn("stream weather refinement: sync weather tags failed", "activity_id", activityID, "error", err)
			}
		}
	}
}

// ProcessActivity is the central entry point for all cross-platform imports.
// It runs duplicate detection (Tier 1), then either enriches/replaces an existing
// activity or inserts a new one. Platform IDs are backfilled in all cases.
func (s *Service) ProcessActivity(ctx context.Context, userID string, data *model.ActivityData, fileBytes []byte, meta ImportMetadata) (result *ActivityResult, retErr error) {
	defer func() {
		if s.PostImportHook != nil && retErr == nil && result != nil && result.Status != "skipped" {
			s.PostImportHook(ctx, userID)
		}
	}()
	// 1. Duplicate check (Tier 1)
	match, err := CheckTier1(ctx, s.DB, userID, fileBytes, data, meta.GarminActivityID, meta.StravaActivityID, meta.KomootTourID)
	if err != nil {
		return nil, fmt.Errorf("duplicate check: %w", err)
	}

	if match != nil {
		// Duplicate found — always backfill platform IDs regardless of strategy
		backfilled, _ := s.backfillPlatformIDs(ctx, match.ExistingActivityID, meta)

		strategy := meta.ConflictStrategy
		if strategy == "" {
			strategy = StrategyEnrichMissing
		}

		switch strategy {
		case StrategySkip:
			// Only backfill IDs, no enrichment or stream replacement
			status := "skipped"
			if len(backfilled) > 0 {
				status = "enriched" // we did backfill something
			}
			return &ActivityResult{
				ActivityID:            match.ExistingActivityID,
				Status:                status,
				PlatformIDsBackfilled: backfilled,
			}, nil

		case StrategyReplaceAll:
			// Replace all stream/child data unconditionally.
			// Note: caller must run PostProcess on "replaced" status to rebuild
			// heatmap, segments, and other derived data.
			if err := ReplaceActivity(ctx, s.DB, match.ExistingActivityID, data); err != nil {
				slog.Warn("replace activity streams failed", "activity", match.ExistingActivityID, "err", err)
			}
			return &ActivityResult{
				ActivityID:            match.ExistingActivityID,
				Status:                "replaced",
				PlatformIDsBackfilled: backfilled,
			}, nil

		default: // enrich_missing
			// Enrich scalar fields via COALESCE (existing values are never overwritten)
			existing, _ := s.DB.GetActivity(ctx, match.ExistingActivityID)
			enrichedCount, err := EnrichActivity(ctx, s.DB, match.ExistingActivityID, existing, data)
			if err != nil {
				slog.Warn("enrich duplicate failed", "activity", match.ExistingActivityID, "err", err)
			}

			// Child-table richness check: replace stream data if new data is richer
			streamsReplaced := false
			if existing != nil && s.compareStreamRichness(ctx, data, match.ExistingActivityID) > 0 {
				if err := ReplaceActivity(ctx, s.DB, match.ExistingActivityID, data); err != nil {
					slog.Warn("replace activity streams failed", "activity", match.ExistingActivityID, "err", err)
				} else {
					streamsReplaced = true
				}
			}

			status := "skipped"
			if enrichedCount > 0 || streamsReplaced || len(backfilled) > 0 {
				status = "enriched"
			}
			return &ActivityResult{
				ActivityID:            match.ExistingActivityID,
				Status:                status,
				PlatformIDsBackfilled: backfilled,
			}, nil
		}
	}

	// New activity: insert (with parent link for multisport children)
	var inserted *model.Activity
	if meta.ParentID != "" {
		inserted, err = s.DB.InsertActivityWithParent(ctx, userID, data, meta.ParentID)
	} else {
		inserted, err = s.DB.InsertActivity(ctx, userID, data)
	}
	if err != nil {
		// Unique constraint violation: concurrent upload of identical file — treat as duplicate.
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" && pgErr.ConstraintName == "activities_unique_file_hash" {
			existing, dbErr := s.DB.CheckDuplicate(ctx, userID, data.FileHash)
			if dbErr == nil && existing != nil {
				slog.Info("concurrent duplicate upload resolved via unique constraint", "existing", existing.ID)
				return &ActivityResult{ActivityID: existing.ID, Status: "skipped"}, nil
			}
			slog.Warn("unique constraint fired but conflicting activity not found", "user_id", userID, "constraint", pgErr.ConstraintName)
		}
		return nil, fmt.Errorf("insert activity: %w", err)
	}

	// Set platform IDs on the new activity (best-effort)
	s.backfillPlatformIDs(ctx, inserted.ID, meta) //nolint:errcheck

	return &ActivityResult{
		ActivityID: inserted.ID,
		Status:     "created",
	}, nil
}

// backfillPlatformIDs sets Garmin/Strava/Komoot IDs on an activity using
// COALESCE semantics (existing values are never overwritten by the DB functions).
// Returns the list of column names that were successfully written.
func (s *Service) backfillPlatformIDs(ctx context.Context, activityID string, meta ImportMetadata) ([]string, error) {
	var backfilled []string
	if meta.GarminActivityID > 0 {
		if err := s.DB.SetActivityGarminID(ctx, activityID, meta.GarminActivityID); err != nil {
			slog.Warn("backfill garmin_activity_id failed", "activity", activityID, "err", err)
		} else {
			backfilled = append(backfilled, "garmin_activity_id")
		}
	}
	if meta.StravaActivityID > 0 {
		if err := s.DB.SetActivityStravaID(ctx, activityID, meta.StravaActivityID); err != nil {
			slog.Warn("backfill strava_activity_id failed", "activity", activityID, "err", err)
		} else {
			backfilled = append(backfilled, "strava_activity_id")
		}
	}
	if meta.KomootTourID > 0 {
		if err := s.DB.SetActivityKomootTourID(ctx, activityID, meta.KomootTourID); err != nil {
			slog.Warn("backfill komoot_tour_id failed", "activity", activityID, "err", err)
		} else {
			backfilled = append(backfilled, "komoot_tour_id")
		}
	}
	return backfilled, nil
}

// compareStreamRichness returns a positive value if newData has more filled
// stream sensor columns than the existing activity's stored streams.
// It counts the distinct sensor channels present in newData.Streams and
// compares against the same count from the existing activity in the DB.
// A positive result means the new data is richer and child tables should be replaced.
func (s *Service) compareStreamRichness(ctx context.Context, newData *model.ActivityData, existingActivityID string) int {
	if newData == nil {
		return 0
	}
	// Count distinct non-nil sensor channels in new data
	var hasHR, hasCadence, hasPower, hasTemp, hasAlt, hasSpeed bool
	for _, sp := range newData.Streams {
		if sp.HeartRate != nil {
			hasHR = true
		}
		if sp.Cadence != nil {
			hasCadence = true
		}
		if sp.PowerW != nil {
			hasPower = true
		}
		if sp.TemperatureC != nil {
			hasTemp = true
		}
		if sp.AltitudeM != nil {
			hasAlt = true
		}
		if sp.SpeedMS != nil {
			hasSpeed = true
		}
	}
	newCount := 0
	if hasHR {
		newCount++
	}
	if hasCadence {
		newCount++
	}
	if hasPower {
		newCount++
	}
	if hasTemp {
		newCount++
	}
	if hasAlt {
		newCount++
	}
	if hasSpeed {
		newCount++
	}

	// Fetch existing channel count from DB
	existingCount, err := s.DB.CountStreamChannels(ctx, existingActivityID)
	if err != nil {
		slog.Warn("compareStreamRichness: failed to count existing channels, assuming 0", "activity", existingActivityID, "err", err)
		existingCount = 0
	}

	return newCount - existingCount
}

