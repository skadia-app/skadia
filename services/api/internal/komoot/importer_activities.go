package komoot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// ActivitiesImporter imports recorded tours (tour_recorded) from the Komoot API.
type ActivitiesImporter struct {
	Client *Client
}

func (i *ActivitiesImporter) Name() string        { return "activities" }
func (i *ActivitiesImporter) Description() string { return "Aufgezeichnete Touren" }
func (i *ActivitiesImporter) Priority() int       { return 20 }

// KomootImportParams bundles everything the Komoot importers need.
type KomootImportParams struct {
	DB            *database.DB
	ShadowFS      shadowfs.ShadowFS // nil when not configured
	ImportService *importer.Service // centralised duplicate detection + insert
	UserID        string
	BatchID       string  // correlates all import_log entries for one import run
	TourIDs       []int64 // nil/empty = all tours
	Options       importer.ImportOptions
	OnProgress    func(current, total int, msg string)
}

// KomootImportResult summarises the outcome of an import run.
type KomootImportResult struct {
	Imported        int
	Skipped         int
	Errors          []string
	TourActivityMap map[int64]string // komoot_tour_id → activity.ID (for photo importer)
}

// Import fetches all tour_recorded tours from Komoot and imports them as activities.
// Duplicate detection is delegated to ImportService.ProcessActivity which runs
// Tier-0 (Komoot Tour-ID) and Tier-1 (file hash + GPS identity) internally.
func (i *ActivitiesImporter) Import(ctx context.Context, p KomootImportParams) (*KomootImportResult, error) {
	result := &KomootImportResult{
		TourActivityMap: make(map[int64]string),
	}

	// Build knownIDs set for incremental sync optimization (skip API pages with all-known tours)
	knownIDs, err := buildKnownKomootIDs(ctx, p.DB, p.UserID)
	if err != nil {
		slog.Warn("komoot activities: cannot build known IDs set", "error", err)
		knownIDs = map[int64]bool{}
	}

	// Fetch tour list from Komoot API
	tours, err := i.Client.ListTours(ctx, knownIDs)
	if err != nil {
		return result, fmt.Errorf("komoot activities: list tours: %w", err)
	}

	// Filter to tour_recorded only, and optionally to specific TourIDs
	var recorded []Tour
	for _, t := range tours {
		if t.Type != "tour_recorded" {
			continue
		}
		if len(p.TourIDs) > 0 && !containsID(p.TourIDs, t.ID) {
			continue
		}
		recorded = append(recorded, t)
	}

	total := len(recorded)
	slog.Info("komoot activities: found tours to import", "count", total)

	for idx, tour := range recorded {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if p.OnProgress != nil {
			p.OnProgress(idx+1, total, fmt.Sprintf("activities: %d/%d %s", idx+1, total, tour.Name))
		}

		// --- Import log tracking ---
		logFilename := fmt.Sprintf("%d", tour.ID)
		var batchIDPtr *string
		if p.BatchID != "" {
			batchIDPtr = &p.BatchID
		}
		tracker := p.DB.NewImportTracker(ctx, p.UserID, "komoot", logFilename, nil, batchIDPtr)
		tracker.Processing(ctx)

		// Date filter
		if p.Options.DateFrom != nil && tour.Date.Before(*p.Options.DateFrom) {
			result.Skipped++
			tracker.Skipped(ctx, "date_filtered", nil)
			continue
		}
		if p.Options.DateTo != nil && tour.Date.After(*p.Options.DateTo) {
			result.Skipped++
			tracker.Skipped(ctx, "date_filtered", nil)
			continue
		}

		// Download FIT (preferred) or GPX
		fileBytes, format, err := i.Client.DownloadFIT(ctx, tour.ID)
		if err != nil {
			slog.Warn("komoot activities: download failed", "tour_id", tour.ID, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("download tour %d: %s", tour.ID, err))
			tracker.Error(ctx, err)
			continue
		}

		// Compute file hash for dedup
		sum := sha256.Sum256(fileBytes)
		hashStr := hex.EncodeToString(sum[:])
		tracker.SetFileHash(ctx, hashStr)

		// Parse file to get ActivityData
		var data *model.ActivityData
		switch format {
		case "fit":
			data, err = fitimport.ParseFIT(bytes.NewReader(fileBytes), fmt.Sprintf("%d.fit", tour.ID))
		case "gpx":
			data, err = gpximport.ParseGPX(bytes.NewReader(fileBytes), fmt.Sprintf("%d.gpx", tour.ID))
		}
		if err != nil || data == nil {
			slog.Warn("komoot activities: parse failed", "tour_id", tour.ID, "format", format, "error", err)
			// Fallback: build minimal ActivityData from tour metadata
			data = tourToActivityData(tour)
		}

		var conflictPlatformSnapshot komootSportConflictSnapshot
		var conflictFileSnapshot komootSportConflictSnapshot
		if mapping, ok := normalizedKomootSportMapping(tour.Sport, "user_id", p.UserID, "tour_id", tour.ID); ok {
			platformSnapshot := komootSportConflictSnapshot{
				Sport:      mapping.SportType,
				SubType:    mapping.SubType,
				IsElectric: mapping.IsElectric,
				IsIndoor:   mapping.IsIndoor,
				IsVirtual:  mapping.IsVirtual,
			}
			fileSnapshot := komootSportConflictSnapshot{
				Sport:      data.SportType,
				SubType:    data.SubType,
				IsElectric: data.IsElectric,
				IsIndoor:   data.IsIndoor,
				IsVirtual:  data.IsVirtual,
			}
			if komootSportConflictSnapshotsConflict(platformSnapshot, fileSnapshot) {
				conflictPlatformSnapshot = platformSnapshot
				conflictFileSnapshot = fileSnapshot
				data.SportConflictPending = true
			}
			applyKomootSportMapping(data, mapping)
		}

		// Map Komoot tour visibility to activity visibility
		if tour.Status == "public" {
			data.Visibility = "public"
		} else {
			data.Visibility = "private"
		}

		data.Source = "Komoot"
		data.FileHash = hashStr
		data.FileFormat = format
		if data.OriginalName == "" && tour.Name != "" {
			data.OriginalName = tour.Name
		}

		// Duplicate detection + insert via ImportService (handles Tier-0 via KomootTourID + Tier-1 hash/temporal)
		actResult, processErr := p.ImportService.ProcessActivity(ctx, p.UserID, data, fileBytes,
			importer.ImportMetadata{KomootTourID: tour.ID, SourcePlatform: "komoot", ConflictStrategy: p.Options.ConflictStrategy})
		if processErr != nil {
			slog.Warn("komoot activities: process failed", "tour_id", tour.ID, "error", processErr)
			result.Errors = append(result.Errors, fmt.Sprintf("process tour %d: %s", tour.ID, processErr))
			tracker.Error(ctx, processErr)
			continue
		}

		// Always populate TourActivityMap — even for duplicates — so the PhotosImporter can look up the activity ID.
		result.TourActivityMap[tour.ID] = actResult.ActivityID

		if data.SportConflictPending {
			if conflictFileSnapshot.Sport == "" {
				conflictFileSnapshot = komootSportConflictSnapshot{Sport: data.SportType, SubType: data.SubType, IsElectric: data.IsElectric, IsIndoor: data.IsIndoor, IsVirtual: data.IsVirtual}
			}
			if _, markErr := p.DB.Pool.Exec(ctx,
				`UPDATE activities SET sport_conflict_pending = TRUE WHERE id = $1 AND user_id = $2`,
				actResult.ActivityID, p.UserID,
			); markErr != nil {
				slog.Warn("komoot sport conflict: failed to mark activity", "activity", actResult.ActivityID, "error", markErr)
			}
			if qErr := createKomootSportConflictQueueEntry(ctx, p.DB, p.UserID, actResult.ActivityID, conflictPlatformSnapshot, conflictFileSnapshot, format); qErr != nil {
				slog.Warn("komoot sport conflict: failed to create queue entry", "activity", actResult.ActivityID, "error", qErr)
			}
		}

		if actResult.Status == "enriched" || actResult.Status == "replaced" {
			// M21: Run PostProcess for enriched/replaced so heatmap+streams stay up to date.
			if postErr := p.ImportService.PostProcess(ctx, p.UserID, actResult.ActivityID, data, "", format, fmt.Sprintf("%d.%s", tour.ID, format)); postErr != nil {
				slog.Warn("komoot activities: PostProcess failed (enriched/replaced)", "tour_id", tour.ID, "error", postErr)
			}
			result.Skipped++ // stats: not newly created
			tracker.Completed(ctx, actResult.ActivityID)
			continue
		}
		if actResult.Status == "skipped" {
			result.Skipped++
			tracker.Skipped(ctx, "duplicate", &actResult.ActivityID)
			continue
		}

		activityID := actResult.ActivityID

		// Upload original file via ShadowFS
		contentType := "application/octet-stream"
		if format == "gpx" || format == "tcx" {
			contentType = "application/xml"
		}
		var fileID string
		if len(fileBytes) > 0 && p.ShadowFS != nil {
			rec, putErr := p.ShadowFS.Put(ctx, shadowfs.FileEntry{
				UserID:      p.UserID,
				FileType:    "activity",
				ContentType: contentType,
				FileName:    fmt.Sprintf("%d.%s", tour.ID, format),
				SizeBytes:   int64(len(fileBytes)),
				FileHash:    hashStr,
				Tags:        shadowfs.ActivitySchemaTags(importer.ActivitySchemaTagsInputFromData(format, "komoot", data)),
			}, bytes.NewReader(fileBytes))
			if putErr != nil {
				slog.Warn("komoot activities: ShadowFS.Put failed", "tour_id", tour.ID, "error", putErr)
			} else {
				fileID = rec.ID
			}
		}

		// Set Komoot tour title (before PostProcess which may generate auto-title)
		if tour.Name != "" {
			_ = p.DB.UpdateEnrichmentFields(ctx, activityID, map[string]any{"title": tour.Name})
		}

		// PostProcess handles file info, streams, track, heatmap, laps, segment matching, Stage 1 etc.
		if err := p.ImportService.PostProcess(ctx, p.UserID, activityID, data, fileID, format, fmt.Sprintf("%d.%s", tour.ID, format)); err != nil {
			slog.Warn("komoot activities: PostProcess failed", "tour_id", tour.ID, "activity_id", activityID, "error", err)
		}

		tracker.Completed(ctx, activityID)
		result.Imported++
	}

	slog.Info("komoot activities import done",
		"imported", result.Imported,
		"skipped", result.Skipped,
		"errors", len(result.Errors),
	)
	return result, nil
}

// tourToActivityData converts Komoot tour metadata to a minimal ActivityData.
// Used as fallback when FIT/GPX parsing fails.
func tourToActivityData(tour Tour) *model.ActivityData {
	data := &model.ActivityData{
		Source:    "Komoot",
		StartedAt: tour.Date,
	}
	if mapping, ok := normalizedKomootSportMapping(tour.Sport, "tour_id", tour.ID); ok {
		data.SportType = mapping.SportType
		data.SubType = mapping.SubType
		data.IsElectric = mapping.IsElectric
		data.IsIndoor = mapping.IsIndoor
		data.IsVirtual = mapping.IsVirtual
	} else {
		data.SportType = "unknown"
	}
	if tour.Distance > 0 {
		data.DistanceM = tour.Distance
	}
	if tour.ElevationUp > 0 {
		data.ElevationGainM = tour.ElevationUp
	}
	if tour.Duration > 0 {
		data.ElapsedTimeS = int(tour.Duration)
		data.MovingTimeS = int(tour.Duration)
	}
	if tour.TimeInMotion > 0 {
		data.MovingTimeS = int(tour.TimeInMotion)
	}
	return data
}

func normalizedKomootSportMapping(komootSport string, logAttrs ...any) (SportMapping, bool) {
	resolved, err := sport.ResolveSport(sport.ResolveInput{Source: sport.SourceKomoot, RawSport: komootSport})
	if err != nil || !resolved.Resolved {
		attrs := []any{"source", "komoot", "raw_sport", strings.TrimSpace(komootSport)}
		attrs = append(attrs, logAttrs...)
		slog.Warn("komoot: unmapped sport type", attrs...)
		return SportMapping{}, false
	}
	modifiers := sport.ApplyModifierPatch(sport.ModifierValues{}, resolved.Patch)
	return SportMapping{
		SportType:  resolved.Tuple.SportType,
		SubType:    resolved.Tuple.SubType,
		IsElectric: modifiers.IsElectric,
		IsIndoor:   modifiers.IsIndoor,
		IsVirtual:  modifiers.IsVirtual,
	}, true
}

func applyKomootSportMapping(data *model.ActivityData, mapping SportMapping) {
	fileGroup := sport.SegmentGroup(data.SportType)
	komootGroup := sport.SegmentGroup(mapping.SportType)

	switch {
	case fileGroup == "generic":
		data.SportType = mapping.SportType
		data.SubType = mapping.SubType
	case komootGroup == "generic":
	case fileGroup == komootGroup:
		if data.SportType == mapping.SportType && (data.SubType == "" || data.SubType == "generic") && mapping.SubType != "" && mapping.SubType != "generic" {
			data.SubType = mapping.SubType
		}
	default:
	}

	if mapping.IsElectric {
		data.IsElectric = true
	}
	if mapping.IsIndoor {
		data.IsIndoor = true
	}
	if mapping.IsVirtual {
		data.IsVirtual = true
	}
}

type komootSportConflictSnapshot struct {
	Sport      string
	SubType    string
	IsElectric bool
	IsIndoor   bool
	IsVirtual  bool
}

func komootSportConflictSnapshotsConflict(platform, file komootSportConflictSnapshot) bool {
	if platform.Sport == "" || file.Sport == "" || file.Sport == "unknown" {
		return false
	}
	platformGroup := sport.SegmentGroup(platform.Sport)
	fileGroup := sport.SegmentGroup(file.Sport)
	if platformGroup == "generic" || fileGroup == "generic" {
		return false
	}
	if platformGroup != fileGroup {
		return true
	}
	if platform.Sport != file.Sport {
		return true
	}
	if platform.SubType != "" && platform.SubType != "generic" && file.SubType != "" && file.SubType != "generic" && platform.SubType != file.SubType {
		return true
	}
	return platform.IsElectric != file.IsElectric ||
		platform.IsIndoor != file.IsIndoor ||
		platform.IsVirtual != file.IsVirtual
}

func komootSportConflictMetadata(platform, file komootSportConflictSnapshot, fileFormat string) map[string]any {
	return map[string]any{
		"activity_id":          "",
		"file_sport":           file.Sport,
		"file_sub_type":        file.SubType,
		"file_is_electric":     file.IsElectric,
		"file_is_indoor":       file.IsIndoor,
		"file_is_virtual":      file.IsVirtual,
		"file_source":          fileFormat,
		"platform_sport":       platform.Sport,
		"platform_sub_type":    platform.SubType,
		"platform_is_electric": platform.IsElectric,
		"platform_is_indoor":   platform.IsIndoor,
		"platform_is_virtual":  platform.IsVirtual,
		"platform_source":      "komoot",
	}
}

func createKomootSportConflictQueueEntry(ctx context.Context, db *database.DB, userID, activityID string, platform, file komootSportConflictSnapshot, fileFormat string) error {
	exists, err := db.SportConflictQueueExists(ctx, userID, activityID)
	if err != nil || exists {
		return err
	}
	meta := komootSportConflictMetadata(platform, file, fileFormat)
	meta["activity_id"] = activityID
	_, err = db.CreateImportQueueItem(ctx, database.ImportQueueItem{
		UserID:    userID,
		QueueType: "sport_conflict",
		Metadata:  meta,
	})
	return err
}

// buildKnownKomootIDs fetches all known komoot_tour_ids for a user (for incremental sync).
func buildKnownKomootIDs(ctx context.Context, db *database.DB, userID string) (map[int64]bool, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT komoot_tour_id FROM activities WHERE user_id = $1 AND komoot_tour_id IS NOT NULL`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			result[id] = true
		}
	}
	return result, rows.Err()
}

// containsID checks if id is in ids.
func containsID(ids []int64, id int64) bool {
	return slices.Contains(ids, id)
}
