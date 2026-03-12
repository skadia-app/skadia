package komoot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/storage"
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
	Storage       *storage.S3Client
	ImportService *importer.Service // centralised duplicate detection + insert
	UserID        string
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
// Tier-0 (Komoot Tour-ID) and Tier-1 (file hash + temporal match) internally.
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
		if p.OnProgress != nil {
			p.OnProgress(idx+1, total, fmt.Sprintf("activities: %d/%d %s", idx+1, total, tour.Name))
		}

		// Date filter
		if p.Options.DateFrom != nil && tour.Date.Before(*p.Options.DateFrom) {
			result.Skipped++
			continue
		}
		if p.Options.DateTo != nil && tour.Date.After(*p.Options.DateTo) {
			result.Skipped++
			continue
		}

		// Download FIT (preferred) or GPX
		fileBytes, format, err := i.Client.DownloadFIT(ctx, tour.ID)
		if err != nil {
			slog.Warn("komoot activities: download failed", "tour_id", tour.ID, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("download tour %d: %s", tour.ID, err))
			continue
		}

		// Compute file hash for dedup
		sum := sha256.Sum256(fileBytes)
		hashStr := hex.EncodeToString(sum[:])

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

		// Apply Komoot sport mapping. Komoot is authoritative for sport type on its own tours —
		// FIT files from Komoot often have no valid sport field (returns "unknown" or "generic").
		if mapping, ok := KomootSportMapping[tour.Sport]; ok {
			if data.SportType == "" || data.SportType == "generic" || data.SportType == "unknown" {
				data.SportType = mapping.SportType
				data.SubType = mapping.SubType
			}
			if mapping.IsEbike {
				data.IsEbike = true
			}
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
			continue
		}

		// Always populate TourActivityMap — even for duplicates — so the PhotosImporter can look up the activity ID.
		result.TourActivityMap[tour.ID] = actResult.ActivityID

		if actResult.Status != "created" {
			result.Skipped++
			continue
		}

		activityID := actResult.ActivityID

		// Upload original file to S3
		var s3Key string
		if len(fileBytes) > 0 && p.Storage != nil {
			s3Key = fmt.Sprintf("%s/%s.%s", p.UserID, hashStr, format)
			contentType := "application/octet-stream"
			if format == "gpx" {
				contentType = "application/xml"
			}
			if err := p.Storage.Upload(ctx, s3Key, fileBytes, contentType); err != nil {
				slog.Warn("komoot activities: S3 upload failed", "tour_id", tour.ID, "error", err)
			}
		}

		// Set Komoot tour title (before PostProcess which may generate auto-title)
		if tour.Name != "" {
			_ = p.DB.UpdateEnrichmentFields(ctx, activityID, map[string]any{"title": tour.Name})
		}

		// PostProcess handles file info, sidecar, streams, track, heatmap, laps, segment matching, Stage 1 etc.
		if err := p.ImportService.PostProcess(ctx, p.UserID, activityID, data, fileBytes, s3Key, format, fmt.Sprintf("%d.%s", tour.ID, format)); err != nil {
			slog.Warn("komoot activities: PostProcess failed", "tour_id", tour.ID, "activity_id", activityID, "error", err)
		}

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
	if mapping, ok := KomootSportMapping[tour.Sport]; ok {
		data.SportType = mapping.SportType
		data.SubType = mapping.SubType
		data.IsEbike = mapping.IsEbike
	} else {
		data.SportType = "generic"
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
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
