package database

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (db *DB) GetPMCRange(ctx context.Context, userID string, from, to time.Time) ([]model.PMCDay, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT date::text, tss, ctl, atl, tsb FROM pmc_daily
		 WHERE user_id = $1 AND date >= $2 AND date <= $3
		 ORDER BY date ASC`,
		userID, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("get pmc range: %w", err)
	}
	defer rows.Close()

	var days []model.PMCDay
	for rows.Next() {
		var d model.PMCDay
		if err := rows.Scan(&d.Date, &d.TSS, &d.CTL, &d.ATL, &d.TSB); err != nil {
			return nil, fmt.Errorf("scan pmc day: %w", err)
		}
		if d.CTL > 0 {
			d.ACWR = math.Round(d.ATL/d.CTL*100) / 100
		}
		days = append(days, d)
	}
	if days == nil {
		days = []model.PMCDay{}
	}
	return days, nil
}

func (db *DB) UpsertPMCDay(ctx context.Context, userID string, date time.Time, tss, ctl, atl, tsb float64) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO pmc_daily (user_id, date, tss, ctl, atl, tsb)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (user_id, date) DO UPDATE
		 SET tss = EXCLUDED.tss, ctl = EXCLUDED.ctl, atl = EXCLUDED.atl, tsb = EXCLUDED.tsb`,
		userID, date, tss, ctl, atl, tsb,
	)
	if err != nil {
		return fmt.Errorf("upsert pmc day: %w", err)
	}
	return nil
}

func (db *DB) UpsertPMCDays(ctx context.Context, userID string, days []model.PMCDay) error {
	batch := &pgx.Batch{}
	for _, d := range days {
		batch.Queue(
			`INSERT INTO pmc_daily (user_id, date, tss, ctl, atl, tsb)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (user_id, date) DO UPDATE
			 SET tss = EXCLUDED.tss, ctl = EXCLUDED.ctl, atl = EXCLUDED.atl, tsb = EXCLUDED.tsb`,
			userID, d.Date, d.TSS, d.CTL, d.ATL, d.TSB,
		)
	}

	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range days {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("upsert pmc batch: %w", err)
		}
	}
	return nil
}

// GetDailyTSS returns a map of date -> summed TSS for a user in a date range.
// Falls back to estimating TSS from activity power/HR data.
func (db *DB) GetDailyTSS(ctx context.Context, userID string, from, to time.Time) (map[string]float64, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT started_at::date AS day,
			COALESCE(SUM(
				CASE
					WHEN tss IS NOT NULL AND tss > 0 THEN tss
					WHEN avg_power_w IS NOT NULL AND avg_power_w > 0 AND moving_time_s IS NOT NULL THEN
						(moving_time_s::float * avg_power_w * (avg_power_w / 200.0)) / (200.0 * 3600) * 100
					WHEN avg_heart_rate IS NOT NULL AND avg_heart_rate > 0 AND moving_time_s IS NOT NULL THEN
						moving_time_s::float / 3600 * (avg_heart_rate / 190.0) * 100
					ELSE 0
				END
			), 0) AS daily_tss
		 FROM activities
		 WHERE user_id = $1 AND started_at >= $2 AND started_at < ($3::date + 1)
		 GROUP BY day
		 ORDER BY day`,
		userID, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("get daily tss: %w", err)
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var day time.Time
		var tss float64
		if err := rows.Scan(&day, &tss); err != nil {
			return nil, fmt.Errorf("scan daily tss: %w", err)
		}
		result[day.Format("2006-01-02")] = tss
	}
	return result, nil
}

// GetEarliestActivityDate returns the started_at of the user's oldest activity.
// Returns zero time if the user has no activities.
func (db *DB) GetEarliestActivityDate(ctx context.Context, userID string) (time.Time, error) {
	var t time.Time
	err := db.Pool.QueryRow(ctx,
		`SELECT MIN(started_at) FROM activities WHERE user_id = $1`,
		userID,
	).Scan(&t)
	if err == pgx.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("get earliest activity date: %w", err)
	}
	return t, nil
}

// GetPMCDayBefore returns the PMC values for the day before `date` for seeding calculations.
func (db *DB) GetPMCDayBefore(ctx context.Context, userID string, date time.Time) (ctl, atl float64, err error) {
	before := date.AddDate(0, 0, -1)
	err = db.Pool.QueryRow(ctx,
		`SELECT ctl, atl FROM pmc_daily WHERE user_id = $1 AND date = $2`,
		userID, before,
	).Scan(&ctl, &atl)
	if err == pgx.ErrNoRows {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("get pmc day before: %w", err)
	}
	return ctl, atl, nil
}
