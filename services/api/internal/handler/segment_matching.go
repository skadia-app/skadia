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
		// Lap segments: use crossing detection instead of sub-track matching
		if cand.SegmentType == "lap" {
			count := matchLapSegment(matchCtx, db, activityID, userID, sportType, subType, gearID, &cand, gpsStream)
			matchCount += count
			continue
		}

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

	// Forward traversals: enter near start, exit near end.
	// Use the cluster point nearest to the actual segment endpoint so the sub-track
	// starts/ends as close as possible to the segment geometry, minimising Hausdorff error.
	for _, sc := range startClusters {
		entryIdx := nearestInCluster(stream, sc, ep.StartLat, ep.StartLng)
		for _, ec := range endClusters {
			exitIdx := nearestInCluster(stream, ec, ep.EndLat, ep.EndLng)
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

	// Reverse traversals: enter near end, exit near start.
	for _, ec := range endClusters {
		entryIdx := nearestInCluster(stream, ec, ep.EndLat, ep.EndLng)
		for _, sc := range startClusters {
			exitIdx := nearestInCluster(stream, sc, ep.StartLat, ep.StartLng)
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

// nearestInCluster returns the stream index within cluster that is closest to (lat, lng).
func nearestInCluster(stream []database.GPSStreamPoint, cluster []int, lat, lng float64) int {
	best := cluster[0]
	bestDist := haversineM(stream[cluster[0]].Lat, stream[cluster[0]].Lng, lat, lng)
	for _, idx := range cluster[1:] {
		d := haversineM(stream[idx].Lat, stream[idx].Lng, lat, lng)
		if d < bestDist {
			best = idx
			bestDist = d
		}
	}
	return best
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

const lapFinishProximityM = 30.0
const lapMinGapM = 100.0 // minimum GPS distance between two crossings

// matchLapSegment detects each individual lap traversal for a closed-loop segment.
// It finds all GPS points within lapFinishProximityM of the finish_point stored in
// segment_type_meta, groups them into crossing events separated by lapMinGapM,
// and creates one segment_effort per crossing pair.
func matchLapSegment(
	ctx context.Context,
	db *database.DB,
	activityID, userID, sportType, subType string,
	gearID *string,
	cand *database.CandidateSegment,
	gpsStream []database.GPSStreamPoint,
) int {
	// Extract finish_point from segment_type_meta
	finishLat, finishLng, ok := extractFinishPoint(cand.TypeMeta)
	if !ok {
		slog.Warn("lap segment missing finish_point in type_meta", "segment_id", cand.SegmentID)
		return 0
	}

	// Find all indices near finish point
	nearFinish := findProximityIndices(gpsStream, finishLat, finishLng, lapFinishProximityM)
	if len(nearFinish) < 2 {
		return 0
	}

	// Group into crossing events (separated by at least lapMinGapM of GPS distance)
	crossings := groupLapCrossings(gpsStream, nearFinish, lapMinGapM)
	if len(crossings) < 2 {
		return 0
	}

	count := 0
	for i := 1; i < len(crossings); i++ {
		startIdx := crossings[i-1]
		endIdx := crossings[i]
		subTrack := gpsStream[startIdx : endIdx+1]

		startTime := subTrack[0].Time
		endTime := subTrack[len(subTrack)-1].Time
		elapsedS := int(endTime.Sub(startTime).Seconds())
		if elapsedS < minSegmentEffortTimeS {
			continue
		}

		var avgSpeed, avgHR, avgPower *float64
		if metrics, err := db.GetSegmentEffortMetrics(ctx, activityID, startTime, endTime); err == nil && metrics != nil {
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
			IsReverse:        false, // laps have no direction concept
			SportType:        sportType,
			SubType:          subType,
			GearID:           gearID,
			AvgSpeedMs:       avgSpeed,
			AvgHeartRate:     avgHR,
			AvgPowerW:        avgPower,
		}
		if err := db.InsertSegmentEffort(ctx, effort); err != nil {
			slog.Error("lap matching: insert effort failed", "error", err)
			continue
		}
		count++
	}

	if count > 0 {
		_ = db.UpdateSegmentEffortRanks(ctx, cand.SegmentID)
		slog.Info("lap segment matched", "activity_id", activityID,
			"segment_id", cand.SegmentID, "laps", count)
	}
	return count
}

// extractFinishPoint reads finish_lat and finish_lng from segment_type_meta.
func extractFinishPoint(meta map[string]interface{}) (lat, lng float64, ok bool) {
	if meta == nil {
		return 0, 0, false
	}
	latV, hasLat := meta["finish_lat"]
	lngV, hasLng := meta["finish_lng"]
	if !hasLat || !hasLng {
		return 0, 0, false
	}
	lat, ok1 := latV.(float64)
	lng, ok2 := lngV.(float64)
	return lat, lng, ok1 && ok2
}

// groupLapCrossings takes all indices near the finish point and returns
// one representative index per crossing event, separated by minGapM of travel.
func groupLapCrossings(stream []database.GPSStreamPoint, indices []int, minGapM float64) []int {
	if len(indices) == 0 {
		return nil
	}
	var crossings []int
	crossings = append(crossings, indices[0])

	for _, idx := range indices[1:] {
		last := crossings[len(crossings)-1]
		dist := estimateSubTrackLength(stream[last : idx+1])
		if dist >= minGapM {
			crossings = append(crossings, idx)
		}
	}
	return crossings
}
