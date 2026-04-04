package bestefforts

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ptr returns a pointer to v. It is used to take the address of non-addressable
// composite-literal fields and function-return values without a named variable.
func ptr[T any](v T) *T { return &v }

// BestEffortRecord is the data written to and read from persistent storage.
// It mirrors database.BestEffort but lives here to avoid an import cycle
// (database already imports bestefforts for ActivityInfo).
type BestEffortRecord struct {
	ID             string
	UserID         string
	ActivityID     string
	Category       string
	SportGroup     string
	ElapsedTimeS   *int
	ValueNumeric   *float64
	AchievedAt     time.Time
	StreamStartIdx *int
	StreamEndIdx   *int
}

// BestEffortRankedRecord is a BestEffortRecord decorated with a per-user rank.
type BestEffortRankedRecord struct {
	BestEffortRecord
	Rank int
}

// Store is the persistence interface required by Service.
// It is implemented by *database.DB via adapter methods; the interface lives
// here to break the import cycle that would arise if service.go imported the
// database package (which already imports bestefforts for ActivityInfo).
type Store interface {
	UpsertBestEffortRec(ctx context.Context, be BestEffortRecord) error
	BatchUpsertBestEffortRecs(ctx context.Context, records []BestEffortRecord) error
	DeleteAllBestEfforts(ctx context.Context, userID string) error
	GetBestEffortsForActivityRec(ctx context.Context, activityID, userID string) ([]BestEffortRankedRecord, error)
	// GetBestEffortRecalcActivities replaces the former GetBestEffortRecalcIDs +
	// GetBestEffortActivityInfo pair used in RecalcAll. A single query returns
	// all eligible activities with full metadata, eliminating the N+1 pattern.
	GetBestEffortRecalcActivities(ctx context.Context, userID string) ([]ActivityInfo, error)
	// GetBestEffortActivityInfo is retained for single-activity compute paths
	// (e.g. triggered after import or edit).
	GetBestEffortActivityInfo(ctx context.Context, activityID string) (*ActivityInfo, error)
	GetStreamPoints(ctx context.Context, activityID string) ([]model.StreamPoint, error)
	// GetStreamPointsLightweight fetches only the 5 columns accessed by the
	// Compute* functions (time, distance_m, altitude_m, power_w, gps_accuracy_m).
	// See NOTE(E-6) in model/stream.go: integration into RecalcAll requires
	// refactoring the Compute* signatures first.
	GetStreamPointsLightweight(ctx context.Context, activityID string) ([]model.StreamPointLight, error)
}

// Service coordinates best-effort computation and persistence for a single
// activity or a full recalculation across all activities of a user.
type Service struct {
	Store Store
}

// ComputeForActivity computes all best-effort records for the given activity
// and upserts them into persistent storage. It returns the ranked results.
//
// Returns nil, nil when the activity's sport has no best-efforts semantics.
func (s *Service) ComputeForActivity(ctx context.Context, info ActivityInfo, stream []model.StreamPoint) ([]BestEffortRankedRecord, error) {
	sportGroup := DetermineSportGroup(info.SportType, info.SubType, info.IsIndoor, info.IsVirtual, info.IsElectric)
	if sportGroup == "" {
		return nil, nil
	}

	categories := CategoriesFor(sportGroup)

	// TODO(lift-indices): populate liftIndices from ski-lift segment stream indices once
	// the Python ML "lifts" enrichment module is migrated to Go and stores per-stream
	// start/end indices in the database. Currently activity_climbs stores only
	// distance-based ranges (start_distance_m / end_distance_m) with no stream index
	// mapping, so lift exclusion for elevation best efforts cannot be applied yet.
	liftIndices := map[int]bool{}

	var efforts []BestEffortRecord
	for _, cat := range categories {
		var result *EffortResult

		switch {
		case cat.DistanceM > 0:
			result = ComputeDistanceBestEffort(stream, cat)
		case cat.ElevationM > 0:
			result = ComputeElevationBestEffort(stream, cat, liftIndices)
		case cat.DurationS > 0:
			result = ComputePowerBestEffort(stream, cat)
		case cat.Type == TypeValueSuperlative:
			result = ComputeSuperlative(cat, info.DistanceM, info.ElevationGainM, info.BiggestClimbM, float64(info.ElapsedTimeS))
		}

		if result == nil {
			continue
		}

		efforts = append(efforts, BestEffortRecord{
			ID:             uuid.New().String(),
			UserID:         info.UserID,
			ActivityID:     info.ID,
			Category:       result.Category,
			SportGroup:     sportGroup,
			ElapsedTimeS:   ptr(result.ElapsedTimeS),
			ValueNumeric:   ptr(result.ValueNumeric),
			AchievedAt:     info.StartedAt,
			StreamStartIdx: ptr(result.StreamStartIdx),
			StreamEndIdx:   ptr(result.StreamEndIdx),
		})
	}

	if err := s.Store.BatchUpsertBestEffortRecs(ctx, efforts); err != nil {
		return nil, fmt.Errorf("batch upsert best efforts for activity %s: %w", info.ID, err)
	}

	return s.Store.GetBestEffortsForActivityRec(ctx, info.ID, info.UserID)
}

// RecalcAll deletes all best-effort records for a user and recomputes them
// from scratch in chronological order. progressFn is called every 50 activities
// and once at the end with (total, total). progressFn may be nil.
func (s *Service) RecalcAll(ctx context.Context, userID string, progressFn func(done, total int)) error {
	if err := s.Store.DeleteAllBestEfforts(ctx, userID); err != nil {
		return fmt.Errorf("delete all best efforts: %w", err)
	}

	// E-1: single query instead of GetBestEffortRecalcIDs + per-activity
	// GetBestEffortActivityInfo — eliminates the N+1 DB round-trips.
	activities, err := s.Store.GetBestEffortRecalcActivities(ctx, userID)
	if err != nil {
		return fmt.Errorf("get recalc activities: %w", err)
	}

	total := len(activities)

	for i, info := range activities {
		// TODO(E-6): replace GetStreamPoints with GetStreamPointsLightweight once
		// the Compute* function signatures are refactored to accept StreamPointLight
		// (or a generic/interface type). Converting Light→StreamPoint would negate
		// the ~16× memory saving and is therefore not done here.
		stream, err := s.Store.GetStreamPoints(ctx, info.ID)
		if err != nil {
			return fmt.Errorf("get stream points for %s: %w", info.ID, err)
		}

		if _, err := s.ComputeForActivity(ctx, info, stream); err != nil {
			slog.ErrorContext(ctx, "best efforts: compute failed", "activity_id", info.ID, "error", err)
			// Non-fatal: continue with remaining activities.
		}

		// Discard stream slice immediately to avoid accumulating memory.
		stream = nil

		done := i + 1
		if done%50 == 0 {
			if progressFn != nil {
				progressFn(done, total)
			}
		}
	}

	if progressFn != nil {
		progressFn(total, total)
	}

	return nil
}
