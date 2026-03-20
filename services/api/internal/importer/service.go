package importer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/skadia-app/skadia/services/api/internal/automation"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/imageutil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/phash"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
	"github.com/skadia-app/skadia/services/api/internal/storage"
	"github.com/google/uuid"
)

// ShadowWriterFunc is a callback that writes the shadow file for an activity.
// The handler package provides this via (*Handler).WriteShadowForActivity.
type ShadowWriterFunc func(ctx context.Context, userID, activityID string, data *model.ActivityData, fileBytes []byte) error

// SegmentMatcherFunc is a callback that runs background segment matching and lap detection.
// The handler package provides the concrete implementation.
type SegmentMatcherFunc func(activityID, userID, sportType, subType string, gearID *string, activityDate time.Time)

// Service holds shared dependencies needed to run post-insert processing steps
// after an activity has been written to the database. It is used by the upload
// handler and will be reused by the cross-platform import pipeline so that all
// import paths go through the same post-processing logic.
type Service struct {
	DB      *database.DB
	Storage *storage.S3Client
	Events  *event.Publisher

	// Callbacks for operations that live in the handler package to avoid
	// circular imports. Both are optional (nil is safe – the step is skipped).
	ShadowWriter   ShadowWriterFunc
	SegmentMatcher SegmentMatcherFunc
}

// NewService constructs a Service with the provided dependencies.
// shadowWriter and segmentMatcher may be nil; those steps will be skipped.
func NewService(
	db *database.DB,
	s3 *storage.S3Client,
	events *event.Publisher,
	shadowWriter ShadowWriterFunc,
	segmentMatcher SegmentMatcherFunc,
) *Service {
	return &Service{
		DB:             db,
		Storage:        s3,
		Events:         events,
		ShadowWriter:   shadowWriter,
		SegmentMatcher: segmentMatcher,
	}
}

// PostProcess runs all post-insert steps for a newly stored activity:
//  1. Phase 0 (sequential): Stage 1 Enrichment + ApplyRulesToActivity
//  2. Phase 1 (parallel):
//     Group A (sequential): InsertStreams → InsertTrack → InsertHeatmapData
//     Group C (parallel): RR-Intervals, Sensors+Devices, Laps, Splits, SplitSummaries,
//                          Lengths, ClimbPro, TimeInZones
//     Group D (parallel): UpdateActivityFileInfo + S3 Meta Upload
//  3. NATS Events, Shadow FS, Background Segment-Matching
func (s *Service) PostProcess(
	ctx context.Context,
	userID, activityID string,
	data *model.ActivityData,
	fileBytes []byte,
	s3Key, format, filename string,
) error {
	// Resolve activity from DB to get full model (GearID etc.)
	activity, err := s.DB.GetActivity(ctx, activityID)
	if err != nil || activity == nil {
		slog.Error("postprocess: failed to fetch activity", "id", activityID, "error", err)
		return err
	}

	// ── Phase 0 (sequential, MUST run first) ──────────────────────────────
	// RunStage1 + ApplyRulesToActivity set metadata used by subsequent steps.

	if data.HasGPS && data.StartLat != 0 {
		// skipStreamEnrichment=true: activity_streams is written in Phase 1 (InsertStreams).
		// RunStreamEnrichment is called explicitly in Phase 2 after g.Wait().
		enrichment.RunStage1(ctx, s.DB, userID, activityID, data, s.Events, true, true)
	}

	if allRules, err := s.DB.ListRules(ctx, userID); err == nil {
		_ = automation.ApplyRulesToActivity(ctx, s.DB, activityID, userID, allRules)
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

		// Heatmap (best-effort, logged only)
		if len(data.TrackPoints) >= 2 {
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
			if err := s.DB.SaveTimeInZones(gCtx, activityID, data.TimeInZoneData); err != nil {
				slog.Error("save time in zones failed", "error", err)
			}
		}
		return nil
	})

	// Group D: file info + S3 meta sidecar (parallel to everything else)
	g.Go(func() error {
		metaKey := s3Key + ".meta"
		bucket := "skadia-originals"
		if err := s.DB.UpdateActivityFileInfo(gCtx, activityID, bucket, s3Key, format, filename, metaKey); err != nil {
			slog.Error("update file info failed", "error", err)
		}
		meta := sidecar.GenerateMeta(activityID, data)
		metaBytes, marshalErr := meta.Marshal()
		if marshalErr == nil {
			if err := s.Storage.Upload(gCtx, metaKey, metaBytes, "application/json"); err != nil {
				slog.Error("meta upload failed", "error", err)
			}
		}
		return nil
	})

	// Wait for all parallel groups. Group A errors (streams, track) are fatal.
	if err := g.Wait(); err != nil {
		return err
	}

	// ── Phase 2 (post-parallel) ───────────────────────────────────────────

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

	// Write to Shadow FS (best-effort)
	if s.ShadowWriter != nil {
		if err := s.ShadowWriter(ctx, userID, activityID, data, fileBytes); err != nil {
			slog.Error("shadow fs write failed", "id", activityID, "error", err)
		}
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
func (s *Service) ProcessActivity(ctx context.Context, userID string, data *model.ActivityData, fileBytes []byte, meta ImportMetadata) (*ActivityResult, error) {
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
			if _, err := EnrichActivity(ctx, s.DB, match.ExistingActivityID, existing, data); err != nil {
				slog.Warn("enrich duplicate failed", "activity", match.ExistingActivityID, "err", err)
			}

			// Child-table richness check: replace stream data if new data is richer
			if existing != nil && s.compareStreamRichness(ctx, data, match.ExistingActivityID) > 0 {
				if err := ReplaceActivity(ctx, s.DB, match.ExistingActivityID, data); err != nil {
					slog.Warn("replace activity streams failed", "activity", match.ExistingActivityID, "err", err)
				}
			}

			status := "enriched"
			if match.Confidence == "platform_id" && len(backfilled) == 0 {
				status = "skipped"
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
		return nil, fmt.Errorf("insert activity: %w", err)
	}

	// Set platform IDs on the new activity (best-effort)
	s.backfillPlatformIDs(ctx, inserted.ID, meta) //nolint:errcheck

	return &ActivityResult{
		ActivityID: inserted.ID,
		Status:     "created",
	}, nil
}

// MultisportResult is returned by ProcessMultisport.
type MultisportResult struct {
	ParentID      string
	ParentCreated bool
	Children      []*ActivityResult
}

// ProcessMultisport handles multi-session FIT files (e.g. triathlons).
// It creates a multisport parent (if not already present via file hash dedup),
// then routes each child session through ProcessActivity + PostProcess.
// Each child gets an individual file hash: SHA256(originalHash + ":" + sessionIndex).
func (s *Service) ProcessMultisport(
	ctx context.Context,
	userID string,
	sessions []*model.ActivityData,
	fileBytes []byte,
	fileHash string,
	s3Key string,
	format string,
	filename string,
	meta ImportMetadata,
) (*MultisportResult, error) {
	if len(sessions) < 2 {
		return nil, fmt.Errorf("ProcessMultisport requires >= 2 sessions, got %d", len(sessions))
	}

	result := &MultisportResult{}

	// 1. Parent dedup: check if multisport parent with this file hash already exists
	parentID, err := s.DB.FindMultisportParentByHash(ctx, userID, fileHash)
	if err != nil {
		slog.Warn("multisport parent lookup failed", "err", err)
	}

	if parentID != "" {
		// Parent already exists (re-import of same file)
		result.ParentID = parentID
		result.ParentCreated = false
	} else {
		// Create new parent
		parent := &model.Activity{
			ID:        newUUID(),
			UserID:    userID,
			SportType: "multisport",
			Title:     new("Multisport"),
			StartedAt: sessions[0].StartedAt,
			FileHash:  &fileHash,
		}
		if err := s.DB.InsertMultisportParent(ctx, parent); err != nil {
			return nil, fmt.Errorf("insert multisport parent: %w", err)
		}
		result.ParentID = parent.ID
		result.ParentCreated = true
	}

	// 2. Process each child session
	for i, childData := range sessions {
		// Skip multisport summary sessions
		if childData.SportType == "multisport" {
			continue
		}

		// Individual hash per child to avoid FileHash collisions
		childHash := fmt.Sprintf("%x", sha256.Sum256([]byte(fileHash+":"+fmt.Sprintf("%d", i))))
		childData.FileHash = childHash

		childMeta := meta
		childMeta.ParentID = result.ParentID

		actResult, err := s.ProcessActivity(ctx, userID, childData, fileBytes, childMeta)
		if err != nil {
			slog.Warn("multisport child import failed", "session", i, "sport", childData.SportType, "err", err)
			continue
		}
		result.Children = append(result.Children, actResult)

		// PostProcess only for newly created activities
		if actResult.Status == "created" {
			if err := s.PostProcess(ctx, userID, actResult.ActivityID, childData, fileBytes, s3Key, format, filename); err != nil {
				slog.Warn("multisport child postprocess failed", "activity", actResult.ActivityID, "err", err)
			}
		}
	}

	// 3. Re-aggregate parent metrics (always, to pick up any new/enriched children)
	if err := s.DB.AggregateParentMetrics(ctx, result.ParentID); err != nil {
		slog.Warn("aggregate parent metrics failed", "parent", result.ParentID, "err", err)
	}

	return result, nil
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

const mediaBucket = "skadia-media"

// ProcessPhoto centralizes all photo import logic: SHA256 exact dedup, pHash near-duplicate
// detection, thumbnail/preview generation, S3 upload, and DB insertion.
// It mirrors the logic of handler.UploadActivityPhoto but accepts raw bytes + PhotoMetadata
// instead of an HTTP request.
func (s *Service) ProcessPhoto(ctx context.Context, userID, activityID string, photoBytes []byte, meta PhotoMetadata) (*PhotoResult, error) {
	// Load enrichment settings once — used for pHash threshold, preview size, and clip frames.
	enrichSettings, err := s.DB.GetEnrichmentSettings(ctx, userID)
	if err != nil {
		slog.Warn("ProcessPhoto: get enrichment settings failed", "error", err)
	}

	// 1. SHA256 exact dedup
	hashArr := sha256.Sum256(photoBytes)
	fileHash := hex.EncodeToString(hashArr[:])

	if exists, err := s.DB.ActivityPhotoExistsByHash(ctx, userID, fileHash); err == nil && exists {
		return &PhotoResult{Status: "duplicate_exact"}, nil
	}

	// 2. Determine content type: trust meta.ContentType if provided; otherwise sniff bytes.
	ct := meta.ContentType
	if ct == "" {
		sniff := photoBytes
		if len(sniff) > 512 {
			sniff = sniff[:512]
		}
		ct = http.DetectContentType(sniff)
		// Fallback for HEIC/JXL/video that returns application/octet-stream
		if ct == "application/octet-stream" || ct == "application/zip" {
			ct = contentTypeFromFilename(meta.OriginalFilename)
		}
	}

	isVideo := strings.HasPrefix(ct, "video/")

	// 3. Compute pHash
	var pHashVal int64
	var pHashOk bool
	if isVideo {
		durationS := 30.0
		if meta.DurationS != nil {
			durationS = *meta.DurationS
		}
		if h, err := phash.ComputeVideoHash(photoBytes, durationS); err == nil {
			pHashVal = int64(h)
			pHashOk = true
		} else {
			slog.Warn("importer: video phash failed", "err", err)
		}
	} else {
		if h, err := phash.ComputeFromBytes(photoBytes); err == nil {
			pHashVal = int64(h)
			pHashOk = true
		} else {
			// Fallback for HEIC/JXL via ffmpeg
			if h2, err2 := phash.ComputeViaFFmpeg(photoBytes); err2 == nil {
				pHashVal = int64(h2)
				pHashOk = true
			} else {
				slog.Warn("importer: phash computation failed, importing with phash=NULL", "err", err)
			}
		}
	}

	// 4. pHash duplicate check against existing photos of this activity
	if pHashOk && activityID != "" {
		existing, err := s.DB.GetActivityPhotoHashes(ctx, activityID)
		if err == nil && len(existing) > 0 {
			pHashThreshold := 3
			if enrichSettings != nil && enrichSettings.PhashThresholdBits > 0 {
				pHashThreshold = enrichSettings.PhashThresholdBits
			}
			hashes := make([]uint64, len(existing))
			for i, e := range existing {
				if e.PHash != nil {
					hashes[i] = uint64(*e.PHash)
				}
			}
			bestIdx, _ := phash.FindBestMatch(uint64(pHashVal), hashes, pHashThreshold)
			if bestIdx >= 0 {
				matchedPhoto := existing[bestIdx]
				// Resolution check: replace if new image is strictly larger
				var newW, newH int
				if img, _, err := image.Decode(bytes.NewReader(photoBytes)); err == nil {
					newW = img.Bounds().Dx()
					newH = img.Bounds().Dy()
				}
				existingPixels := 0
				if matchedPhoto.Width != nil && matchedPhoto.Height != nil {
					existingPixels = *matchedPhoto.Width * *matchedPhoto.Height
				}
				newPixels := newW * newH
				if newPixels > existingPixels && existingPixels > 0 {
					slog.Info("importer: phash duplicate with higher resolution, replacing",
						"photo", matchedPhoto.ID, "old_pixels", existingPixels, "new_pixels", newPixels)

					// Generate new S3 keys based on new file hash
					ext := fileExtFromContentType(ct, meta.OriginalFilename)
					newS3Key := fmt.Sprintf("activity-photos/%s/%s%s", userID, fileHash, ext)
					newS3KeyThumb := fmt.Sprintf("activity-photos/%s/%s_thumb.jpg", userID, fileHash)
					newS3KeyPreview := ""

					// Generate thumbnail (200px)
					var thumbData []byte
					switch {
					case isVideo:
						thumbData, _ = imageutil.GenerateThumbnailFFmpeg(photoBytes, ext, 200, 0)
					case ct == "image/heic" || ct == "image/heif" || ct == "image/jxl":
						thumbData, _ = imageutil.GenerateThumbnailFFmpeg(photoBytes, ext, 200, 0)
					default:
						thumbData, _ = imageutil.Resize(photoBytes, 200, 200, 80)
					}

					// Generate preview
					var previewData []byte
					if isVideo {
						var midOffset float64
						if vm, err := imageutil.ProbeVideo(photoBytes, ext); err == nil && vm.DurationS > 2 {
							midOffset = vm.DurationS / 2
						}
						previewData, _ = imageutil.GenerateThumbnailFFmpeg(photoBytes, ext, 2560, midOffset)
					} else {
						prevMaxW, prevMaxH := 2048, 2048
						if enrichSettings != nil {
							if w, h := imageutil.PreviewSizePixels(enrichSettings.PhotoPreviewSize); w > 0 {
								prevMaxW, prevMaxH = w, h
							}
						}
						if ct == "image/heic" || ct == "image/heif" || ct == "image/jxl" {
							previewData, _ = imageutil.GenerateThumbnailFFmpeg(photoBytes, ext, prevMaxW, 0)
						} else {
							previewData, _ = imageutil.Resize(photoBytes, prevMaxW, prevMaxH, 85)
						}
					}

					// Upload original to S3
					if err := s.Storage.UploadToBucket(ctx, mediaBucket, newS3Key, photoBytes, ct); err != nil {
						return nil, fmt.Errorf("s3 upload replacement photo: %w", err)
					}

					// Upload thumbnail to S3
					if thumbData != nil {
						if err := s.Storage.UploadToBucket(ctx, mediaBucket, newS3KeyThumb, thumbData, "image/jpeg"); err != nil {
							slog.Warn("importer: s3 upload replacement thumb failed", "err", err)
							newS3KeyThumb = newS3Key
						}
					} else {
						newS3KeyThumb = newS3Key
					}

					// Upload preview to S3
					if previewData != nil {
						pk := fmt.Sprintf("activity-photos/%s/%s_preview.jpg", userID, fileHash)
						if err := s.Storage.UploadToBucket(ctx, mediaBucket, pk, previewData, "image/jpeg"); err != nil {
							slog.Warn("importer: s3 upload replacement preview failed", "err", err)
						} else {
							newS3KeyPreview = pk
						}
					}

					// Update DB with new file, dimensions, hash, and S3 keys
					if err := s.DB.ReplaceActivityPhotoFile(ctx, matchedPhoto.ID, newS3Key, newS3KeyThumb, newS3KeyPreview, int64(len(photoBytes)), fileHash, newW, newH, pHashVal); err != nil {
						slog.Error("importer: ReplaceActivityPhotoFile failed", "photo", matchedPhoto.ID, "err", err)
						return nil, fmt.Errorf("replace photo file in DB: %w", err)
					}

					// Best-effort delete old S3 keys
					if matchedPhoto.S3Key != "" && matchedPhoto.S3Key != newS3Key {
						if err := s.Storage.DeleteFromBucket(ctx, mediaBucket, matchedPhoto.S3Key); err != nil {
							slog.Warn("importer: failed to delete old S3 original", "key", matchedPhoto.S3Key, "err", err)
						}
					}
					if matchedPhoto.S3KeyThumb != "" && matchedPhoto.S3KeyThumb != newS3KeyThumb && matchedPhoto.S3KeyThumb != matchedPhoto.S3Key {
						if err := s.Storage.DeleteFromBucket(ctx, mediaBucket, matchedPhoto.S3KeyThumb); err != nil {
							slog.Warn("importer: failed to delete old S3 thumb", "key", matchedPhoto.S3KeyThumb, "err", err)
						}
					}
					if matchedPhoto.S3KeyPreview != "" && matchedPhoto.S3KeyPreview != newS3KeyPreview {
						if err := s.Storage.DeleteFromBucket(ctx, mediaBucket, matchedPhoto.S3KeyPreview); err != nil {
							slog.Warn("importer: failed to delete old S3 preview", "key", matchedPhoto.S3KeyPreview, "err", err)
						}
					}

					return &PhotoResult{PhotoID: matchedPhoto.ID, Status: "replaced", PHash: pHashVal}, nil
				}
				return &PhotoResult{PhotoID: matchedPhoto.ID, Status: "duplicate_phash", PHash: pHashVal}, nil
			}
		}
	}

	// 5. No duplicate — insert new photo.

	// Determine file extension for S3 key
	ext := fileExtFromContentType(ct, meta.OriginalFilename)

	// Video metadata via ffprobe
	var videoMeta *imageutil.VideoMeta
	if isVideo {
		if vm, err := imageutil.ProbeVideo(photoBytes, ext); err == nil {
			videoMeta = &vm
		} else {
			slog.Warn("importer: ffprobe failed", "filename", meta.OriginalFilename, "error", err)
		}
	}

	// Thumbnail (200px) — first frame for video/HEIC/JXL, otherwise imageutil.Resize
	var thumbData []byte
	switch {
	case isVideo:
		if td, err := imageutil.GenerateThumbnailFFmpeg(photoBytes, ext, 200, 0); err == nil {
			thumbData = td
		} else {
			slog.Warn("importer: ffmpeg video thumbnail failed", "filename", meta.OriginalFilename, "error", err)
		}
	case ct == "image/heic" || ct == "image/heif" || ct == "image/jxl":
		if td, err := imageutil.GenerateThumbnailFFmpeg(photoBytes, ext, 200, 0); err == nil {
			thumbData = td
		} else {
			slog.Warn("importer: ffmpeg image thumbnail failed", "ct", ct, "error", err)
		}
	default:
		thumbData, _ = imageutil.Resize(photoBytes, 200, 200, 80)
	}

	// Preview thumbnail (2048px for images; mid-frame for video)
	var previewData []byte
	s3KeyPreview := ""
	if isVideo {
		var midOffset float64
		if videoMeta != nil && videoMeta.DurationS > 2 {
			midOffset = videoMeta.DurationS / 2
		}
		if pd, err := imageutil.GenerateThumbnailFFmpeg(photoBytes, ext, 2560, midOffset); err == nil {
			previewData = pd
		} else {
			slog.Warn("importer: ffmpeg video preview failed", "filename", meta.OriginalFilename, "error", err)
		}
	} else {
		// Load user preview size setting; default to 2048px on error.
		maxW, maxH := 2048, 2048
		if enrichSettings != nil {
			if w, h := imageutil.PreviewSizePixels(enrichSettings.PhotoPreviewSize); w > 0 {
				maxW, maxH = w, h
			}
		}
		if ct == "image/heic" || ct == "image/heif" || ct == "image/jxl" {
			previewData, _ = imageutil.GenerateThumbnailFFmpeg(photoBytes, ext, maxW, 0)
		} else {
			previewData, _ = imageutil.Resize(photoBytes, maxW, maxH, 85)
		}
	}

	// Decode image dimensions for width/height columns
	var imgWidth, imgHeight int
	if !isVideo {
		if img, _, err := image.Decode(bytes.NewReader(photoBytes)); err == nil {
			imgWidth = img.Bounds().Dx()
			imgHeight = img.Bounds().Dy()
		}
	} else if videoMeta != nil {
		imgWidth = videoMeta.Width
		imgHeight = videoMeta.Height
	}

	// S3 paths
	s3Key := fmt.Sprintf("activity-photos/%s/%s%s", userID, fileHash, ext)
	s3KeyThumb := fmt.Sprintf("activity-photos/%s/%s_thumb.jpg", userID, fileHash)

	// Upload original
	if err := s.Storage.UploadToBucket(ctx, mediaBucket, s3Key, photoBytes, ct); err != nil {
		return nil, fmt.Errorf("s3 upload photo: %w", err)
	}

	// Upload thumbnail
	if thumbData != nil {
		if err := s.Storage.UploadToBucket(ctx, mediaBucket, s3KeyThumb, thumbData, "image/jpeg"); err != nil {
			slog.Warn("importer: s3 upload thumb failed", "err", err)
			s3KeyThumb = s3Key // fallback: original
		}
	} else {
		s3KeyThumb = s3Key
	}

	// Upload preview
	if previewData != nil {
		pk := fmt.Sprintf("activity-photos/%s/%s_preview.jpg", userID, fileHash)
		if err := s.Storage.UploadToBucket(ctx, mediaBucket, pk, previewData, "image/jpeg"); err != nil {
			slog.Warn("importer: s3 upload preview failed", "err", err)
		} else {
			s3KeyPreview = pk
		}
	}

	// Build DB model
	var actIDPtr *string
	if activityID != "" {
		actIDPtr = &activityID
	}
	hashCopy := fileHash
	photo := model.ActivityPhoto{
		UserID:           userID,
		ActivityID:       actIDPtr,
		S3Bucket:         mediaBucket,
		S3Key:            s3Key,
		S3KeyThumb:       s3KeyThumb,
		S3KeyPreview:     s3KeyPreview,
		OriginalFilename: meta.OriginalFilename,
		ContentType:      ct,
		FileSizeBytes:    int64(len(photoBytes)),
		FileHash:         &hashCopy,
		TakenAt:          meta.TakenAt,
		IsVideo:          isVideo,
	}

	// Location fields from metadata
	if meta.Lat != nil {
		photo.Lat = meta.Lat
		photo.Lng = meta.Lng
		if meta.LocationSource != "" {
			ls := meta.LocationSource
			photo.LocationSource = &ls
		}
	}

	// pHash
	if pHashOk {
		photo.PHash = &pHashVal
	}

	// Image dimensions
	if imgWidth > 0 {
		photo.Width = &imgWidth
		photo.Height = &imgHeight
	}

	// Video metadata
	if videoMeta != nil {
		photo.DurationS = &videoMeta.DurationS
		photo.VideoWidth = &videoMeta.Width
		photo.VideoHeight = &videoMeta.Height
	} else if meta.DurationS != nil {
		// Use caller-supplied video metadata as fallback
		photo.DurationS = meta.DurationS
		photo.VideoWidth = meta.VideoWidth
		photo.VideoHeight = meta.VideoHeight
	}

	saved, err := s.DB.InsertActivityPhoto(ctx, photo)
	if err != nil {
		return nil, fmt.Errorf("insert activity photo: %w", err)
	}

	// Publish CLIP embed event (best-effort)
	if s.Events != nil {
		clipFrames := 4
		if enrichSettings != nil && enrichSettings.VideoClipFrames > 0 {
			clipFrames = enrichSettings.VideoClipFrames
		}
		ev := event.PhotoEmbedEvent{
			PhotoID:         saved.ID,
			UserID:          userID,
			S3Bucket:        mediaBucket,
			S3Key:           s3Key,
			S3KeyPreview:    s3KeyPreview,
			ContentType:     ct,
			VideoClipFrames: clipFrames,
		}
		if err := s.Events.PublishPhotoEmbed(ev); err != nil {
			slog.Warn("importer: failed to publish photo embed event", "photo_id", saved.ID, "err", err)
		}
	}

	result := &PhotoResult{
		PhotoID: saved.ID,
		Status:  "created",
	}
	if pHashOk {
		result.PHash = pHashVal
	}
	return result, nil
}

const importQueueBucket = "skadia-media"
const importQueuePrefix = "import-queue"

// ProcessOrphanPhoto handles a photo without a known activity.
// If the photo's TakenAt timestamp falls within a known activity's window, it is
// immediately processed via ProcessPhoto (status: "matched").
// Otherwise the photo is stored in S3 under import-queue/{userID}/{hash} and a
// corresponding import_queue record (queue_type="image_no_match") is created
// (status: "queued").
func (s *Service) ProcessOrphanPhoto(ctx context.Context, userID string, photoBytes []byte, meta PhotoMetadata) (*OrphanResult, error) {
	// 1. Time-based activity lookup
	if meta.TakenAt != nil {
		activityID, err := s.DB.FindActivityIDAtTime(ctx, userID, *meta.TakenAt)
		if err != nil {
			slog.Warn("importer: FindActivityIDAtTime failed", "err", err)
		}
		if activityID != "" {
			result, err := s.ProcessPhoto(ctx, userID, activityID, photoBytes, meta)
			if err != nil {
				return nil, fmt.Errorf("process orphan photo (matched): %w", err)
			}
			return &OrphanResult{Status: "matched", ActivityID: activityID, QueueEntryID: result.PhotoID}, nil
		}
	}

	// 2. No match — store in S3 and enqueue for manual review
	hashArr := sha256.Sum256(photoBytes)
	fileHash := hex.EncodeToString(hashArr[:])

	s3Key := fmt.Sprintf("%s/%s/%s", importQueuePrefix, userID, fileHash)

	// Determine content type
	ct := meta.ContentType
	if ct == "" {
		sniff := photoBytes
		if len(sniff) > 512 {
			sniff = sniff[:512]
		}
		ct = http.DetectContentType(sniff)
		if ct == "application/octet-stream" || ct == "application/zip" {
			ct = contentTypeFromFilename(meta.OriginalFilename)
		}
	}

	// Upload original to S3
	if err := s.Storage.UploadToBucket(ctx, importQueueBucket, s3Key, photoBytes, ct); err != nil {
		return nil, fmt.Errorf("s3 upload orphan photo: %w", err)
	}

	// Generate thumbnail (200px) for the queue preview
	isVideo := strings.HasPrefix(ct, "video/")
	var thumbData []byte
	ext := fileExtFromContentType(ct, meta.OriginalFilename)
	switch {
	case isVideo:
		if td, err := imageutil.GenerateThumbnailFFmpeg(photoBytes, ext, 200, 0); err == nil {
			thumbData = td
		}
	case ct == "image/heic" || ct == "image/heif" || ct == "image/jxl":
		if td, err := imageutil.GenerateThumbnailFFmpeg(photoBytes, ext, 200, 0); err == nil {
			thumbData = td
		}
	default:
		thumbData, _ = imageutil.Resize(photoBytes, 200, 200, 80)
	}

	s3KeyThumb := ""
	if thumbData != nil {
		thumbKey := fmt.Sprintf("%s/%s/%s_thumb.jpg", importQueuePrefix, userID, fileHash)
		if err := s.Storage.UploadToBucket(ctx, importQueueBucket, thumbKey, thumbData, "image/jpeg"); err == nil {
			s3KeyThumb = thumbKey
		} else {
			slog.Warn("importer: orphan thumb upload failed", "err", err)
		}
	}

	fileSize := int64(len(photoBytes))
	bucket := importQueueBucket
	queueMeta := map[string]any{
		"filename": meta.OriginalFilename,
	}
	if s3KeyThumb != "" {
		queueMeta["s3_key_thumb"] = s3KeyThumb
	}

	queueItem := database.ImportQueueItem{
		UserID:           userID,
		QueueType:        "image_no_match",
		OriginalFilename: meta.OriginalFilename,
		ContentType:      ct,
		FileSizeBytes:    &fileSize,
		S3Bucket:         &bucket,
		S3Key:            &s3Key,
		Metadata:         queueMeta,
	}
	entryID, err := s.DB.CreateImportQueueItem(ctx, queueItem)
	if err != nil {
		return nil, fmt.Errorf("create import queue item: %w", err)
	}

	return &OrphanResult{Status: "queued", QueueEntryID: entryID}, nil
}

// ResolveQueuedPhoto assigns a queued image_no_match item to a specific activity.
// It loads the file from S3, runs ProcessPhoto, marks the queue item as resolved,
// and cleans up the temporary S3 files.
func (s *Service) ResolveQueuedPhoto(ctx context.Context, userID, queueEntryID, targetActivityID string) (*PhotoResult, error) {
	// 1. Load and validate queue item
	item, err := s.DB.GetImportQueueItem(ctx, queueEntryID, userID)
	if err != nil || item == nil {
		return nil, fmt.Errorf("queue item not found: %w", err)
	}
	if item.QueueType != "image_no_match" {
		return nil, fmt.Errorf("queue item is not an image_no_match entry (got %q)", item.QueueType)
	}
	// Ownership already enforced by GetImportQueueItem (WHERE user_id = $2)

	if item.S3Bucket == nil || item.S3Key == nil {
		return nil, fmt.Errorf("queue item has no stored S3 file")
	}

	// 2. Download from S3
	photoBytes, ct, err := s.Storage.DownloadFromBucket(ctx, *item.S3Bucket, *item.S3Key)
	if err != nil {
		return nil, fmt.Errorf("s3 download queued photo: %w", err)
	}
	if ct == "" {
		ct = item.ContentType
	}

	// 3. Process as a normal activity photo
	meta := PhotoMetadata{
		OriginalFilename: item.OriginalFilename,
		ContentType:      ct,
	}
	result, err := s.ProcessPhoto(ctx, userID, targetActivityID, photoBytes, meta)
	if err != nil {
		return nil, fmt.Errorf("process queued photo: %w", err)
	}

	if result.Status == "error" {
		return result, nil
	}

	// 4. Cleanup: mark resolved + delete S3 files
	if resolveErr := s.DB.ResolveImportQueueItem(ctx, queueEntryID, userID); resolveErr != nil {
		slog.Warn("importer: ResolveImportQueueItem failed", "id", queueEntryID, "err", resolveErr)
	}
	_ = s.Storage.DeleteFromBucket(ctx, *item.S3Bucket, *item.S3Key)
	if thumbKey, ok := item.Metadata["s3_key_thumb"].(string); ok && thumbKey != "" {
		_ = s.Storage.DeleteFromBucket(ctx, *item.S3Bucket, thumbKey)
	}

	return result, nil
}

// contentTypeFromFilename derives a MIME type from a filename's extension.
// Used as fallback when http.DetectContentType returns application/octet-stream.
func contentTypeFromFilename(filename string) string {
	dot := strings.LastIndex(filename, ".")
	if dot < 0 {
		return "application/octet-stream"
	}
	switch strings.ToLower(filename[dot:]) {
	case ".heic", ".heif":
		return "image/heic"
	case ".jxl":
		return "image/jxl"
	case ".avif":
		return "image/avif"
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".avi":
		return "video/x-msvideo"
	case ".mkv":
		return "video/x-matroska"
	case ".webm":
		return "video/webm"
	default:
		return "application/octet-stream"
	}
}

// fileExtFromContentType returns the canonical file extension (with dot) for a content type.
// Prefers the extension from the original filename when available.
func fileExtFromContentType(ct, originalFilename string) string {
	if dot := strings.LastIndex(originalFilename, "."); dot >= 0 {
		origExt := strings.ToLower(originalFilename[dot:])
		switch origExt {
		case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".tiff", ".tif",
			".heic", ".heif", ".avif", ".jxl",
			".mp4", ".m4v", ".mov", ".avi", ".mkv", ".webm":
			return origExt
		}
	}
	switch {
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "gif"):
		return ".gif"
	case strings.Contains(ct, "webp"):
		return ".webp"
	case strings.Contains(ct, "heic") || strings.Contains(ct, "heif"):
		return ".heic"
	case strings.Contains(ct, "avif"):
		return ".avif"
	case strings.Contains(ct, "jxl"):
		return ".jxl"
	case strings.Contains(ct, "video/mp4"):
		return ".mp4"
	case strings.Contains(ct, "quicktime"):
		return ".mov"
	case strings.Contains(ct, "x-msvideo"):
		return ".avi"
	case strings.Contains(ct, "x-matroska"):
		return ".mkv"
	case strings.Contains(ct, "webm"):
		return ".webm"
	default:
		return ".jpg"
	}
}

func newUUID() string { return uuid.New().String() }

//go:fix inline
func strPtr(s string) *string { return new(s) }
