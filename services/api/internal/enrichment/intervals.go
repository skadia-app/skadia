package enrichment

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

const (
	intervalMinActivityS    = 600  // minimum activity duration in seconds (10 min)
	intervalMinSegmentS     = 30   // minimum segment duration in seconds
	intervalMaxDepth        = 10   // maximum recursion depth for binary split
	intervalHardRatio       = 1.20 // was 1.10 — raised to reduce false positives
	intervalModerateRatio   = 0.90
	intervalEasyRatio       = 0.70
	intervalSmoothWindow    = 30 // was 10 — HR needs 30s smoothing (HR-lag)
	intervalMinHardSegments = 3  // was 2 (implicit) — 2 hills != interval training
)

// workoutSplitTypes are FIT split types that indicate structured interval training.
// See: vendor/github.com/muktihari/fit/profile/typedef/split_type_gen.go
var workoutSplitTypes = map[string]bool{
	"interval_active":   true,
	"interval_rest":     true,
	"interval_warmup":   true,
	"interval_cooldown": true,
	"interval_recovery": true,
	"interval_other":    true,
	"workout_round":     true,
}

// intervalEligibleGroups are segment groups where interval detection is allowed
// by default (whitelist, not blacklist).
var intervalEligibleGroups = map[string]bool{
	"bike": true,
	"swim": true,
}

// intervalEligibleFoot lists the specific foot-group sports that are eligible.
// Running yes, hiking no.
var intervalEligibleFoot = map[string]bool{
	"running":       true,
	"trail_running": true,
}

// intervalEligibleExtra lists sports eligible regardless of their segment group.
var intervalEligibleExtra = map[string]bool{
	"cross_country_skiing": true,
	"inline_skating":       true,
	"rowing":               true,
	"fitness_equipment":    true,
	"hiit":                 true,
	"training":             true,
}

// splitTypeToIntensity maps a FIT split_type to an intensity classification.
// Returns "" for non-workout split types.
func splitTypeToIntensity(splitType string) string {
	switch splitType {
	case "interval_active":
		return "hard"
	case "interval_rest", "interval_recovery":
		return "recovery"
	case "interval_warmup", "interval_cooldown":
		return "easy"
	case "interval_other", "workout_round":
		return "moderate"
	default:
		return ""
	}
}

// splitTypeKeys returns the workout split types as a string slice for SQL ANY().
func splitTypeKeys() []string {
	keys := make([]string, 0, len(workoutSplitTypes))
	for k := range workoutSplitTypes {
		keys = append(keys, k)
	}
	return keys
}

// extractIntervalsFromSplits reads activity_splits for structured workout data.
// Returns the number of intervals inserted, or 0 if no workout splits found.
// This is Tier-1 detection: zero false positives, only works for FIT files
// with explicit structured workout data from the device.
func extractIntervalsFromSplits(ctx context.Context, db *database.DB, activityID, userID string) (int, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT split_type, start_time, end_time, total_elapsed_time_s,
		       total_distance_m, avg_speed_ms, total_ascent_m, total_descent_m
		FROM activity_splits
		WHERE activity_id = $1
		  AND split_type = ANY($2)
		ORDER BY message_index
	`, activityID, splitTypeKeys())
	if err != nil {
		return 0, fmt.Errorf("intervals: query splits: %w", err)
	}
	defer rows.Close()

	type splitRow struct {
		SplitType    string
		StartTime    time.Time
		EndTime      time.Time
		ElapsedTimeS *float64
		DistanceM    *float64
		AvgSpeedMS   *float64
		AscentM      *int
		DescentM     *int
	}

	var splits []splitRow
	for rows.Next() {
		var s splitRow
		if err := rows.Scan(&s.SplitType, &s.StartTime, &s.EndTime,
			&s.ElapsedTimeS, &s.DistanceM, &s.AvgSpeedMS,
			&s.AscentM, &s.DescentM); err != nil {
			return 0, fmt.Errorf("intervals: scan split: %w", err)
		}
		splits = append(splits, s)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("intervals: rows: %w", err)
	}

	// Need at least 2 splits with at least 1 interval_active
	if len(splits) < 2 {
		return 0, nil
	}
	hasActive := false
	for _, s := range splits {
		if s.SplitType == "interval_active" {
			hasActive = true
			break
		}
	}
	if !hasActive {
		return 0, nil
	}

	// Delete existing intervals + insert from splits — wrapped in a transaction
	// so a partial failure cannot leave the table in an inconsistent state.
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("intervals: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx,
		"DELETE FROM activity_intervals WHERE activity_id = $1",
		activityID); err != nil {
		return 0, fmt.Errorf("intervals: delete existing: %w", err)
	}

	count := 0
	for i, s := range splits {
		intensity := splitTypeToIntensity(s.SplitType)
		if intensity == "" {
			continue
		}

		durationS := int(s.EndTime.Sub(s.StartTime).Seconds())
		if s.ElapsedTimeS != nil && *s.ElapsedTimeS > 0 {
			durationS = int(*s.ElapsedTimeS)
		}
		if durationS < 1 {
			durationS = 1
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_intervals
			    (activity_id, segment_num, start_time, end_time,
			     duration_s, avg_speed_ms, intensity)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, activityID, i+1, s.StartTime, s.EndTime,
			durationS, s.AvgSpeedMS, intensity); err != nil {
			slog.Warn("intervals: insert split-interval failed",
				"activity_id", activityID, "segment", i+1, "error", err)
			continue
		}
		count++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("intervals: commit tx: %w", err)
	}

	// Auto-tag "Intervalltraining"
	if count > 0 {
		if err := upsertIntervalTag(ctx, db, activityID, userID); err != nil {
			slog.Warn("intervals: auto-tag from splits failed",
				"activity_id", activityID, "error", err)
		}
	}

	slog.Info("intervals: extracted from FIT splits",
		"activity_id", activityID, "intervals", count)
	return count, nil
}

// intervalStream holds a single row from activity_streams.
type intervalStream struct {
	T         time.Time
	PowerW    *float64
	HeartRate *int
	SpeedMS   *float64
	Cadence   *float64
}

// intervalSegment is a contiguous slice of the stream.
type intervalSegment struct {
	startIdx int
	endIdx   int
	startT   time.Time
	endT     time.Time
}

// classifiedSegment is an intervalSegment with aggregated stats.
type classifiedSegment struct {
	startT    time.Time
	endT      time.Time
	durationS int
	avgPower  *float64
	avgHR     *int
	avgSpeed  *float64
	avgCad    *float64
	intensity string
}

// enrichStreamIntervals is the entry point for interval detection.
// It runs a two-tier strategy:
//
//  1. Tier-1 (FIT Splits): structured workout data from the device — zero false
//     positives; only available for FIT files with explicit workout records.
//  2. Tier-2 (Heuristic): binary segmentation on power/HR/speed signal — only
//     fires when Tier-1 found nothing; gated by sport type, hard-segment count
//     and confidence score.
func enrichStreamIntervals(ctx context.Context, db *database.DB, userID, activityID string, stream []EnrichmentStreamPoint) error {
	// Sport gate: whitelist approach — only eligible sports get interval detection.
	var sportType string
	_ = db.Pool.QueryRow(ctx, "SELECT sport_type FROM activities WHERE id = $1", activityID).Scan(&sportType)

	eligible := intervalEligibleExtra[sportType]
	if !eligible {
		if def, ok := sport.GetSportDef(sportType); ok {
			if def.SegmentGroup == "foot" {
				eligible = intervalEligibleFoot[sportType]
			} else {
				eligible = intervalEligibleGroups[def.SegmentGroup]
			}
		}
	}
	if !eligible {
		retractIntervalTag(ctx, db, activityID, userID)
		return nil
	}

	// Tier 1: FIT Splits (zero false positives).
	inserted, err := extractIntervalsFromSplits(ctx, db, activityID, userID)
	if err != nil {
		slog.Warn("intervals: splits extraction failed, falling through to heuristic",
			"activity_id", activityID, "error", err)
	}
	if inserted > 0 {
		return nil // Tier-1 found intervals, done
	}

	// Tier 2: Heuristic (only for GPX/TCX or FIT without structured workouts).
	return enrichStreamIntervalsHeuristic(ctx, db, userID, activityID, stream)
}

// enrichStreamIntervalsHeuristic is the Tier-2 binary-segmentation detector.
// It mirrors services/ml/enrichment/intervals.py but with additional gates:
//   - minimum intervalMinHardSegments (3) hard phases required
//   - confidence score >= 0.7 required before tagging
//   - retraction: removes auto-tag + intervals when evidence is insufficient
func enrichStreamIntervalsHeuristic(ctx context.Context, db *database.DB, userID, activityID string, stream []EnrichmentStreamPoint) error {
	// 1. Map shared stream — all points included, nulls handled per-field in extractSignal.
	pts := make([]intervalStream, len(stream))
	for i, sp := range stream {
		pts[i] = intervalStream{
			T:         sp.Time,
			PowerW:    sp.PowerW,
			HeartRate: sp.HeartRate,
			SpeedMS:   sp.SpeedMs,
			Cadence:   sp.Cadence,
		}
	}

	if len(pts) < 2 {
		return nil
	}
	durationS := pts[len(pts)-1].T.Sub(pts[0].T).Seconds()
	if durationS < intervalMinActivityS {
		return nil
	}

	// 2. Extract + smooth primary signal.
	signal := extractSignal(pts)
	if signal == nil {
		return nil
	}

	// 3. Binary segmentation.
	times := make([]time.Time, len(pts))
	for i, p := range pts {
		times[i] = p.T
	}
	var segs []intervalSegment
	binarySplit(signal, times, 0, len(signal)-1, 0, &segs)
	sortSegments(segs)

	if len(segs) == 0 {
		return nil
	}

	// 4. Classify segments.
	classified := classifySegments(segs, signal, pts)

	hardCount := 0
	for _, s := range classified {
		if s.intensity == "hard" {
			hardCount++
		}
	}
	if hardCount < intervalMinHardSegments {
		retractIntervalTag(ctx, db, activityID, userID)
		return nil
	}

	// 5. Confidence check — only tag at confidence >= 0.7.
	confidence := computeIntervalConfidence(classified)
	if confidence < 0.7 {
		slog.Debug("intervals: rejected — confidence too low",
			"activity_id", activityID, "confidence", confidence, "hard_segments", hardCount)
		retractIntervalTag(ctx, db, activityID, userID)
		return nil
	}

	// 6. Persist results.
	if _, err := db.Pool.Exec(ctx,
		"DELETE FROM activity_intervals WHERE activity_id = $1",
		activityID,
	); err != nil {
		return fmt.Errorf("intervals: delete existing: %w", err)
	}

	for i, seg := range classified {
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO activity_intervals
			    (activity_id, segment_num, start_time, end_time,
			     duration_s, avg_power_w, avg_heart_rate,
			     avg_speed_ms, avg_cadence, intensity)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		`, activityID, i+1,
			seg.startT, seg.endT, seg.durationS,
			seg.avgPower, seg.avgHR,
			seg.avgSpeed, seg.avgCad,
			seg.intensity,
		); err != nil {
			slog.Warn("intervals: insert segment failed",
				"activity_id", activityID,
				"segment_num", i+1,
				"error", err,
			)
		}
	}

	// 7. Auto-tag "Intervalltraining".
	if err := upsertIntervalTag(ctx, db, activityID, userID); err != nil {
		slog.Warn("intervals: auto-tag failed",
			"activity_id", activityID, "error", err)
	}

	slog.Info("intervals: heuristic enrichment complete",
		"activity_id", activityID,
		"segments", len(classified),
		"hard", hardCount,
		"confidence", confidence,
	)
	return nil
}

// computeIntervalConfidence returns a confidence score for detected intervals.
// Based on: number of work phases, duration regularity (CV), intensity
// regularity, and work/rest alternation pattern.
//
// Score thresholds (from spec):
//
//	3 work phases, CV < 0.40              → 0.5
//	4+ work phases, CV < 0.30            → 0.7
//	5+ work phases, CV < 0.25, strict alt → 0.9
//	FIT Splits (Tier-1)                   → 1.0
//
// Tag "Intervalltraining" only at confidence >= 0.7.
func computeIntervalConfidence(classified []classifiedSegment) float64 {
	// Collect work phase stats — filter out implausible durations.
	var workDurations []float64
	var workIntensities []float64
	for _, s := range classified {
		if s.intensity == "hard" && s.durationS >= 20 && s.durationS <= 1800 {
			workDurations = append(workDurations, float64(s.durationS))
			// Use mean signal value as intensity proxy.
			if s.avgPower != nil {
				workIntensities = append(workIntensities, *s.avgPower)
			} else if s.avgHR != nil {
				workIntensities = append(workIntensities, float64(*s.avgHR))
			} else if s.avgSpeed != nil {
				workIntensities = append(workIntensities, *s.avgSpeed)
			}
		}
	}

	workCount := len(workDurations)
	if workCount < 3 {
		return 0.0 // not enough work phases
	}

	// Duration CV (Coefficient of Variation = stddev / mean).
	durationMean := meanSlice(workDurations)
	durationCV := 1.0
	if durationMean > 0 {
		durationCV = stddev(workDurations) / durationMean
	}
	if durationCV >= 0.40 {
		return 0.0 // too irregular
	}

	// Intensity CV (optional — only if we have intensity data).
	// Default 1.0 (= "irregular") when no data → prevents false 0.9 score.
	intensityCV := 1.0
	if len(workIntensities) >= 3 {
		intMean := meanSlice(workIntensities)
		if intMean > 0 {
			intensityCV = stddev(workIntensities) / intMean
		}
	}

	// Work/rest alternation check.
	alternationCount := 0
	for i := 1; i < len(classified); i++ {
		prevHard := classified[i-1].intensity == "hard"
		currHard := classified[i].intensity == "hard"
		if prevHard != currHard {
			alternationCount++
		}
	}
	alternationRatio := 0.0
	if len(classified) > 1 {
		alternationRatio = float64(alternationCount) / float64(len(classified)-1)
	}

	// Score calculation.
	switch {
	case workCount >= 5 && durationCV < 0.25 && alternationRatio >= 0.60 && intensityCV < 0.25:
		return 0.9
	case workCount >= 4 && durationCV < 0.30:
		return 0.7
	default:
		return 0.5
	}
}

// retractIntervalTag removes the auto-assigned "Intervalltraining" tag and
// deletes any existing activity_intervals rows. Called when the algorithm
// determines an activity does NOT have structured intervals.
// Manual tags assigned by the user are protected and never removed.
func retractIntervalTag(ctx context.Context, db *database.DB, activityID, userID string) {
	// Delete interval rows.
	db.Pool.Exec(ctx, "DELETE FROM activity_intervals WHERE activity_id = $1", activityID) //nolint:errcheck

	// Find the auto-assigned tag entry.
	var tagID string
	err := db.Pool.QueryRow(ctx, `
		SELECT t.id FROM tags t
		JOIN activity_tags at ON at.tag_id = t.id
		WHERE at.activity_id = $1 AND t.user_id = $2
		  AND t.name = 'Intervalltraining' AND at.assigned_by = 'auto'
	`, activityID, userID).Scan(&tagID)
	if err != nil {
		return // no auto-tag exists — nothing to retract
	}

	db.Pool.Exec(ctx, //nolint:errcheck
		"DELETE FROM activity_tags WHERE activity_id = $1 AND tag_id = $2",
		activityID, tagID)

	// Only update the denormalised tags array when no manual tag remains.
	// array_remove removes ALL occurrences, which would also wipe a manually
	// assigned "Intervalltraining" tag → check first.
	var hasManualTag bool
	_ = db.Pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM activity_tags at
			JOIN tags t ON at.tag_id = t.id
			WHERE at.activity_id = $1 AND t.name = 'Intervalltraining'
			  AND at.assigned_by != 'auto'
		)`, activityID).Scan(&hasManualTag)

	if !hasManualTag {
		db.Pool.Exec(ctx, //nolint:errcheck
			`UPDATE activities SET tags = array_remove(tags, 'Intervalltraining'), updated_at = NOW()
			 WHERE id = $1`, activityID)
	}
}

// upsertIntervalTag inserts (or ignores) the "Intervalltraining" auto-tag and
// assigns it to the activity.
func upsertIntervalTag(ctx context.Context, db *database.DB, activityID, userID string) error {
	var tagID string
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, is_auto, category)
		VALUES ($1, 'Intervalltraining', '#EF4444', '#FFFFFF', true, 'activity')
		ON CONFLICT (user_id, name) DO UPDATE SET name = tags.name
		RETURNING id
	`, userID).Scan(&tagID); err != nil {
		return fmt.Errorf("upsert tag: %w", err)
	}

	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
		VALUES ($1, $2, 'auto')
		ON CONFLICT (activity_id, tag_id) DO NOTHING
	`, activityID, tagID); err != nil {
		return fmt.Errorf("assign tag: %w", err)
	}

	if _, err := db.Pool.Exec(ctx, `
		UPDATE activities
		SET tags = array_append(tags, 'Intervalltraining'),
		    updated_at = NOW()
		WHERE id = $1
		  AND NOT ('Intervalltraining' = ANY(COALESCE(tags, '{}')))
	`, activityID); err != nil {
		return fmt.Errorf("update activities.tags: %w", err)
	}

	return nil
}

// forwardBackfill fills null gaps in raw using forward-fill then backfill.
// isValid[i] indicates whether raw[i] was a real measurement.
func forwardBackfill(raw []float64, isValid []bool) {
	// forward fill
	last := -1
	for i := range raw {
		if isValid[i] {
			last = i
		} else if last >= 0 {
			raw[i] = raw[last]
		}
	}
	// backfill leading nulls
	first := -1
	for i := range raw {
		if isValid[i] {
			first = i
			break
		}
	}
	if first > 0 {
		for i := range first {
			raw[i] = raw[first]
		}
	}
}

// extractSignal picks the best signal channel (power > HR > speed) and applies
// forward-backfill for nulls then rolling-mean smoothing.
// Returns nil if no channel has >50 % non-null coverage.
func extractSignal(pts []intervalStream) []float64 {
	n := len(pts)

	// Power
	{
		cnt := 0
		for _, p := range pts {
			if p.PowerW != nil {
				cnt++
			}
		}
		if cnt > n/2 {
			raw := make([]float64, n)
			valid := make([]bool, n)
			for i, p := range pts {
				if p.PowerW != nil {
					raw[i] = *p.PowerW
					valid[i] = true
				}
			}
			forwardBackfill(raw, valid)
			return rollingMean(raw, intervalSmoothWindow)
		}
	}

	// Heart rate
	{
		cnt := 0
		for _, p := range pts {
			if p.HeartRate != nil {
				cnt++
			}
		}
		if cnt > n/2 {
			raw := make([]float64, n)
			valid := make([]bool, n)
			for i, p := range pts {
				if p.HeartRate != nil {
					raw[i] = float64(*p.HeartRate)
					valid[i] = true
				}
			}
			forwardBackfill(raw, valid)
			return rollingMean(raw, intervalSmoothWindow)
		}
	}

	// Speed
	{
		cnt := 0
		for _, p := range pts {
			if p.SpeedMS != nil {
				cnt++
			}
		}
		if cnt > n/2 {
			raw := make([]float64, n)
			valid := make([]bool, n)
			for i, p := range pts {
				if p.SpeedMS != nil {
					raw[i] = *p.SpeedMS
					valid[i] = true
				}
			}
			forwardBackfill(raw, valid)
			return rollingMean(raw, intervalSmoothWindow)
		}
	}

	return nil
}

// rollingMean applies numpy.convolve(arr, ones(w)/w, mode="same").
// For "same" mode the output length equals the input length; near the edges the
// window is implicitly clipped (zero-padded on both sides like numpy).
func rollingMean(arr []float64, window int) []float64 {
	n := len(arr)
	out := make([]float64, n)
	half := window / 2
	for i := range arr {
		sum := 0.0
		cnt := 0
		for j := i - half; j < i-half+window; j++ {
			if j >= 0 && j < n {
				sum += arr[j]
				cnt++
			}
		}
		if cnt > 0 {
			out[i] = sum / float64(cnt)
		}
	}
	return out
}

// variance computes the population variance of a slice.
func variance(s []float64) float64 {
	if len(s) == 0 {
		return 0
	}
	mean := 0.0
	for _, v := range s {
		mean += v
	}
	mean /= float64(len(s))
	vr := 0.0
	for _, v := range s {
		d := v - mean
		vr += d * d
	}
	return vr / float64(len(s))
}

// stddev returns the population standard deviation.
func stddev(s []float64) float64 {
	return math.Sqrt(variance(s))
}

// meanSlice returns the arithmetic mean of a slice (0 for empty).
func meanSlice(s []float64) float64 {
	if len(s) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range s {
		sum += v
	}
	return sum / float64(len(s))
}

// binarySplit recursively splits [left, right] at the index that maximises
// variance reduction, mirroring _binary_split in intervals.py.
func binarySplit(signal []float64, times []time.Time, left, right, depth int, out *[]intervalSegment) {
	segDur := times[right].Sub(times[left]).Seconds()
	if segDur < intervalMinSegmentS || depth > intervalMaxDepth {
		*out = append(*out, intervalSegment{
			startIdx: left, endIdx: right,
			startT: times[left], endT: times[right],
		})
		return
	}

	sub := signal[left : right+1]
	totalVar := variance(sub) * float64(len(sub))

	bestIdx := -1
	bestScore := -1.0

	for i := left + 1; i < right; i++ {
		if times[i].Sub(times[left]).Seconds() < intervalMinSegmentS {
			continue
		}
		if times[right].Sub(times[i]).Seconds() < intervalMinSegmentS {
			break
		}
		lSeg := signal[left:i]
		rSeg := signal[i : right+1]
		if len(lSeg) < 2 || len(rSeg) < 2 {
			continue
		}
		score := totalVar - (variance(lSeg)*float64(len(lSeg)) + variance(rSeg)*float64(len(rSeg)))
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	if bestIdx < 0 || bestScore < stddev(sub)*0.5 {
		*out = append(*out, intervalSegment{
			startIdx: left, endIdx: right,
			startT: times[left], endT: times[right],
		})
		return
	}

	binarySplit(signal, times, left, bestIdx, depth+1, out)
	binarySplit(signal, times, bestIdx+1, right, depth+1, out)
}

// sortSegments sorts segments by startIdx (insertion sort — typically few segments).
func sortSegments(segs []intervalSegment) {
	for i := 1; i < len(segs); i++ {
		key := segs[i]
		j := i - 1
		for j >= 0 && segs[j].startIdx > key.startIdx {
			segs[j+1] = segs[j]
			j--
		}
		segs[j+1] = key
	}
}

// classifySegments mirrors _classify_segments in intervals.py.
func classifySegments(segs []intervalSegment, signal []float64, pts []intervalStream) []classifiedSegment {
	globalMean := meanSlice(signal)
	result := make([]classifiedSegment, 0, len(segs))

	for _, seg := range segs {
		sl := signal[seg.startIdx : seg.endIdx+1]
		segMean := meanSlice(sl)

		ratio := 1.0
		if globalMean > 0 {
			ratio = segMean / globalMean
		}

		intensity := "recovery"
		switch {
		case ratio > intervalHardRatio:
			intensity = "hard"
		case ratio >= intervalModerateRatio:
			intensity = "moderate"
		case ratio >= intervalEasyRatio:
			intensity = "easy"
		}

		segPts := pts[seg.startIdx : seg.endIdx+1]

		var powerSum float64
		var powerCnt int
		var hrSum, hrCnt int
		var speedSum float64
		var speedCnt int
		var cadSum float64
		var cadCnt int

		for _, p := range segPts {
			if p.PowerW != nil {
				powerSum += *p.PowerW
				powerCnt++
			}
			if p.HeartRate != nil {
				hrSum += *p.HeartRate
				hrCnt++
			}
			if p.SpeedMS != nil {
				speedSum += *p.SpeedMS
				speedCnt++
			}
			if p.Cadence != nil {
				cadSum += *p.Cadence
				cadCnt++
			}
		}

		durationS := int(seg.endT.Sub(seg.startT).Seconds())
		if durationS < 1 {
			durationS = 1
		}

		cs := classifiedSegment{
			startT:    seg.startT,
			endT:      seg.endT,
			durationS: durationS,
			intensity: intensity,
		}
		if powerCnt > 0 {
			v := powerSum / float64(powerCnt)
			cs.avgPower = &v
		}
		if hrCnt > 0 {
			v := hrSum / hrCnt
			cs.avgHR = &v
		}
		if speedCnt > 0 {
			v := speedSum / float64(speedCnt)
			cs.avgSpeed = &v
		}
		if cadCnt > 0 {
			v := cadSum / float64(cadCnt)
			cs.avgCad = &v
		}

		result = append(result, cs)
	}

	return result
}
