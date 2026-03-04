package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// HRVPoint is a single HRV data point for chart rendering.
type HRVPoint struct {
	Date      string   `json:"date"`
	RMSSD     *float64 `json:"rmssd"`
	SDRR      *float64 `json:"sdrr"`
	TimeOfDay string   `json:"time_of_day"`
}

// GetHRVAnalysis loads HRV snapshots for chart display over a time range.
func (db *DB) GetHRVAnalysis(ctx context.Context, userID string, from, to time.Time) ([]HRVPoint, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT
			measured_at_utc::date::text AS date,
			rmssd_ms::float,
			sdrr_ms::float,
			COALESCE(time_of_day, '')
		FROM health_hrv_snapshots
		WHERE user_id = $1
		  AND measured_at_utc BETWEEN $2 AND $3
		ORDER BY measured_at_utc
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get hrv analysis: %w", err)
	}
	defer rows.Close()

	var result []HRVPoint
	for rows.Next() {
		var p HRVPoint
		if err := rows.Scan(&p.Date, &p.RMSSD, &p.SDRR, &p.TimeOfDay); err != nil {
			return nil, fmt.Errorf("scan hrv point: %w", err)
		}
		result = append(result, p)
	}
	if result == nil {
		result = []HRVPoint{}
	}
	return result, rows.Err()
}

// HRVCorrelationPoint links daily RMSSD to the prior day's TSS.
type HRVCorrelationPoint struct {
	Date     string  `json:"date"`
	RMSSD    float64 `json:"rmssd"`
	DailyTSS float64 `json:"daily_tss"`
}

// GetHRVCorrelation loads paired HRV/TSS data for Pearson-correlation analysis.
// TSS is summed from activities on the day *before* the HRV measurement.
func (db *DB) GetHRVCorrelation(ctx context.Context, userID string, from, to time.Time) ([]HRVCorrelationPoint, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT
			h.measured_at_utc::date AS date,
			h.rmssd_ms::float       AS rmssd,
			COALESCE(SUM(a.tss), 0) AS daily_tss
		FROM health_hrv_snapshots h
		LEFT JOIN activities a
			ON  a.user_id    = h.user_id
			AND a.started_at::date = h.measured_at_utc::date - INTERVAL '1 day'
		WHERE h.user_id = $1
		  AND h.rmssd_ms IS NOT NULL
		  AND h.measured_at_utc BETWEEN $2 AND $3
		GROUP BY h.measured_at_utc::date, h.rmssd_ms
		ORDER BY date
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get hrv correlation: %w", err)
	}
	defer rows.Close()

	var result []HRVCorrelationPoint
	for rows.Next() {
		var p HRVCorrelationPoint
		if err := rows.Scan(&p.Date, &p.RMSSD, &p.DailyTSS); err != nil {
			return nil, fmt.Errorf("scan hrv correlation point: %w", err)
		}
		result = append(result, p)
	}
	if result == nil {
		result = []HRVCorrelationPoint{}
	}
	return result, rows.Err()
}

// GetRRIntervals loads raw R-R interval values for an activity (for DFA-alpha1).
// Returns an empty slice when the activity has no stored R-R data.
func (db *DB) GetRRIntervals(ctx context.Context, activityID string) ([]int16, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT rr_ms
		FROM activity_rr_intervals
		WHERE activity_id = $1
		ORDER BY seq
	`, activityID)
	if err != nil {
		// Table may not exist yet — return empty gracefully.
		return nil, fmt.Errorf("get rr intervals: %w", err)
	}
	defer rows.Close()

	var result []int16
	for rows.Next() {
		var v int16
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan rr interval: %w", err)
		}
		result = append(result, v)
	}
	if result == nil {
		result = []int16{}
	}
	return result, rows.Err()
}

// InsertRRIntervals stores raw R-R intervals (in ms) for an activity.
// Existing rows for the activity are replaced on conflict (idempotent on re-import).
func (db *DB) InsertRRIntervals(ctx context.Context, activityID string, intervals []uint16) error {
	if len(intervals) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for i, rr := range intervals {
		batch.Queue(
			`INSERT INTO activity_rr_intervals (activity_id, seq, rr_ms)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (activity_id, seq) DO UPDATE SET rr_ms = EXCLUDED.rr_ms`,
			activityID, i, int16(rr),
		)
	}
	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()
	for range intervals {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert rr interval: %w", err)
		}
	}
	return nil
}

// SleepAnalyticsPoint is a daily sleep summary for analytics charts.
type SleepAnalyticsPoint struct {
	Date         string   `json:"date"`
	DurationMin  *int     `json:"duration_min"`
	ScoreOverall *int     `json:"score_overall"`
	DeepSleepPct *float64 `json:"deep_sleep_pct"`
	RemSleepPct  *float64 `json:"rem_sleep_pct"`
}

// GetSleepAnalytics returns sleep metrics per day for the given time range.
func (db *DB) GetSleepAnalytics(ctx context.Context, userID string, from, to time.Time) ([]SleepAnalyticsPoint, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT
			calendar_date::text,
			CASE
				WHEN (COALESCE(deep_seconds,0) + COALESCE(light_seconds,0) + COALESCE(rem_seconds,0)) > 0
				THEN (COALESCE(deep_seconds,0) + COALESCE(light_seconds,0) + COALESCE(rem_seconds,0)) / 60
				ELSE NULL
			END AS duration_min,
			score_overall,
			CASE WHEN COALESCE(deep_seconds,0) + COALESCE(light_seconds,0) + COALESCE(rem_seconds,0) > 0
				THEN deep_seconds * 100.0 / NULLIF(deep_seconds + light_seconds + rem_seconds, 0)
				ELSE NULL
			END AS deep_sleep_pct,
			CASE WHEN COALESCE(deep_seconds,0) + COALESCE(light_seconds,0) + COALESCE(rem_seconds,0) > 0
				THEN rem_seconds * 100.0 / NULLIF(deep_seconds + light_seconds + rem_seconds, 0)
				ELSE NULL
			END AS rem_sleep_pct
		FROM health_sleep
		WHERE user_id = $1
		  AND calendar_date BETWEEN $2::date AND $3::date
		ORDER BY calendar_date
	`, userID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("get sleep analytics: %w", err)
	}
	defer rows.Close()

	var result []SleepAnalyticsPoint
	for rows.Next() {
		var p SleepAnalyticsPoint
		if err := rows.Scan(&p.Date, &p.DurationMin, &p.ScoreOverall, &p.DeepSleepPct, &p.RemSleepPct); err != nil {
			return nil, fmt.Errorf("scan sleep analytics: %w", err)
		}
		result = append(result, p)
	}
	if result == nil {
		result = []SleepAnalyticsPoint{}
	}
	return result, rows.Err()
}

// WeightPoint is a single body-weight measurement for analytics.
type WeightPoint struct {
	Date       string   `json:"date"`
	WeightKg   float64  `json:"weight_kg"`
	BodyFatPct *float64 `json:"body_fat_pct"`
}

// GetWeightAnalytics returns weight entries for the given time range.
// weight_g is stored in grams; this method converts to kg.
func (db *DB) GetWeightAnalytics(ctx context.Context, userID string, from, to time.Time) ([]WeightPoint, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT
			measured_at_utc::date::text AS date,
			weight_g / 1000.0           AS weight_kg,
			body_fat_pct
		FROM health_weight_entries
		WHERE user_id = $1
		  AND measured_at_utc BETWEEN $2 AND $3
		ORDER BY measured_at_utc
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get weight analytics: %w", err)
	}
	defer rows.Close()

	var result []WeightPoint
	for rows.Next() {
		var p WeightPoint
		if err := rows.Scan(&p.Date, &p.WeightKg, &p.BodyFatPct); err != nil {
			return nil, fmt.Errorf("scan weight point: %w", err)
		}
		result = append(result, p)
	}
	if result == nil {
		result = []WeightPoint{}
	}
	return result, rows.Err()
}

// HealthMetricPoint is a single value/date pair used inside HealthMetric.
type HealthMetricPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// HealthMetric aggregates a health data series with trend information.
type HealthMetric struct {
	Name    string              `json:"name"`
	Current float64             `json:"current"`
	Unit    string              `json:"unit"`
	Trend   float64             `json:"trend"` // +/- % vs. previous 7 days
	Points  []HealthMetricPoint `json:"points"`
}

// GetHealthMetrics returns aggregated health metric grid values for the given period.
// Included metrics: RHR, HRV mean, sleep duration, sleep score, weight (if available).
func (db *DB) GetHealthMetrics(ctx context.Context, userID string, from, to time.Time) ([]HealthMetric, error) {
	var metrics []HealthMetric

	// --- Resting Heart Rate (from health_daily_summary) ---
	rhrMetric, err := db.healthMetricFromDailyRHR(ctx, userID, from, to)
	if err == nil && rhrMetric != nil {
		metrics = append(metrics, *rhrMetric)
	}

	// --- HRV Mean RMSSD ---
	hrvMetric, err := db.healthMetricFromHRV(ctx, userID, from, to)
	if err == nil && hrvMetric != nil {
		metrics = append(metrics, *hrvMetric)
	}

	// --- Sleep Duration ---
	sleepDurMetric, err := db.healthMetricFromSleepDuration(ctx, userID, from, to)
	if err == nil && sleepDurMetric != nil {
		metrics = append(metrics, *sleepDurMetric)
	}

	// --- Sleep Score ---
	sleepScoreMetric, err := db.healthMetricFromSleepScore(ctx, userID, from, to)
	if err == nil && sleepScoreMetric != nil {
		metrics = append(metrics, *sleepScoreMetric)
	}

	// --- Weight ---
	weightMetric, err := db.healthMetricFromWeight(ctx, userID, from, to)
	if err == nil && weightMetric != nil {
		metrics = append(metrics, *weightMetric)
	}

	if metrics == nil {
		metrics = []HealthMetric{}
	}
	return metrics, nil
}

// healthMetricFromDailyRHR builds an RHR HealthMetric from health_daily_summary.
func (db *DB) healthMetricFromDailyRHR(ctx context.Context, userID string, from, to time.Time) (*HealthMetric, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT calendar_date::text, resting_heart_rate::float
		FROM health_daily_summary
		WHERE user_id = $1
		  AND resting_heart_rate IS NOT NULL
		  AND calendar_date BETWEEN $2::date AND $3::date
		ORDER BY calendar_date
	`, userID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return buildMetricFromRows(rows, "rhr", "bpm")
}

// healthMetricFromHRV builds an HRV-mean HealthMetric from health_hrv_snapshots.
func (db *DB) healthMetricFromHRV(ctx context.Context, userID string, from, to time.Time) (*HealthMetric, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT measured_at_utc::date::text AS date, AVG(rmssd_ms)::float AS value
		FROM health_hrv_snapshots
		WHERE user_id = $1
		  AND rmssd_ms IS NOT NULL
		  AND measured_at_utc BETWEEN $2 AND $3
		GROUP BY measured_at_utc::date
		ORDER BY date
	`, userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return buildMetricFromRows(rows, "hrv_rmssd", "ms")
}

// healthMetricFromSleepDuration builds a sleep-duration HealthMetric from health_sleep.
func (db *DB) healthMetricFromSleepDuration(ctx context.Context, userID string, from, to time.Time) (*HealthMetric, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT
			calendar_date::text,
			((COALESCE(deep_seconds,0) + COALESCE(light_seconds,0) + COALESCE(rem_seconds,0)) / 60.0)::float
		FROM health_sleep
		WHERE user_id = $1
		  AND calendar_date BETWEEN $2::date AND $3::date
		  AND (deep_seconds IS NOT NULL OR light_seconds IS NOT NULL OR rem_seconds IS NOT NULL)
		ORDER BY calendar_date
	`, userID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return buildMetricFromRows(rows, "sleep_duration", "min")
}

// healthMetricFromSleepScore builds a sleep-score HealthMetric from health_sleep.
func (db *DB) healthMetricFromSleepScore(ctx context.Context, userID string, from, to time.Time) (*HealthMetric, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT calendar_date::text, score_overall::float
		FROM health_sleep
		WHERE user_id = $1
		  AND score_overall IS NOT NULL
		  AND calendar_date BETWEEN $2::date AND $3::date
		ORDER BY calendar_date
	`, userID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return buildMetricFromRows(rows, "sleep_score", "pts")
}

// healthMetricFromWeight builds a weight HealthMetric from health_weight_entries.
func (db *DB) healthMetricFromWeight(ctx context.Context, userID string, from, to time.Time) (*HealthMetric, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT measured_at_utc::date::text, (weight_g / 1000.0)::float
		FROM health_weight_entries
		WHERE user_id = $1
		  AND measured_at_utc BETWEEN $2 AND $3
		ORDER BY measured_at_utc
	`, userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return buildMetricFromRows(rows, "weight", "kg")
}

// buildMetricFromRows reads (date text, value float64) rows and assembles a HealthMetric.
// Trend is the % difference between the average of the last 7 points and the prior 7 points.
func buildMetricFromRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}, name, unit string) (*HealthMetric, error) {
	var points []HealthMetricPoint
	for rows.Next() {
		var date string
		var value float64
		if err := rows.Scan(&date, &value); err != nil {
			return nil, fmt.Errorf("scan metric %s: %w", name, err)
		}
		points = append(points, HealthMetricPoint{Date: date, Value: value})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(points) == 0 {
		return nil, nil
	}

	// Current value: last point.
	current := points[len(points)-1].Value

	// Trend: avg of last 7 vs. avg of prior 7.
	var sumRecent, sumPrior float64
	var cntRecent, cntPrior int
	n := len(points)
	for i, p := range points {
		if i >= n-7 {
			sumRecent += p.Value
			cntRecent++
		} else if i >= n-14 {
			sumPrior += p.Value
			cntPrior++
		}
	}
	var trend float64
	if cntPrior > 0 && cntRecent > 0 {
		avgRecent := sumRecent / float64(cntRecent)
		avgPrior := sumPrior / float64(cntPrior)
		if avgPrior != 0 {
			trend = (avgRecent - avgPrior) / avgPrior * 100
		}
	}

	return &HealthMetric{
		Name:    name,
		Current: current,
		Unit:    unit,
		Trend:   trend,
		Points:  points,
	}, nil
}
