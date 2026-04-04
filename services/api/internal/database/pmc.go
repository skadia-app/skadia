package database

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// UpdateActivityTSS saves computed TSS metrics for a single activity.
// Parameters correspond to pmc.TSSResult fields; kept as primitives to avoid
// a circular import between the database and pmc packages.
func (db *DB) UpdateActivityTSS(ctx context.Context, activityID string,
	computedTSS, computedNPW, computedNGPMps, computedIF, trimp *float64,
	tssMethod string,
) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE activities SET
			computed_tss = $2, computed_np_w = $3, computed_ngp_mps = $4,
			tss_method = $5, computed_if = $6, trimp = $7
		WHERE id = $1
	`, activityID, computedTSS, computedNPW, computedNGPMps,
		tssMethod, computedIF, trimp)
	return err
}

func (db *DB) GetPMCRange(ctx context.Context, userID string, from, to time.Time) ([]model.PMCDay, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT date::text, tss, ctl, atl, tsb, COALESCE(acwr_ra, 0) FROM pmc_daily
		 WHERE user_id = $1 AND date >= $2 AND date <= $3
		   AND sport_group = '__combined__'
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
		if err := rows.Scan(&d.Date, &d.TSS, &d.CTL, &d.ATL, &d.TSB, &d.ACWRRA); err != nil {
			return nil, fmt.Errorf("scan pmc day: %w", err)
		}
		if d.CTL > 0 {
			d.ACWR = math.Round(d.ATL/d.CTL*100) / 100
		}
		days = append(days, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get pmc range: %w", err)
	}
	if days == nil {
		days = []model.PMCDay{}
	}
	return days, nil
}

// GetPMCRangeAllSports returns all pmc_daily rows for the given range, including
// sport-specific rows (sport_group != '__combined__'). Used by the split_by_sport
// API parameter to populate per-sport series in the frontend.
func (db *DB) GetPMCRangeAllSports(ctx context.Context, userID string, from, to time.Time) ([]model.PMCDay, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT date::text, sport_group, tss, ctl, atl, tsb, COALESCE(acwr_ra, 0) FROM pmc_daily
		 WHERE user_id = $1 AND date >= $2 AND date <= $3
		 ORDER BY sport_group ASC, date ASC`,
		userID, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("get pmc range all sports: %w", err)
	}
	defer rows.Close()

	var days []model.PMCDay
	for rows.Next() {
		var d model.PMCDay
		if err := rows.Scan(&d.Date, &d.SportGroup, &d.TSS, &d.CTL, &d.ATL, &d.TSB, &d.ACWRRA); err != nil {
			return nil, fmt.Errorf("scan pmc day all sports: %w", err)
		}
		if d.CTL > 0 {
			d.ACWR = math.Round(d.ATL/d.CTL*100) / 100
		}
		days = append(days, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get pmc range all sports: %w", err)
	}
	if days == nil {
		days = []model.PMCDay{}
	}
	return days, nil
}

func (db *DB) UpsertPMCDays(ctx context.Context, userID string, days []model.PMCDay) error {
	batch := &pgx.Batch{}
	for _, d := range days {
		sg := d.SportGroup
		if sg == "" {
			sg = "__combined__"
		}
		batch.Queue(
			`INSERT INTO pmc_daily (user_id, date, sport_group, tss, ctl, atl, tsb, acwr_ra)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 ON CONFLICT (user_id, date, sport_group) DO UPDATE
			 SET tss = EXCLUDED.tss, ctl = EXCLUDED.ctl, atl = EXCLUDED.atl, tsb = EXCLUDED.tsb,
			     acwr_ra = EXCLUDED.acwr_ra`,
			userID, d.Date, sg, d.TSS, d.CTL, d.ATL, d.TSB, d.ACWRRA,
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

// DeterminePMCSportGroup maps a sport_type string to a coarse PMC sport group.
// PMC uses fewer buckets than the full sport taxonomy for statistical significance.
func DeterminePMCSportGroup(sportType string) string {
	key := sport.CategoryForSport(sportType).Key
	switch key {
	case "bike":
		return "cycling"
	case "run":
		return "running" // includes hiking, walking — PMC is intentionally coarser
	case "water":
		return "swimming"
	case "fitness":
		return "strength" // strength_training, fitness_equipment, hiit, yoga, pilates, etc.
	default:
		return "other"
	}
}

// GetDailyTSSBySport aggregates pre-computed TSS values per day and PMC sport group.
// Returns map[dateStr]map[sportGroup]totalTSS.
func (db *DB) GetDailyTSSBySport(ctx context.Context, userID string, from, to time.Time) (map[string]map[string]float64, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT date_trunc('day', a.started_at AT TIME ZONE u.timezone)::date AS day,
		       a.sport_type,
		       COALESCE(SUM(a.computed_tss), 0) AS tss
		FROM activities a
		JOIN users u ON u.id = a.user_id
		WHERE a.user_id = $1
		  AND a.started_at >= $2 AND a.started_at < $3
		  AND a.computed_tss IS NOT NULL AND a.computed_tss > 0
		  AND NOT a.exclude_from_stats
		  AND NOT a.is_group_secondary
		GROUP BY 1, 2
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get daily tss by sport: %w", err)
	}
	defer rows.Close()

	result := make(map[string]map[string]float64)
	for rows.Next() {
		var day time.Time
		var sportType string
		var tss float64
		if err := rows.Scan(&day, &sportType, &tss); err != nil {
			return nil, fmt.Errorf("scan daily tss by sport: %w", err)
		}
		dateStr := day.Format("2006-01-02")
		sg := DeterminePMCSportGroup(sportType)
		if result[dateStr] == nil {
			result[dateStr] = make(map[string]float64)
		}
		result[dateStr][sg] += tss
	}
	return result, rows.Err()
}

// GetDailyTSS aggregates pre-computed TSS values per day.
func (db *DB) GetDailyTSS(ctx context.Context, userID string, from, to time.Time) (map[string]float64, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT date_trunc('day', a.started_at AT TIME ZONE u.timezone)::date AS day,
		       COALESCE(SUM(a.computed_tss), 0) AS tss
		FROM activities a
		JOIN users u ON u.id = a.user_id
		WHERE a.user_id = $1
		  AND a.started_at >= $2 AND a.started_at < $3
		  AND a.computed_tss IS NOT NULL AND a.computed_tss > 0
		  AND NOT a.exclude_from_stats
		  AND NOT a.is_group_secondary
		GROUP BY 1
	`, userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var day time.Time
		var tss float64
		if err := rows.Scan(&day, &tss); err != nil {
			return nil, err
		}
		result[day.Format("2006-01-02")] = tss
	}
	return result, rows.Err()
}

// GetEarliestActivityDate returns the started_at of the user's oldest activity.
// Returns zero time if the user has no activities.
func (db *DB) GetEarliestActivityDate(ctx context.Context, userID string) (time.Time, error) {
	var t *time.Time
	err := db.Pool.QueryRow(ctx,
		`SELECT MIN(started_at) FROM activities WHERE user_id = $1`,
		userID,
	).Scan(&t)
	if errors.Is(err, pgx.ErrNoRows) || t == nil {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("get earliest activity date: %w", err)
	}
	return *t, nil
}

// GetPMCDayBefore returns the combined PMC values for the day before `date` for seeding calculations.
func (db *DB) GetPMCDayBefore(ctx context.Context, userID string, date time.Time) (ctl, atl float64, err error) {
	before := date.AddDate(0, 0, -1)
	err = db.Pool.QueryRow(ctx,
		`SELECT ctl, atl FROM pmc_daily WHERE user_id = $1 AND date = $2 AND sport_group = '__combined__'`,
		userID, before,
	).Scan(&ctl, &atl)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("get pmc day before: %w", err)
	}
	return ctl, atl, nil
}

// GetPMCSportCTLBefore returns per-sport CTL values for the day before `date`.
// Used to seed the per-sport EWMA in CalculateMultiSport.
// Returns an empty (non-nil) map if no rows exist.
func (db *DB) GetPMCSportCTLBefore(ctx context.Context, userID string, date time.Time) (map[string]float64, error) {
	before := date.AddDate(0, 0, -1)
	rows, err := db.Pool.Query(ctx,
		`SELECT sport_group, ctl FROM pmc_daily
		 WHERE user_id = $1 AND date = $2 AND sport_group != '__combined__'`,
		userID, before,
	)
	if err != nil {
		return nil, fmt.Errorf("get pmc sport ctl before: %w", err)
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var sg string
		var ctl float64
		if err := rows.Scan(&sg, &ctl); err != nil {
			return nil, fmt.Errorf("scan pmc sport ctl: %w", err)
		}
		result[sg] = ctl
	}
	return result, rows.Err()
}

// DeleteStalePMCSportRows removes per-sport pmc_daily rows (sport_group != '__combined__')
// for the given user within [from, to]. Called by RecalculatePMC before upserting the
// freshly computed sport-specific series to avoid stale rows for sport groups that no
// longer have activities (e.g. after a sport-type edit or activity deletion).
func (db *DB) DeleteStalePMCSportRows(ctx context.Context, userID string, from, to time.Time) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM pmc_daily
		 WHERE user_id = $1
		   AND date >= $2
		   AND date <= $3
		   AND sport_group != '__combined__'`,
		userID, from.Format("2006-01-02"), to.Format("2006-01-02"),
	)
	if err != nil {
		return fmt.Errorf("delete stale pmc sport rows: %w", err)
	}
	return nil
}
