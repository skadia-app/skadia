package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/bestefforts"
)

// BestEffort represents a single computed best effort record.
type BestEffort struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	ActivityID     string     `json:"activity_id"`
	Category       string     `json:"category"`
	SportGroup     string     `json:"sport_group"`
	ElapsedTimeS   *int       `json:"elapsed_time_s,omitempty"`
	ValueNumeric   *float64   `json:"value_numeric,omitempty"`
	AchievedAt     time.Time  `json:"achieved_at"`
	StreamStartIdx *int       `json:"stream_start_idx,omitempty"`
	StreamEndIdx   *int       `json:"stream_end_idx,omitempty"`
}

// BestEffortRanked is a BestEffort with a rank within the user's sport group and category.
type BestEffortRanked struct {
	BestEffort
	Rank int `json:"rank"`
}

// UpsertBestEffort inserts or updates a best effort record.
// The conflict target is (activity_id, category, sport_group).
func (db *DB) UpsertBestEffort(ctx context.Context, be BestEffort) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO best_efforts
		    (id, user_id, activity_id, category, sport_group,
		     elapsed_time_s, value_numeric, achieved_at,
		     stream_start_idx, stream_end_idx)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (activity_id, category, sport_group) DO UPDATE SET
		     user_id         = EXCLUDED.user_id,
		     elapsed_time_s  = EXCLUDED.elapsed_time_s,
		     value_numeric   = EXCLUDED.value_numeric,
		     achieved_at     = EXCLUDED.achieved_at,
		     stream_start_idx = EXCLUDED.stream_start_idx,
		     stream_end_idx  = EXCLUDED.stream_end_idx`,
		be.ID, be.UserID, be.ActivityID, be.Category, be.SportGroup,
		be.ElapsedTimeS, be.ValueNumeric, be.AchievedAt,
		be.StreamStartIdx, be.StreamEndIdx,
	)
	if err != nil {
		return fmt.Errorf("upsert best effort: %w", err)
	}
	return nil
}

// DeleteBestEffortsForActivity removes all best effort records for a single activity.
func (db *DB) DeleteBestEffortsForActivity(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM best_efforts WHERE activity_id = $1`,
		activityID,
	)
	if err != nil {
		return fmt.Errorf("delete best efforts for activity: %w", err)
	}
	return nil
}

// DeleteAllBestEfforts removes all best effort records for a user (for full recalc).
func (db *DB) DeleteAllBestEfforts(ctx context.Context, userID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM best_efforts WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("delete all best efforts: %w", err)
	}
	return nil
}

// rankingOrderExpr is the ORDER BY clause shared by all ranking queries.
// TypeTimeLower categories have elapsed_time_s set, TypeValueHigher have value_numeric.
const rankingOrderExpr = `
	CASE WHEN be.elapsed_time_s IS NOT NULL THEN be.elapsed_time_s END ASC NULLS LAST,
	CASE WHEN be.value_numeric  IS NOT NULL THEN be.value_numeric  END DESC NULLS LAST`

// GetBestEffortsForActivity returns all best effort records for an activity,
// each annotated with the user's rank (top-3 only) among eligible activities.
//
// Eligibility guards applied to ranked activities:
//   - a.deleted_at IS NULL
//   - NOT COALESCE(a.exclude_from_stats, false)
//   - NOT COALESCE(a.is_group_secondary, false)
//   - No active best_effort_exclusions for (user, activity, category OR '__all__')
//
// Only ranks 1–3 are returned; records outside the top-3 receive rank > 3.
func (db *DB) GetBestEffortsForActivity(ctx context.Context, activityID, userID string) ([]BestEffortRanked, error) {
	rows, err := db.Pool.Query(ctx,
		`WITH ranked AS (
		    SELECT
		        be.id,
		        be.user_id,
		        be.activity_id,
		        be.category,
		        be.sport_group,
		        be.elapsed_time_s,
		        be.value_numeric,
		        be.achieved_at,
		        be.stream_start_idx,
		        be.stream_end_idx,
		        RANK() OVER (
		            PARTITION BY be.user_id, be.sport_group, be.category
		            ORDER BY `+rankingOrderExpr+`
		        ) AS rank
		    FROM best_efforts be
		    JOIN activities a ON a.id = be.activity_id
		    WHERE be.user_id = $2
		      AND a.deleted_at IS NULL
		      AND NOT COALESCE(a.exclude_from_stats, false)
		      AND NOT COALESCE(a.is_group_secondary, false)
		      AND NOT EXISTS (
		          SELECT 1 FROM best_effort_exclusions exc
		          WHERE exc.user_id    = be.user_id
		            AND exc.activity_id = be.activity_id
		            AND (exc.category  = '__all__' OR exc.category = be.category)
		      )
		)
		SELECT
		    id, user_id, activity_id, category, sport_group,
		    elapsed_time_s, value_numeric, achieved_at,
		    stream_start_idx, stream_end_idx, rank
		FROM ranked
		WHERE activity_id = $1
		  AND rank <= 3
		ORDER BY category, rank`,
		activityID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get best efforts for activity: %w", err)
	}
	defer rows.Close()

	return scanBestEffortRanked(rows)
}

// GetBestEffortPersonalRecords returns the top-N best effort records for a user
// across all activities, optionally filtered by sport group and time window.
//
// sportGroup may be empty to include all sport groups.
// achievedAfter / achievedBefore may be nil to disable the time filter.
func (db *DB) GetBestEffortPersonalRecords(
	ctx context.Context,
	userID, sportGroup string,
	topN int,
	achievedAfter, achievedBefore *time.Time,
) ([]BestEffortRanked, error) {
	rows, err := db.Pool.Query(ctx,
		`WITH ranked AS (
		    SELECT
		        be.id,
		        be.user_id,
		        be.activity_id,
		        be.category,
		        be.sport_group,
		        be.elapsed_time_s,
		        be.value_numeric,
		        be.achieved_at,
		        be.stream_start_idx,
		        be.stream_end_idx,
		        RANK() OVER (
		            PARTITION BY be.user_id, be.sport_group, be.category
		            ORDER BY `+rankingOrderExpr+`
		        ) AS rank
		    FROM best_efforts be
		    JOIN activities a ON a.id = be.activity_id
		    WHERE be.user_id = $1
		      AND ($2::text = '' OR be.sport_group = $2)
		      AND ($4::timestamptz IS NULL OR be.achieved_at >= $4)
		      AND ($5::timestamptz IS NULL OR be.achieved_at <  $5)
		      AND a.deleted_at IS NULL
		      AND NOT COALESCE(a.exclude_from_stats, false)
		      AND NOT COALESCE(a.is_group_secondary, false)
		      AND NOT EXISTS (
		          SELECT 1 FROM best_effort_exclusions exc
		          WHERE exc.user_id     = be.user_id
		            AND exc.activity_id = be.activity_id
		            AND (exc.category   = '__all__' OR exc.category = be.category)
		      )
		)
		SELECT
		    id, user_id, activity_id, category, sport_group,
		    elapsed_time_s, value_numeric, achieved_at,
		    stream_start_idx, stream_end_idx, rank
		FROM ranked
		WHERE rank <= $3
		ORDER BY sport_group, category, rank`,
		userID, sportGroup, topN, achievedAfter, achievedBefore,
	)
	if err != nil {
		return nil, fmt.Errorf("get personal records: %w", err)
	}
	defer rows.Close()

	return scanBestEffortRanked(rows)
}

// UpsertBestEffortExclusion marks an activity's best effort (or all efforts if
// category is "__all__") as excluded from ranking for the given user.
// Duplicate inserts are silently ignored.
func (db *DB) UpsertBestEffortExclusion(ctx context.Context, userID, activityID, category string) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO best_effort_exclusions (user_id, activity_id, category)
		 VALUES ($1, $2, $3)
		 ON CONFLICT DO NOTHING`,
		userID, activityID, category,
	)
	if err != nil {
		return fmt.Errorf("upsert best effort exclusion: %w", err)
	}
	return nil
}

// BestEffortExclusion represents a single exclusion record for the authenticated user.
type BestEffortExclusion struct {
	ActivityID string    `json:"activity_id"`
	Category   string    `json:"category"`
	CreatedAt  time.Time `json:"created_at"`
}

// GetBestEffortExclusions returns all exclusion records for a user, newest first.
func (db *DB) GetBestEffortExclusions(ctx context.Context, userID string) ([]BestEffortExclusion, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT activity_id, category, created_at
		 FROM best_effort_exclusions
		 WHERE user_id = $1
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get best effort exclusions: %w", err)
	}
	defer rows.Close()

	var results []BestEffortExclusion
	for rows.Next() {
		var e BestEffortExclusion
		if err := rows.Scan(&e.ActivityID, &e.Category, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan best effort exclusion: %w", err)
		}
		results = append(results, e)
	}
	if results == nil {
		results = []BestEffortExclusion{}
	}
	return results, rows.Err()
}

// DeleteBestEffortExclusion removes an exclusion so the effort is ranked again.
func (db *DB) DeleteBestEffortExclusion(ctx context.Context, userID, activityID, category string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM best_effort_exclusions
		 WHERE user_id = $1 AND activity_id = $2 AND category = $3`,
		userID, activityID, category,
	)
	if err != nil {
		return fmt.Errorf("delete best effort exclusion: %w", err)
	}
	return nil
}

// GetBestEffortRecalcIDs returns the IDs of all activities eligible for best
// effort computation for a user, in chronological order (started_at ASC).
//
// Excluded activities: deleted, excluded_from_stats, group secondaries, fusion sources.
func (db *DB) GetBestEffortRecalcIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id FROM activities
		 WHERE user_id = $1
		   AND deleted_at IS NULL
		   AND NOT COALESCE(exclude_from_stats, false)
		   AND NOT COALESCE(is_group_secondary, false)
		   AND NOT COALESCE(is_fusion_source, false)
		 ORDER BY started_at ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get best effort recalc ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan recalc id: %w", err)
		}
		ids = append(ids, id)
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, rows.Err()
}

// GetBestEffortActivityInfo loads the activity metadata required to compute
// best efforts for a single activity.
func (db *DB) GetBestEffortActivityInfo(ctx context.Context, activityID string) (*bestefforts.ActivityInfo, error) {
	row := db.Pool.QueryRow(ctx,
		`SELECT
		     a.id,
		     a.user_id,
		     a.sport_type,
		     COALESCE(a.sub_type, ''),
		     COALESCE(a.is_indoor, false),
		     COALESCE(a.is_virtual, false),
		     COALESCE(a.is_electric, false),
		     a.started_at,
		     COALESCE(a.distance_m, 0),
		     COALESCE(a.elevation_gain_m, 0),
		     COALESCE(a.elapsed_time_s, 0)
		 FROM activities a
		 WHERE a.id = $1
		   AND a.deleted_at IS NULL
		   AND NOT COALESCE(a.exclude_from_stats, false)
		   AND NOT COALESCE(a.is_group_secondary, false)
		   AND NOT COALESCE(a.is_fusion_source, false)`,
		activityID,
	)
	var info bestefforts.ActivityInfo
	err := row.Scan(
		&info.ID,
		&info.UserID,
		&info.SportType,
		&info.SubType,
		&info.IsIndoor,
		&info.IsVirtual,
		&info.IsElectric,
		&info.StartedAt,
		&info.DistanceM,
		&info.ElevationGainM,
		&info.ElapsedTimeS,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get best effort activity info: %w", err)
	}
	return &info, nil
}

// ---- bestefforts.Store adapter methods ----
// These thin wrappers allow *DB to satisfy the bestefforts.Store interface
// without importing the bestefforts package from service.go (which would
// create an import cycle, since this file already imports bestefforts).

// UpsertBestEffortRec converts a bestefforts.BestEffortRecord to a BestEffort
// and delegates to UpsertBestEffort. This satisfies bestefforts.Store.
func (db *DB) UpsertBestEffortRec(ctx context.Context, be bestefforts.BestEffortRecord) error {
	return db.UpsertBestEffort(ctx, BestEffort{
		ID:             be.ID,
		UserID:         be.UserID,
		ActivityID:     be.ActivityID,
		Category:       be.Category,
		SportGroup:     be.SportGroup,
		ElapsedTimeS:   be.ElapsedTimeS,
		ValueNumeric:   be.ValueNumeric,
		AchievedAt:     be.AchievedAt,
		StreamStartIdx: be.StreamStartIdx,
		StreamEndIdx:   be.StreamEndIdx,
	})
}

// GetBestEffortsForActivityRec returns ranked best efforts as bestefforts.BestEffortRankedRecord
// slices, satisfying bestefforts.Store without a direct import from service.go.
func (db *DB) GetBestEffortsForActivityRec(ctx context.Context, activityID, userID string) ([]bestefforts.BestEffortRankedRecord, error) {
	ranked, err := db.GetBestEffortsForActivity(ctx, activityID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]bestefforts.BestEffortRankedRecord, len(ranked))
	for i, r := range ranked {
		out[i] = bestefforts.BestEffortRankedRecord{
			BestEffortRecord: bestefforts.BestEffortRecord{
				ID:             r.ID,
				UserID:         r.UserID,
				ActivityID:     r.ActivityID,
				Category:       r.Category,
				SportGroup:     r.SportGroup,
				ElapsedTimeS:   r.ElapsedTimeS,
				ValueNumeric:   r.ValueNumeric,
				AchievedAt:     r.AchievedAt,
				StreamStartIdx: r.StreamStartIdx,
				StreamEndIdx:   r.StreamEndIdx,
			},
			Rank: r.Rank,
		}
	}
	return out, nil
}

// NearMiss describes a best effort from an activity that narrowly missed the
// user's current personal record for that category.
type NearMiss struct {
	Category    string   `json:"category"`
	SportGroup  string   `json:"sport_group"`
	DiffSeconds int      `json:"diff_seconds,omitempty"` // for time-based PRs (dist / hm)
	DiffWatts   float64  `json:"diff_watts,omitempty"`   // for power PRs
	PRTimeS     *int     `json:"pr_time_s,omitempty"`
	PRWatts     *float64 `json:"pr_watts,omitempty"`
}

// GetNearMisses returns the efforts from activityID that almost beat the user's
// current PR (rank=1) for each category / sport-group combination.
//
// Near-miss thresholds:
//   - Time-based: effort is within LEAST(pr_time * 0.02, 30) seconds of the PR.
//   - Power-based: effort is within pr_watts * 0.02 watts below the PR.
//
// The same eligibility guards used in GetBestEffortsForActivity apply to the
// pool of ranked activities that determine the current PR.
func (db *DB) GetNearMisses(ctx context.Context, activityID, userID string) ([]NearMiss, error) {
	rows, err := db.Pool.Query(ctx,
		`WITH current_prs AS (
		    -- Rank-1 effort per (user, category, sport_group), excluding the
		    -- target activity so that an activity cannot be compared with itself.
		    SELECT DISTINCT ON (be.category, be.sport_group)
		        be.category,
		        be.sport_group,
		        be.elapsed_time_s  AS pr_time_s,
		        be.value_numeric   AS pr_watts
		    FROM best_efforts be
		    JOIN activities a ON a.id = be.activity_id
		    WHERE be.user_id = $2
		      AND be.activity_id <> $1
		      AND a.deleted_at IS NULL
		      AND NOT COALESCE(a.exclude_from_stats, false)
		      AND NOT COALESCE(a.is_group_secondary, false)
		      AND NOT EXISTS (
		          SELECT 1 FROM best_effort_exclusions exc
		          WHERE exc.user_id     = be.user_id
		            AND exc.activity_id = be.activity_id
		            AND (exc.category   = '__all__' OR exc.category = be.category)
		      )
		    ORDER BY
		        be.category,
		        be.sport_group,
		        CASE WHEN be.elapsed_time_s IS NOT NULL THEN be.elapsed_time_s END ASC NULLS LAST,
		        CASE WHEN be.value_numeric  IS NOT NULL THEN be.value_numeric  END DESC NULLS LAST
		),
		my_efforts AS (
		    SELECT
		        be.category,
		        be.sport_group,
		        be.elapsed_time_s,
		        be.value_numeric
		    FROM best_efforts be
		    WHERE be.activity_id = $1
		      AND be.user_id     = $2
		      AND NOT EXISTS (
		          SELECT 1 FROM best_effort_exclusions exc
		          WHERE exc.user_id     = be.user_id
		            AND exc.activity_id = be.activity_id
		            AND (exc.category   = '__all__' OR exc.category = be.category)
		      )
		)
		SELECT
		    me.category,
		    me.sport_group,
		    -- time-based near-miss: elapsed_time_s set, effort is slower than PR but within threshold
		    CASE
		        WHEN me.elapsed_time_s IS NOT NULL
		             AND pr.pr_time_s   IS NOT NULL
		             AND me.elapsed_time_s > pr.pr_time_s
		             AND (me.elapsed_time_s - pr.pr_time_s) <= LEAST(pr.pr_time_s * 0.02, 30)
		        THEN (me.elapsed_time_s - pr.pr_time_s)
		    END AS diff_seconds,
		    -- power-based near-miss: value_numeric set, effort is lower than PR but within threshold
		    CASE
		        WHEN me.value_numeric IS NOT NULL
		             AND pr.pr_watts   IS NOT NULL
		             AND me.value_numeric < pr.pr_watts
		             AND (pr.pr_watts - me.value_numeric) <= pr.pr_watts * 0.02
		        THEN (pr.pr_watts - me.value_numeric)
		    END AS diff_watts,
		    pr.pr_time_s,
		    pr.pr_watts
		FROM my_efforts me
		JOIN current_prs pr
		    ON pr.category   = me.category
		   AND pr.sport_group = me.sport_group
		WHERE
		    -- time-based near-miss
		    (
		        me.elapsed_time_s IS NOT NULL
		        AND pr.pr_time_s   IS NOT NULL
		        AND me.elapsed_time_s > pr.pr_time_s
		        AND (me.elapsed_time_s - pr.pr_time_s) <= LEAST(pr.pr_time_s * 0.02, 30)
		    )
		    OR
		    -- power-based near-miss
		    (
		        me.value_numeric IS NOT NULL
		        AND pr.pr_watts   IS NOT NULL
		        AND me.value_numeric < pr.pr_watts
		        AND (pr.pr_watts - me.value_numeric) <= pr.pr_watts * 0.02
		    )
		ORDER BY me.category, me.sport_group`,
		activityID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get near misses: %w", err)
	}
	defer rows.Close()

	var results []NearMiss
	for rows.Next() {
		var nm NearMiss
		var diffSecondsRaw *int
		var diffWattsRaw *float64
		if err := rows.Scan(
			&nm.Category,
			&nm.SportGroup,
			&diffSecondsRaw,
			&diffWattsRaw,
			&nm.PRTimeS,
			&nm.PRWatts,
		); err != nil {
			return nil, fmt.Errorf("scan near miss: %w", err)
		}
		if diffSecondsRaw != nil {
			nm.DiffSeconds = *diffSecondsRaw
		}
		if diffWattsRaw != nil {
			nm.DiffWatts = *diffWattsRaw
		}
		results = append(results, nm)
	}
	if results == nil {
		results = []NearMiss{}
	}
	return results, rows.Err()
}

// scanBestEffortRanked is a shared helper that drains pgx rows into a slice of
// BestEffortRanked values. The caller is responsible for closing rows.
func scanBestEffortRanked(rows pgx.Rows) ([]BestEffortRanked, error) {
	var results []BestEffortRanked
	for rows.Next() {
		var r BestEffortRanked
		if err := rows.Scan(
			&r.ID,
			&r.UserID,
			&r.ActivityID,
			&r.Category,
			&r.SportGroup,
			&r.ElapsedTimeS,
			&r.ValueNumeric,
			&r.AchievedAt,
			&r.StreamStartIdx,
			&r.StreamEndIdx,
			&r.Rank,
		); err != nil {
			return nil, fmt.Errorf("scan best effort ranked: %w", err)
		}
		results = append(results, r)
	}
	if results == nil {
		results = []BestEffortRanked{}
	}
	return results, rows.Err()
}
