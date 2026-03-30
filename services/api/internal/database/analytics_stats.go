package database

import (
	"context"
	"fmt"
	"time"
)

// LifetimeStats holds aggregated lifetime statistics for a user.
type LifetimeStats struct {
	TotalActivities   int64   `json:"total_activities"`
	TotalDistanceM    float64 `json:"total_distance_m"`
	TotalElevationM   float64 `json:"total_elevation_m"`
	TotalMovingTimeS  int64   `json:"total_moving_time_s"`
	TotalElapsedTimeS int64   `json:"total_elapsed_time_s"`
	TotalCalories     int64   `json:"total_calories"`
	ActiveDays        int64   `json:"active_days"`
	Countries         int64   `json:"countries"`
}

// GetLifetimeStats returns aggregated lifetime statistics for a user.
// whereClause and args are the additional filter conditions (from BuildWhereClause),
// where placeholder numbering starts at startIdx=2 (since $1 is userID).
func (db *DB) GetLifetimeStats(ctx context.Context, userID string, whereClause string, args []any) (*LifetimeStats, error) {
	query := `
		SELECT
			COUNT(*)                             AS total_activities,
			COALESCE(SUM(distance_m), 0)        AS total_distance_m,
			COALESCE(SUM(elevation_gain_m), 0)  AS total_elevation_m,
			COALESCE(SUM(moving_time_s), 0)     AS total_moving_time_s,
			COALESCE(SUM(elapsed_time_s), 0)    AS total_elapsed_time_s,
			COALESCE(SUM(total_calories), 0)    AS total_calories,
			COUNT(DISTINCT (a.started_at AT TIME ZONE COALESCE(a.timezone, 'UTC'))::date) AS active_days,
			COUNT(DISTINCT start_country)        AS countries
		FROM activities a
		WHERE a.user_id = $1` + whereClause

	allArgs := append([]any{userID}, args...)
	var s LifetimeStats
	err := db.Pool.QueryRow(ctx, query, allArgs...).Scan(
		&s.TotalActivities,
		&s.TotalDistanceM,
		&s.TotalElevationM,
		&s.TotalMovingTimeS,
		&s.TotalElapsedTimeS,
		&s.TotalCalories,
		&s.ActiveDays,
		&s.Countries,
	)
	if err != nil {
		return nil, fmt.Errorf("get lifetime stats: %w", err)
	}
	return &s, nil
}

// GetEddingtonData returns per-day aggregated values sorted descending for
// Eddington number calculation.
// metric: "km" → SUM(distance_m)/1000, "elevation" → SUM(elevation_gain_m)
func (db *DB) GetEddingtonData(ctx context.Context, userID string, metric string, whereClause string, args []any) ([]float64, error) {
	// Use whitelisted metric to avoid injection.
	var aggExpr string
	switch metric {
	case "elevation":
		aggExpr = "COALESCE(SUM(elevation_gain_m), 0)"
	default:
		// default: km
		aggExpr = "COALESCE(SUM(distance_m), 0) / 1000.0"
	}

	query := `
		SELECT ` + aggExpr + ` AS value
		FROM activities a
		WHERE a.user_id = $1` + whereClause + `
		GROUP BY (a.started_at AT TIME ZONE COALESCE(a.timezone, 'UTC'))::date
		ORDER BY value DESC`

	allArgs := append([]any{userID}, args...)
	rows, err := db.Pool.Query(ctx, query, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("get eddington data: %w", err)
	}
	defer rows.Close()

	var values []float64
	for rows.Next() {
		var v float64
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan eddington data: %w", err)
		}
		values = append(values, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get eddington data: %w", err)
	}
	if values == nil {
		values = []float64{}
	}
	return values, nil
}

// DistributionRow holds a single row in a distribution result set.
type DistributionRow struct {
	Label      string  `json:"label"`
	Count      int64   `json:"count"`
	Value      float64 `json:"value"`
	Percentage float64 `json:"percentage"`
}

// GetDistribution returns activity distribution by sport, country, gear, or distance bucket.
// distType: "sport"|"country"|"gear"|"distance_bucket"
// metric: "count"|"distance_m"|"elevation_gain_m"|"moving_time_s"
func (db *DB) GetDistribution(ctx context.Context, userID string, distType string, metric string, whereClause string, args []any) ([]DistributionRow, error) {
	// Whitelist metric to avoid injection.
	var valueExpr string
	switch metric {
	case "distance_m":
		valueExpr = "COALESCE(SUM(a.distance_m), 0)"
	case "elevation_gain_m":
		valueExpr = "COALESCE(SUM(a.elevation_gain_m), 0)"
	case "moving_time_s":
		valueExpr = "COALESCE(SUM(a.moving_time_s), 0)"
	default:
		// "count"
		valueExpr = "COUNT(*)"
	}

	var labelExpr, groupBy, joinClause string
	switch distType {
	case "country":
		labelExpr = "COALESCE(a.start_country, 'Unknown')"
		groupBy = "COALESCE(a.start_country, 'Unknown')"
	case "gear":
		labelExpr = "COALESCE(g.name, 'No Gear')"
		groupBy = "COALESCE(g.name, 'No Gear')"
		joinClause = "LEFT JOIN gear g ON g.id = a.gear_id"
	case "distance_bucket":
		labelExpr = `CASE
			WHEN a.distance_m IS NULL OR a.distance_m = 0 THEN 'No distance'
			WHEN a.distance_m < 5000    THEN '<5 km'
			WHEN a.distance_m < 10000   THEN '5–10 km'
			WHEN a.distance_m < 20000   THEN '10–20 km'
			WHEN a.distance_m < 50000   THEN '20–50 km'
			WHEN a.distance_m < 100000  THEN '50–100 km'
			ELSE '100+ km'
		END`
		groupBy = labelExpr
	default:
		// "sport"
		labelExpr = "COALESCE(a.sport_type, 'unknown')"
		groupBy = "COALESCE(a.sport_type, 'unknown')"
	}

	// Build the query with window function for percentage.
	query := `
		SELECT
			` + labelExpr + `            AS label,
			COUNT(*)                     AS count,
			` + valueExpr + `            AS value,
			ROUND(
				(` + valueExpr + ` * 100.0) /
				NULLIF(SUM(` + valueExpr + `) OVER (), 0),
				2
			)                            AS percentage
		FROM activities a
		` + joinClause + `
		WHERE a.user_id = $1` + whereClause + `
		GROUP BY ` + groupBy + `
		ORDER BY value DESC`

	allArgs := append([]any{userID}, args...)
	rows, err := db.Pool.Query(ctx, query, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("get distribution (%s/%s): %w", distType, metric, err)
	}
	defer rows.Close()

	var result []DistributionRow
	for rows.Next() {
		var row DistributionRow
		if err := rows.Scan(&row.Label, &row.Count, &row.Value, &row.Percentage); err != nil {
			return nil, fmt.Errorf("scan distribution: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get distribution: %w", err)
	}
	if result == nil {
		result = []DistributionRow{}
	}
	return result, nil
}

// YearlyOverviewRow holds aggregated activity data per year/month.
type YearlyOverviewRow struct {
	Year       int     `json:"year"`
	Month      int     `json:"month"`
	Activities int64   `json:"count"`
	DistanceM  float64 `json:"distance_m"`
	ElevationM float64 `json:"elevation_m"`
	DurationS  int64   `json:"duration_s"`
}

// GetYearlyOverview returns per-year/month aggregated activity data for the user.
// No additional filters — always covers all data.
func (db *DB) GetYearlyOverview(ctx context.Context, userID string) ([]YearlyOverviewRow, error) {
	query := `
		SELECT
			EXTRACT(YEAR FROM started_at)::int   AS year,
			EXTRACT(MONTH FROM started_at)::int  AS month,
			COUNT(*)                             AS activities,
			COALESCE(SUM(distance_m), 0)        AS distance_m,
			COALESCE(SUM(elevation_gain_m), 0)  AS elevation_m,
			COALESCE(SUM(moving_time_s), 0)     AS duration_s
		FROM activities
		WHERE user_id = $1
		GROUP BY year, month
		ORDER BY year DESC, month`

	rows, err := db.Pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get yearly overview: %w", err)
	}
	defer rows.Close()

	var result []YearlyOverviewRow
	for rows.Next() {
		var r YearlyOverviewRow
		if err := rows.Scan(&r.Year, &r.Month, &r.Activities, &r.DistanceM, &r.ElevationM, &r.DurationS); err != nil {
			return nil, fmt.Errorf("scan yearly overview: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get yearly overview: %w", err)
	}
	if result == nil {
		result = []YearlyOverviewRow{}
	}
	return result, nil
}

// GetActiveDays returns all distinct active dates for a user, sorted ascending.
// Used for streak calculation.
func (db *DB) GetActiveDays(ctx context.Context, userID string) ([]time.Time, error) {
	query := `
		SELECT DISTINCT (started_at AT TIME ZONE COALESCE(timezone, 'UTC'))::date AS day
		FROM activities
		WHERE user_id = $1
		ORDER BY day`

	rows, err := db.Pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get active days: %w", err)
	}
	defer rows.Close()

	var days []time.Time
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("scan active day: %w", err)
		}
		days = append(days, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get active days: %w", err)
	}
	if days == nil {
		days = []time.Time{}
	}
	return days, nil
}
