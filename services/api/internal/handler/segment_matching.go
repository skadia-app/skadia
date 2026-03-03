package handler

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

const (
	segmentMatchThresholdM     = 30.0  // Hausdorff threshold for sub-track match
	endpointProximityM         = 40.0  // proximity to segment start/end
	subTrackLengthTolerancePct = 0.40  // ±40% length tolerance
	minSegmentEffortTimeS      = 5     // minimum effort duration
)

// MatchActivitySegments matches an activity against all segments visible to the user.
// Uses sub-track extraction: finds traversals through segments by detecting
// entry/exit points near segment start/end, then verifies with Hausdorff distance.
// It runs as a background goroutine after activity upload (non-blocking).
func MatchActivitySegments(ctx context.Context, db *database.DB, activityID, userID, sportType, subType string, gearID *string) {
	slog.Info("segment matching started", "activity_id", activityID, "sport_type", sportType)

	matchCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 1. Spatial pre-filter: only segments within 50m of activity track
	candidates, err := db.GetSpatialCandidateSegments(matchCtx, activityID, userID, sportType)
	if err != nil {
		slog.Error("segment matching: spatial pre-filter failed", "error", err)
		return
	}
	if len(candidates) == 0 {
		slog.Debug("segment matching: no spatial candidates", "activity_id", activityID)
		return
	}

	// 2. Load GPS stream once (shared with lap detection later)
	gpsStream, err := db.GetActivityGPSStream(matchCtx, activityID)
	if err != nil {
		slog.Error("segment matching: get GPS stream failed", "error", err)
		return
	}
	if len(gpsStream) < 5 {
		slog.Debug("segment matching: GPS stream too short", "activity_id", activityID, "points", len(gpsStream))
		return
	}

	matchCount := 0
	for _, cand := range candidates {
		// 3. Get segment endpoints
		endpoints, err := db.GetSegmentEndpoints(matchCtx, cand.VersionID)
		if err != nil || endpoints == nil {
			slog.Warn("segment matching: get endpoints failed",
				"segment_id", cand.SegmentID, "error", err)
			continue
		}

		// 4. Find traversals (forward and reverse)
		traversals := findTraversals(gpsStream, endpoints)
		if len(traversals) == 0 {
			continue
		}

		for _, trav := range traversals {
			subTrack := gpsStream[trav.startIdx : trav.endIdx+1]

			// 5. Length validation
			if !isReasonableLength(subTrack, endpoints.LengthM) {
				continue
			}

			// 6. Hausdorff on sub-track
			distance, err := db.VerifySubTrackMatch(matchCtx, cand.VersionID, subTrack)
			if err != nil {
				slog.Warn("segment matching: verify sub-track failed",
					"segment_id", cand.SegmentID, "error", err)
				continue
			}
			if distance == nil || *distance > segmentMatchThresholdM {
				continue
			}

			// 7. Compute elapsed time
			startTime := subTrack[0].Time
			endTime := subTrack[len(subTrack)-1].Time
			elapsedS := int(endTime.Sub(startTime).Seconds())
			if elapsedS < minSegmentEffortTimeS {
				continue
			}

			// 8. Get stream-based metrics
			var avgSpeed, avgHR, avgPower *float64
			if metrics, err := db.GetSegmentEffortMetrics(matchCtx, activityID, startTime, endTime); err == nil && metrics != nil {
				avgSpeed = metrics.AvgSpeedMs
				avgHR = metrics.AvgHeartRate
				avgPower = metrics.AvgPowerW
			}

			effort := &model.SegmentEffort{
				SegmentID:        cand.SegmentID,
				SegmentVersionID: cand.VersionID,
				ActivityID:       activityID,
				UserID:           userID,
				StartedAt:        startTime,
				ElapsedTimeS:     elapsedS,
				IsReverse:        trav.isReverse,
				SportType:        sportType,
				SubType:          subType,
				GearID:           gearID,
				AvgSpeedMs:       avgSpeed,
				AvgHeartRate:     avgHR,
				AvgPowerW:        avgPower,
			}

			if err := db.InsertSegmentEffort(matchCtx, effort); err != nil {
				slog.Error("segment matching: insert effort failed",
					"activity_id", activityID, "segment_id", cand.SegmentID, "error", err)
				continue
			}

			if err := db.UpdateSegmentEffortRanks(matchCtx, cand.SegmentID); err != nil {
				slog.Warn("segment matching: update ranks failed",
					"segment_id", cand.SegmentID, "error", err)
			}

			matchCount++
			slog.Info("segment matched",
				"activity_id", activityID,
				"segment_id", cand.SegmentID,
				"hausdorff_m", *distance,
				"elapsed_s", elapsedS,
				"is_reverse", trav.isReverse)
		}
	}

	slog.Info("segment matching complete",
		"activity_id", activityID,
		"candidates", len(candidates),
		"matched", matchCount)
}

// traversal represents a detected pass through a segment.
type traversal struct {
	startIdx  int
	endIdx    int
	isReverse bool
}

// findTraversals finds all traversals through a segment by detecting entry/exit
// points near the segment's start and end coordinates.
func findTraversals(stream []database.GPSStreamPoint, ep *database.SegmentEndpoints) []traversal {
	// Find all stream indices near segment start and end
	nearStart := findProximityIndices(stream, ep.StartLat, ep.StartLng, endpointProximityM)
	nearEnd := findProximityIndices(stream, ep.EndLat, ep.EndLng, endpointProximityM)

	if len(nearStart) == 0 || len(nearEnd) == 0 {
		return nil
	}

	// Cluster consecutive indices
	startClusters := clusterConsecutive(nearStart, 5)
	endClusters := clusterConsecutive(nearEnd, 5)

	var result []traversal

	// Forward traversals: enter near start, exit near end
	for _, sc := range startClusters {
		entryIdx := sc[len(sc)-1] // last point in start cluster (leaving start area)
		for _, ec := range endClusters {
			exitIdx := ec[0] // first point in end cluster (arriving at end area)
			if exitIdx > entryIdx && (exitIdx-entryIdx) >= 3 {
				result = append(result, traversal{
					startIdx:  entryIdx,
					endIdx:    exitIdx,
					isReverse: false,
				})
				break // only first matching end cluster per start
			}
		}
	}

	// Reverse traversals: enter near end, exit near start
	for _, ec := range endClusters {
		entryIdx := ec[len(ec)-1]
		for _, sc := range startClusters {
			exitIdx := sc[0]
			if exitIdx > entryIdx && (exitIdx-entryIdx) >= 3 {
				result = append(result, traversal{
					startIdx:  entryIdx,
					endIdx:    exitIdx,
					isReverse: true,
				})
				break
			}
		}
	}

	return result
}

// findProximityIndices returns all stream indices where the point is within
// thresholdM meters of the target coordinate.
func findProximityIndices(stream []database.GPSStreamPoint, lat, lng, thresholdM float64) []int {
	var result []int
	for i, p := range stream {
		if haversineM(p.Lat, p.Lng, lat, lng) <= thresholdM {
			result = append(result, i)
		}
	}
	return result
}

// clusterConsecutive groups consecutive indices with a maximum gap.
func clusterConsecutive(indices []int, maxGap int) [][]int {
	if len(indices) == 0 {
		return nil
	}

	var clusters [][]int
	current := []int{indices[0]}
	for i := 1; i < len(indices); i++ {
		if indices[i]-indices[i-1] <= maxGap {
			current = append(current, indices[i])
		} else {
			clusters = append(clusters, current)
			current = []int{indices[i]}
		}
	}
	clusters = append(clusters, current)
	return clusters
}

// isReasonableLength checks if a sub-track's estimated length is within tolerance
// of the segment's known length.
func isReasonableLength(subTrack []database.GPSStreamPoint, segmentLengthM float64) bool {
	if segmentLengthM <= 0 {
		return true // unknown segment length, skip check
	}

	subTrackLength := estimateSubTrackLength(subTrack)
	minLen := segmentLengthM * (1.0 - subTrackLengthTolerancePct)
	maxLen := segmentLengthM * (1.0 + subTrackLengthTolerancePct)
	return subTrackLength >= minLen && subTrackLength <= maxLen
}

// estimateSubTrackLength computes the total haversine distance of a sub-track.
func estimateSubTrackLength(stream []database.GPSStreamPoint) float64 {
	if len(stream) < 2 {
		return 0
	}
	total := 0.0
	for i := 1; i < len(stream); i++ {
		total += haversineM(stream[i-1].Lat, stream[i-1].Lng, stream[i].Lat, stream[i].Lng)
	}
	return total
}

// haversineM returns distance in meters between two lat/lng points.
func haversineM(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusM = 6371000.0
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLng := (lng2 - lng1) * math.Pi / 180.0
	lat1Rad := lat1 * math.Pi / 180.0
	lat2Rad := lat2 * math.Pi / 180.0
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusM * c
}
