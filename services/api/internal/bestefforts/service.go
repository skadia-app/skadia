package bestefforts

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// hashString returns a stable uint32 hash of s using FNV-1a, for advisory lock keys.
func hashString(s string) uint32 {
	var h uint32 = 2166136261
	for i := range len(s) {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// bestEffortLockBase is the domain prefix for advisory lock keys in the best-efforts domain.
// "BEST" = 0x42455354 as big-endian int32.
const bestEffortLockBase = int64(0x42455354)

// BestEffortLockKey returns a stable per-user int64 advisory lock key.
// Exported so that database adapters can share the same derivation logic.
func BestEffortLockKey(userID string) int64 {
	return bestEffortLockBase ^ int64(hashString(userID))
}

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
	// DeleteOrphanedBestEfforts removes best_efforts rows whose activity_id is not
	// in keepActivityIDs. Used by RecalcAll to purge stale rows after a full upsert
	// pass without ever leaving the table empty.
	DeleteOrphanedBestEfforts(ctx context.Context, userID string, keepActivityIDs []string) error
	// AcquireBestEffortLock acquires a session-level advisory lock on a pinned
	// connection. Blocks until the lock is obtained.
	// The caller MUST call conn.Release() when done (typically via defer).
	AcquireBestEffortLock(ctx context.Context, userID string) (*pgxpool.Conn, error)
	// TryAcquireBestEffortLock is a non-blocking variant. Returns (conn, true, nil)
	// when the lock was obtained, (nil, false, nil) when it is already held by
	// another session. The caller MUST call conn.Release() when acquired.
	TryAcquireBestEffortLock(ctx context.Context, userID string) (*pgxpool.Conn, bool, error)
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

// computeForActivityInner performs the actual computation and upsert for a
// single activity. It does NOT acquire any advisory lock — the caller is
// responsible for serialisation.
func computeForActivityInner[T model.BestEffortPoint](ctx context.Context, store Store, info ActivityInfo, stream []T) ([]BestEffortRankedRecord, error) {
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
			ElapsedTimeS:   new(result.ElapsedTimeS),
			ValueNumeric:   new(result.ValueNumeric),
			AchievedAt:     info.StartedAt,
			StreamStartIdx: new(result.StreamStartIdx),
			StreamEndIdx:   new(result.StreamEndIdx),
		})
	}

	if err := store.BatchUpsertBestEffortRecs(ctx, efforts); err != nil {
		return nil, fmt.Errorf("batch upsert best efforts for activity %s: %w", info.ID, err)
	}

	return store.GetBestEffortsForActivityRec(ctx, info.ID, info.UserID)
}

// ComputeForActivity computes all best-effort records for the given activity
// and upserts them into persistent storage. It returns the ranked results.
//
// Returns nil, nil when the activity's sport has no best-efforts semantics.
//
// This method uses a non-blocking advisory lock. If RecalcAll currently holds
// the lock for this user, the call is a no-op (returns nil, nil) — RecalcAll
// will cover this activity in its own full pass.
func (s *Service) ComputeForActivity(ctx context.Context, info ActivityInfo, stream []model.StreamPoint) ([]BestEffortRankedRecord, error) {
	conn, acquired, err := s.Store.TryAcquireBestEffortLock(ctx, info.UserID)
	if err != nil {
		return nil, fmt.Errorf("try acquire best effort lock: %w", err)
	}
	if !acquired {
		// RecalcAll holds the lock — skip; it will recompute this activity.
		slog.DebugContext(ctx, "best efforts: skipping single-activity compute, RecalcAll running",
			"activity_id", info.ID, "user_id", info.UserID)
		return nil, nil
	}
	defer conn.Release()

	return computeForActivityInner(ctx, s.Store, info, stream)
}

// RecalcAll recomputes all best-effort records for a user atomically.
//
// Instead of deleting everything first (which leaves an empty table on crash),
// it upserts each activity's efforts and then removes orphaned rows for
// activities that are no longer eligible. A blocking advisory lock prevents a
// concurrent ComputeForActivity call from racing with the bulk recompute.
//
// progressFn is called every 50 activities and once at the end with (total, total).
// progressFn may be nil.
func (s *Service) RecalcAll(ctx context.Context, userID string, progressFn func(done, total int)) error {
	// Acquire per-user advisory lock on a pinned connection — blocks until no
	// other RecalcAll or ComputeForActivity is running for this user.
	// conn.Release() returns the connection to the pool, which ends the session
	// and automatically releases all session-scoped advisory locks.
	conn, err := s.Store.AcquireBestEffortLock(ctx, userID)
	if err != nil {
		return fmt.Errorf("acquire best effort lock: %w", err)
	}
	defer conn.Release()

	// E-1: single query instead of GetBestEffortRecalcIDs + per-activity
	// GetBestEffortActivityInfo — eliminates the N+1 DB round-trips.
	activities, err := s.Store.GetBestEffortRecalcActivities(ctx, userID)
	if err != nil {
		return fmt.Errorf("get recalc activities: %w", err)
	}

	total := len(activities)
	processedIDs := make([]string, 0, total)

	// Report total immediately so the UI can show progress (e.g. "0 / 442")
	if progressFn != nil {
		progressFn(0, total)
	}

	for i, info := range activities {
		// Use lightweight stream (5 columns) — ~16× less data than full StreamPoint.
		stream, err := s.Store.GetStreamPointsLightweight(ctx, info.ID)
		if err != nil {
			return fmt.Errorf("get stream points for %s: %w", info.ID, err)
		}

		// Call the inner function directly — we already hold the advisory lock.
		if _, err := computeForActivityInner(ctx, s.Store, info, stream); err != nil {
			slog.ErrorContext(ctx, "best efforts: compute failed", "activity_id", info.ID, "error", err)
			// Non-fatal: continue with remaining activities.
		}

		processedIDs = append(processedIDs, info.ID)

		// Discard stream slice immediately to avoid accumulating memory.
		stream = nil

		done := i + 1
		if done%50 == 0 {
			if progressFn != nil {
				progressFn(done, total)
			}
		}
	}

	// Remove stale rows for activities that are no longer eligible (deleted,
	// excluded, etc.). Safe here because we hold the advisory lock and all
	// current eligible activities have already been upserted above.
	if err := s.Store.DeleteOrphanedBestEfforts(ctx, userID, processedIDs); err != nil {
		return fmt.Errorf("delete orphaned best efforts: %w", err)
	}

	if progressFn != nil {
		progressFn(total, total)
	}

	return nil
}
