package handler

import (
	"context"
	"log/slog"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

const segmentMatchThresholdM = 30.0

// MatchActivitySegments matches an activity against all segments visible to the user.
// It runs as a background goroutine after activity upload (non-blocking).
func MatchActivitySegments(ctx context.Context, db *database.DB, activityID, userID, sportType, subType string, gearID *string) {
	slog.Info("segment matching started", "activity_id", activityID, "sport_type", sportType)

	// Use a fresh context with timeout so we don't depend on the request context
	matchCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Get segments that match the sport type
	segments, err := db.GetMatchableSegments(matchCtx, userID, sportType)
	if err != nil {
		slog.Error("segment matching: get matchable segments failed", "error", err)
		return
	}
	if len(segments) == 0 {
		slog.Debug("segment matching: no matchable segments found", "activity_id", activityID)
		return
	}

	matchCount := 0
	for _, seg := range segments {
		distance, err := db.MatchSegmentToActivity(matchCtx, activityID, seg.VersionID)
		if err != nil {
			slog.Warn("segment matching: hausdorff distance failed",
				"activity_id", activityID, "segment_id", seg.SegmentID, "error", err)
			continue
		}
		if distance == nil || *distance > segmentMatchThresholdM {
			continue
		}

		// Match found - create an effort
		effort := &model.SegmentEffort{
			SegmentID:        seg.SegmentID,
			SegmentVersionID: seg.VersionID,
			ActivityID:       activityID,
			UserID:           userID,
			StartedAt:        time.Now(), // Will be updated with actual track time if available
			ElapsedTimeS:     0,          // Will be updated from activity data
			IsReverse:        false,
			SportType:        sportType,
			SubType:          subType,
			GearID:           gearID,
		}

		// Try to get actual timing from the activity
		if elapsed, startedAt, err := db.GetActivityTimingForSegment(matchCtx, activityID); err == nil {
			effort.StartedAt = startedAt
			effort.ElapsedTimeS = elapsed
		}

		if err := db.InsertSegmentEffort(matchCtx, effort); err != nil {
			slog.Error("segment matching: insert effort failed",
				"activity_id", activityID, "segment_id", seg.SegmentID, "error", err)
			continue
		}

		// Update leaderboard ranks for this segment
		if err := db.UpdateSegmentEffortRanks(matchCtx, seg.SegmentID); err != nil {
			slog.Warn("segment matching: update ranks failed",
				"segment_id", seg.SegmentID, "error", err)
		}

		matchCount++
		slog.Info("segment matched",
			"activity_id", activityID,
			"segment_id", seg.SegmentID,
			"hausdorff_m", *distance)
	}

	slog.Info("segment matching complete",
		"activity_id", activityID,
		"checked", len(segments),
		"matched", matchCount)
}
