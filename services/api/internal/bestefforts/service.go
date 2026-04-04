package bestefforts

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

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
	DeleteAllBestEfforts(ctx context.Context, userID string) error
	GetBestEffortsForActivityRec(ctx context.Context, activityID, userID string) ([]BestEffortRankedRecord, error)
	GetBestEffortRecalcIDs(ctx context.Context, userID string) ([]string, error)
	GetBestEffortActivityInfo(ctx context.Context, activityID string) (*ActivityInfo, error)
	GetStreamPoints(ctx context.Context, activityID string) ([]model.StreamPoint, error)
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

	// Lift indices — empty for now; will be populated once ascent integration is available.
	liftIndices := map[int]bool{}

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
			result = ComputeSuperlative(cat, info.DistanceM, info.ElevationGainM, 0, float64(info.ElapsedTimeS))
		}

		if result == nil {
			continue
		}

		// Map EffortResult (plain values) → BestEffortRecord (pointer fields).
		elapsedTimeS := result.ElapsedTimeS
		valueNumeric := result.ValueNumeric
		startIdx := result.StreamStartIdx
		endIdx := result.StreamEndIdx

		be := BestEffortRecord{
			ID:             uuid.New().String(),
			UserID:         info.UserID,
			ActivityID:     info.ID,
			Category:       result.Category,
			SportGroup:     sportGroup,
			ElapsedTimeS:   &elapsedTimeS,
			ValueNumeric:   &valueNumeric,
			AchievedAt:     info.StartedAt,
			StreamStartIdx: &startIdx,
			StreamEndIdx:   &endIdx,
		}

		if err := s.Store.UpsertBestEffortRec(ctx, be); err != nil {
			return nil, fmt.Errorf("upsert best effort %q for activity %s: %w", cat.Key, info.ID, err)
		}
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

	ids, err := s.Store.GetBestEffortRecalcIDs(ctx, userID)
	if err != nil {
		return fmt.Errorf("get recalc ids: %w", err)
	}

	total := len(ids)

	for i, id := range ids {
		info, err := s.Store.GetBestEffortActivityInfo(ctx, id)
		if err != nil {
			return fmt.Errorf("get activity info for %s: %w", id, err)
		}
		if info == nil {
			// Activity was deleted or is no longer eligible; skip silently.
			continue
		}

		stream, err := s.Store.GetStreamPoints(ctx, id)
		if err != nil {
			return fmt.Errorf("get stream points for %s: %w", id, err)
		}

		if _, err := s.ComputeForActivity(ctx, *info, stream); err != nil {
			slog.ErrorContext(ctx, "best efforts: compute failed", "activity_id", id, "error", err)
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
