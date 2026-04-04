package handler

import (
	"net/http"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
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

// GetRunningFormBaseline returns 90-day median running form metrics.
//
// GET /api/v1/running-form/baseline
func (h *Handler) GetRunningFormBaseline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	row := h.DB.Pool.QueryRow(ctx, `
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

	var baseline RunningFormBaseline
	if err := row.Scan(
		&baseline.DutyFactorMedian,
		&baseline.VerticalRatioMedian,
		&baseline.GCTAsymmetryMedian,
		&baseline.ActivityCount,
	); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, baseline)
}

// GetRunningFormTrend returns per-activity running form values as a time series.
//
// GET /api/v1/running-form/trend
// Optional query params: from=2025-01-01&to=2026-04-04
func (h *Handler) GetRunningFormTrend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()

	var from, to time.Time
	if v := q.Get("from"); v != "" {
		from, _ = time.Parse("2006-01-02", v)
	}
	if v := q.Get("to"); v != "" {
		to, _ = time.Parse("2006-01-02", v)
	}

	var (
		query string
		args  []any
	)

	if !from.IsZero() && !to.IsZero() {
		query = `
			SELECT id, started_at::date, running_form_duty_factor, running_form_vertical_ratio,
				running_form_gct_asymmetry_pct, running_form_tier
			FROM activities
			WHERE user_id = $1
				AND running_form_tier IS NOT NULL
				AND deleted_at IS NULL
				AND started_at >= $2
				AND started_at <= $3 + INTERVAL '1 day'
			ORDER BY started_at ASC`
		args = []any{userID, from, to}
	} else if !from.IsZero() {
		query = `
			SELECT id, started_at::date, running_form_duty_factor, running_form_vertical_ratio,
				running_form_gct_asymmetry_pct, running_form_tier
			FROM activities
			WHERE user_id = $1
				AND running_form_tier IS NOT NULL
				AND deleted_at IS NULL
				AND started_at >= $2
			ORDER BY started_at ASC`
		args = []any{userID, from}
	} else if !to.IsZero() {
		query = `
			SELECT id, started_at::date, running_form_duty_factor, running_form_vertical_ratio,
				running_form_gct_asymmetry_pct, running_form_tier
			FROM activities
			WHERE user_id = $1
				AND running_form_tier IS NOT NULL
				AND deleted_at IS NULL
				AND started_at <= $2 + INTERVAL '1 day'
			ORDER BY started_at ASC`
		args = []any{userID, to}
	} else {
		query = `
			SELECT id, started_at::date, running_form_duty_factor, running_form_vertical_ratio,
				running_form_gct_asymmetry_pct, running_form_tier
			FROM activities
			WHERE user_id = $1
				AND running_form_tier IS NOT NULL
				AND deleted_at IS NULL
			ORDER BY started_at ASC`
		args = []any{userID}
	}

	rows, err := h.DB.Pool.Query(ctx, query, args...)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
		return
	}
	defer rows.Close()

	points := make([]RunningFormPoint, 0)
	for rows.Next() {
		var p RunningFormPoint
		var date time.Time
		if err := rows.Scan(&p.ActivityID, &date, &p.DutyFactor, &p.VerticalRatio, &p.GCTAsymmetry, &p.Tier); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
			return
		}
		p.Date = date.Format("2006-01-02")
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"points": points,
	})
}
