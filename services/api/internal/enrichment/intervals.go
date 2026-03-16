package enrichment

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

const (
	intervalMinActivityRows = 30  // minimum stream rows to attempt detection
	intervalMinSegmentRows  = 30  // minimum rows per binary-split segment
	intervalMaxDepth        = 10  // maximum recursion depth for binary split
	intervalHardRatio       = 1.10
	intervalModerateRatio   = 0.90
	intervalEasyRatio       = 0.70
	intervalSmoothWindow    = 10 // rolling-mean window size (samples, not seconds)
)

// intervalStream holds a single row from activity_streams.
type intervalStream struct {
	T        time.Time
	PowerW   *float64
	HeartRate *int
	SpeedMS  *float64
	Cadence  *float64
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

// enrichStreamIntervals is the Go port of services/ml/enrichment/intervals.py.
//
// Algorithm (binary segmentation, mirrors the Python implementation):
//  1. Load the activity stream (time, power_w, heart_rate, speed_ms, cadence).
//  2. Select primary signal channel: power_w > heart_rate > speed_ms.
//  3. Apply rolling-mean smoothing.
//  4. Recursively split the signal at the point of maximum variance reduction.
//  5. Classify each resulting segment as hard/moderate/easy/recovery relative to
//     the global mean.
//  6. If ≥ 2 hard segments are found, write results to activity_intervals and
//     apply the auto-tag "Intervalltraining".
func enrichStreamIntervals(ctx context.Context, db *database.DB, userID, activityID string) error {
	// 1. Load stream.
	rows, err := db.Pool.Query(ctx, `
		SELECT time, power_w, heart_rate, speed_ms, cadence
		FROM activity_streams
		WHERE activity_id = $1
		ORDER BY time
	`, activityID)
	if err != nil {
		return fmt.Errorf("intervals: query stream: %w", err)
	}

	var pts []intervalStream
	for rows.Next() {
		var p intervalStream
		if err := rows.Scan(&p.T, &p.PowerW, &p.HeartRate, &p.SpeedMS, &p.Cadence); err != nil {
			rows.Close()
			return fmt.Errorf("intervals: scan stream: %w", err)
		}
		pts = append(pts, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("intervals: stream rows: %w", err)
	}

	if len(pts) < intervalMinActivityRows {
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
	if hardCount < 2 {
		return nil
	}

	// 5. Persist results.
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

	// 6. Auto-tag "Intervalltraining".
	if err := upsertIntervalTag(ctx, db, activityID, userID); err != nil {
		slog.Warn("intervals: auto-tag failed",
			"activity_id", activityID, "error", err)
	}

	slog.Info("intervals: enrichment complete",
		"activity_id", activityID,
		"segments", len(classified),
		"hard", hardCount,
	)
	return nil
}

// upsertIntervalTag inserts (or ignores) the "Intervalltraining" auto-tag and
// assigns it to the activity, mirroring the Python behaviour exactly.
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

// extractSignal picks the best signal channel (power > HR > speed) and applies
// rolling-mean smoothing.  Returns nil if no channel has >50 % non-null coverage.
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
			for i, p := range pts {
				if p.PowerW != nil {
					raw[i] = *p.PowerW
				}
			}
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
			for i, p := range pts {
				if p.HeartRate != nil {
					raw[i] = float64(*p.HeartRate)
				}
			}
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
			for i, p := range pts {
				if p.SpeedMS != nil {
					raw[i] = *p.SpeedMS
				}
			}
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
	if right-left < intervalMinSegmentRows || depth > intervalMaxDepth {
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

	for i := left + intervalMinSegmentRows; i < right-intervalMinSegmentRows; i++ {
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
	binarySplit(signal, times, bestIdx, right, depth+1, out)
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
