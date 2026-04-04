package database

import (
	"context"
	"fmt"
	"time"
)

// RunningFormBaseline holds 90-day median running form metrics for a user.
type RunningFormBaseline struct {
	DutyFactorMedian    *float64 `json:"duty_factor_median"`
	VerticalRatioMedian *float64 `json:"vertical_ratio_median"`
	GCTAsymmetryMedian  *float64 `json:"gct_asymmetry_median"`
	ActivityCount       int      `json:"activity_count"`
}

// RunningFormPoint holds per-activity running form data for the trend endpoint.
type RunningFormPoint struct {
	ActivityID    string   `json:"activity_id"`
	Date          string   `json:"date"`
	DutyFactor    *float64 `json:"duty_factor"`
	VerticalRatio *float64 `json:"vertical_ratio"`
	GCTAsymmetry  *float64 `json:"gct_asymmetry"`
	Tier          int      `json:"tier"`
}

// GetRunningFormBaseline returns 90-day median running form metrics for a user.
func (db *DB) GetRunningFormBaseline(ctx context.Context, userID string) (*RunningFormBaseline, error) {
	row := db.Pool.QueryRow(ctx, `
		SELECT
			PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY running_form_duty_factor) AS duty_factor_median,
			PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY running_form_vertical_ratio) AS vr_median,
			PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY running_form_gct_asymmetry_pct) AS asymmetry_median,
			COUNT(*) AS activity_count
		FROM activities
		WHERE user_id = $1
			AND running_form_tier IS NOT NULL
			AND sport_type IN ('running', 'trail_running')
			AND started_at >= NOW() - INTERVAL '90 days'
			AND deleted_at IS NULL
	`, userID)

	var b RunningFormBaseline
	if err := row.Scan(
		&b.DutyFactorMedian,
		&b.VerticalRatioMedian,
		&b.GCTAsymmetryMedian,
		&b.ActivityCount,
	); err != nil {
		return nil, fmt.Errorf("get running form baseline: %w", err)
	}
	return &b, nil
}

// GetRunningFormTrend returns per-activity running form values as a time series.
// from and to are optional; nil means unbounded on that side.
func (db *DB) GetRunningFormTrend(ctx context.Context, userID string, from, to *time.Time) ([]RunningFormPoint, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, started_at::date, running_form_duty_factor, running_form_vertical_ratio,
			running_form_gct_asymmetry_pct, running_form_tier
		FROM activities
		WHERE user_id = $1
			AND running_form_tier IS NOT NULL
			AND sport_type IN ('running', 'trail_running')
			AND deleted_at IS NULL
			AND ($2::timestamptz IS NULL OR started_at >= $2)
			AND ($3::timestamptz IS NULL OR started_at <= $3 + INTERVAL '1 day')
		ORDER BY started_at ASC
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get running form trend: %w", err)
	}
	defer rows.Close()

	points := make([]RunningFormPoint, 0)
	for rows.Next() {
		var p RunningFormPoint
		var date time.Time
		if err := rows.Scan(&p.ActivityID, &date, &p.DutyFactor, &p.VerticalRatio, &p.GCTAsymmetry, &p.Tier); err != nil {
			return nil, fmt.Errorf("scan running form trend: %w", err)
		}
		p.Date = date.Format("2006-01-02")
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("running form trend rows: %w", err)
	}
	return points, nil
}
