package database

import (
	"context"
	"fmt"
	"time"
)

// YearProgressionRow holds a single month/year data point for year-on-year comparison.
type YearProgressionRow struct {
	Year  int
	Month int
	Value float64
}

// TrainingVolumeRow holds volume data aggregated by period and sport type.
type TrainingVolumeRow struct {
	Period    time.Time
	SportType string
	Value     float64
}

// ZoneDistributionRow holds time-in-zone data aggregated by period and zone index.
type ZoneDistributionRow struct {
	Period       time.Time
	ZoneIndex    int
	TotalSeconds int64
}

// PersonalRecord holds a single personal record entry.
type PersonalRecord struct {
	Type       string  `json:"type"`
	Value      float64 `json:"value"`
	Unit       string  `json:"unit"`
	ActivityID string  `json:"activity_id"`
	Date       string  `json:"date"`
	Trend      string  `json:"trend"` // "up"|"down"|"same"
}

// safeMetricColumn maps user-supplied metric strings to safe SQL column names.
// Returns an empty string if the metric is unknown.
func safeMetricColumn(metric string) string {
	switch metric {
	case "distance_m":
		return "distance_m"
	case "elevation_gain_m":
		return "elevation_gain_m"
	case "moving_time_s":
		return "moving_time_s"
	case "tss":
		return "tss"
	case "total_calories":
		return "total_calories"
	case "count":
		return "count"
	default:
		return "distance_m"
	}
}

// safeGranularity maps user-supplied granularity strings to safe SQL DATE_TRUNC values.
func safeGranularity(granularity string) string {
	switch granularity {
	case "week":
		return "week"
	case "month":
		return "month"
	case "year":
		return "year"
	case "day":
		return "day"
	default:
		return "week"
	}
}

// GetYearProgression returns monthly aggregated metric values per year for year-on-year comparison.
// startIdx is the starting placeholder index (e.g. 2 if $1 is already used for userID).
// The WHERE clause is appended directly after the fixed user_id condition.
func (db *DB) GetYearProgression(ctx context.Context, userID string, metric string, whereClause string, args []any) ([]YearProgressionRow, error) {
	col := safeMetricColumn(metric)

	var aggExpr string
	if col == "count" {
		aggExpr = "COUNT(*)"
	} else {
		aggExpr = "COALESCE(SUM(" + col + "), 0)"
	}

	query := `
		SELECT
			EXTRACT(YEAR FROM started_at)::int  AS year,
			EXTRACT(MONTH FROM started_at)::int AS month,
			` + aggExpr + `                     AS value
		FROM activities a
		WHERE user_id = $1` + whereClause + `
		GROUP BY year, month
		ORDER BY year, month`

	allArgs := append([]any{userID}, args...)
	rows, err := db.Pool.Query(ctx, query, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("get year progression: %w", err)
	}
	defer rows.Close()

	var result []YearProgressionRow
	for rows.Next() {
		var r YearProgressionRow
		if err := rows.Scan(&r.Year, &r.Month, &r.Value); err != nil {
			return nil, fmt.Errorf("scan year progression: %w", err)
		}
		result = append(result, r)
	}
	if result == nil {
		result = []YearProgressionRow{}
	}
	return result, nil
}

// GetTrainingVolume returns aggregated metric per period and sport type.
// granularity: "week"|"month"|"year"|"day"
// metric: "distance_m"|"elevation_gain_m"|"moving_time_s"|"tss"|"total_calories"|"count"
// startIdx is the starting placeholder for args (after $1=userID).
func (db *DB) GetTrainingVolume(ctx context.Context, userID string, granularity string, metric string, whereClause string, args []any) ([]TrainingVolumeRow, error) {
	col := safeMetricColumn(metric)
	gran := safeGranularity(granularity)

	var aggExpr string
	if col == "count" {
		aggExpr = "COUNT(*)"
	} else {
		aggExpr = "COALESCE(SUM(" + col + "), 0)"
	}

	query := `
		SELECT
			DATE_TRUNC('` + gran + `', started_at) AS period,
			COALESCE(sport_type, 'unknown')          AS sport_type,
			` + aggExpr + `                           AS value
		FROM activities a
		WHERE user_id = $1` + whereClause + `
		GROUP BY period, sport_type
		ORDER BY period, sport_type`

	allArgs := append([]any{userID}, args...)
	rows, err := db.Pool.Query(ctx, query, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("get training volume: %w", err)
	}
	defer rows.Close()

	var result []TrainingVolumeRow
	for rows.Next() {
		var r TrainingVolumeRow
		if err := rows.Scan(&r.Period, &r.SportType, &r.Value); err != nil {
			return nil, fmt.Errorf("scan training volume: %w", err)
		}
		result = append(result, r)
	}
	if result == nil {
		result = []TrainingVolumeRow{}
	}
	return result, nil
}

// GetZoneDistribution returns time-in-zone totals aggregated by period and zone index.
// zoneType: "hr"|"power"|"cadence"
// granularity: "week"|"month"|"year"|"day"
func (db *DB) GetZoneDistribution(ctx context.Context, userID string, zoneType string, granularity string, whereClause string, args []any) ([]ZoneDistributionRow, error) {
	// Validate zoneType to prevent injection.
	safeZoneType := "hr"
	switch zoneType {
	case "hr", "power", "cadence":
		safeZoneType = zoneType
	}
	gran := safeGranularity(granularity)

	// $1=userID, $2=zoneType; filter args start at $3.
	// But whereClause already uses the caller's startIdx offset.
	// We prepend userID and zoneType, so filter args follow.
	query := `
		SELECT
			DATE_TRUNC('` + gran + `', a.started_at) AS period,
			tz.zone_index,
			SUM(tz.seconds)::bigint                  AS total_seconds
		FROM activity_time_in_zone tz
		JOIN activities a ON a.id = tz.activity_id
		WHERE a.user_id = $1
		  AND tz.zone_type = $2` + whereClause + `
		GROUP BY period, tz.zone_index
		ORDER BY period, tz.zone_index`

	allArgs := append([]any{userID, safeZoneType}, args...)
	rows, err := db.Pool.Query(ctx, query, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("get zone distribution: %w", err)
	}
	defer rows.Close()

	var result []ZoneDistributionRow
	for rows.Next() {
		var r ZoneDistributionRow
		if err := rows.Scan(&r.Period, &r.ZoneIndex, &r.TotalSeconds); err != nil {
			return nil, fmt.Errorf("scan zone distribution: %w", err)
		}
		result = append(result, r)
	}
	if result == nil {
		result = []ZoneDistributionRow{}
	}
	return result, nil
}

// personalRecordQuery fetches the activity_id, value, and started_at for a single PR metric.
type prRow struct {
	activityID string
	value      float64
	date       time.Time
}

func (db *DB) fetchMaxPR(ctx context.Context, userID, col string, extraWhere string) (prRow, error) {
	query := `
		SELECT id, ` + col + ` AS value, started_at
		FROM activities
		WHERE user_id = $1
		  AND ` + col + ` IS NOT NULL
		  AND ` + col + ` > 0` + extraWhere + `
		ORDER BY value DESC
		LIMIT 1`
	var r prRow
	err := db.Pool.QueryRow(ctx, query, userID).Scan(&r.activityID, &r.value, &r.date)
	return r, err
}

func (db *DB) fetchMaxPRInWindow(ctx context.Context, userID, col string, extraWhere string, from, to time.Time) (float64, error) {
	query := `
		SELECT COALESCE(MAX(` + col + `), 0)
		FROM activities
		WHERE user_id = $1
		  AND ` + col + ` IS NOT NULL
		  AND ` + col + ` > 0
		  AND started_at >= $2
		  AND started_at < $3` + extraWhere
	var v float64
	err := db.Pool.QueryRow(ctx, query, userID, from, to).Scan(&v)
	return v, err
}

func trendStr(current, previous float64, higherIsBetter bool) string {
	if previous == 0 {
		return "same"
	}
	if higherIsBetter {
		if current > previous {
			return "up"
		} else if current < previous {
			return "down"
		}
		return "same"
	}
	// Lower is better (e.g. fastest pace = lower avg_speed threshold... but we store
	// max_distance and min avg_speed for fastest pace separately).
	if current < previous {
		return "up"
	} else if current > previous {
		return "down"
	}
	return "same"
}

// GetPersonalRecords returns personal records for the user across all activities.
// Trend is computed by comparing the last 90 days against the prior 90-day window.
func (db *DB) GetPersonalRecords(ctx context.Context, userID string) ([]PersonalRecord, error) {
	now := time.Now()
	window1Start := now.AddDate(0, 0, -90)
	window2Start := now.AddDate(0, 0, -180)
	window2End := window1Start

	var records []PersonalRecord

	// 1. Max distance
	if r, err := db.fetchMaxPR(ctx, userID, "distance_m", ""); err == nil && r.activityID != "" {
		cur, _ := db.fetchMaxPRInWindow(ctx, userID, "distance_m", "", window1Start, now)
		prev, _ := db.fetchMaxPRInWindow(ctx, userID, "distance_m", "", window2Start, window2End)
		records = append(records, PersonalRecord{
			Type:       "max_distance",
			Value:      r.value,
			Unit:       "m",
			ActivityID: r.activityID,
			Date:       r.date.Format("2006-01-02"),
			Trend:      trendStr(cur, prev, true),
		})
	}

	// 2. Max elevation gain
	if r, err := db.fetchMaxPR(ctx, userID, "elevation_gain_m", ""); err == nil && r.activityID != "" {
		cur, _ := db.fetchMaxPRInWindow(ctx, userID, "elevation_gain_m", "", window1Start, now)
		prev, _ := db.fetchMaxPRInWindow(ctx, userID, "elevation_gain_m", "", window2Start, window2End)
		records = append(records, PersonalRecord{
			Type:       "max_elevation",
			Value:      r.value,
			Unit:       "m",
			ActivityID: r.activityID,
			Date:       r.date.Format("2006-01-02"),
			Trend:      trendStr(cur, prev, true),
		})
	}

	// 3. Fastest pace (min avg_speed_ms for activities >= 10km)
	// Higher avg_speed_ms means faster pace — we want max speed.
	{
		extraWhere := " AND distance_m >= 10000"
		if r, err := db.fetchMaxPR(ctx, userID, "avg_speed_ms", extraWhere); err == nil && r.activityID != "" {
			cur, _ := db.fetchMaxPRInWindow(ctx, userID, "avg_speed_ms", extraWhere, window1Start, now)
			prev, _ := db.fetchMaxPRInWindow(ctx, userID, "avg_speed_ms", extraWhere, window2Start, window2End)
			records = append(records, PersonalRecord{
				Type:       "fastest_pace",
				Value:      r.value,
				Unit:       "m/s",
				ActivityID: r.activityID,
				Date:       r.date.Format("2006-01-02"),
				Trend:      trendStr(cur, prev, true),
			})
		}
	}

	// 4. Max average power
	if r, err := db.fetchMaxPR(ctx, userID, "avg_power_w", ""); err == nil && r.activityID != "" {
		cur, _ := db.fetchMaxPRInWindow(ctx, userID, "avg_power_w", "", window1Start, now)
		prev, _ := db.fetchMaxPRInWindow(ctx, userID, "avg_power_w", "", window2Start, window2End)
		records = append(records, PersonalRecord{
			Type:       "max_avg_power",
			Value:      r.value,
			Unit:       "W",
			ActivityID: r.activityID,
			Date:       r.date.Format("2006-01-02"),
			Trend:      trendStr(cur, prev, true),
		})
	}

	if records == nil {
		records = []PersonalRecord{}
	}
	return records, nil
}
