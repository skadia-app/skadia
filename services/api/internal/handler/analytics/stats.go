package analytics

import (
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// ---------------------------------------------------------------------------
// Helper: Eddington number calculation
// ---------------------------------------------------------------------------

// calculateEddington returns the Eddington number for a slice of values that
// are already sorted in descending order.
// The Eddington number E is the largest integer such that at least E values
// are >= E.
func calculateEddington(values []float64) int {
	e := 0
	for i, v := range values {
		if v >= float64(i+1) {
			e = i + 1
		} else {
			break
		}
	}
	return e
}

// ---------------------------------------------------------------------------
// Helper: streak calculation
// ---------------------------------------------------------------------------

// streakPeriod represents a contiguous streak interval.
type streakPeriod struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count"`
}

// calculateDayStreaks computes current and max day streaks from a sorted
// (ascending) slice of distinct active dates.
// "current" streak counts backwards from today (or yesterday if today has no
// activity yet). Weeks are counted separately.
func calculateDayStreaks(days []time.Time) (currentStreak, maxStreak int, history []streakPeriod) {
	if len(days) == 0 {
		return 0, 0, []streakPeriod{}
	}

	today := time.Now().Truncate(24 * time.Hour)

	// Build contiguous-day streak segments.
	var segments []streakPeriod
	segStart := days[0]
	segEnd := days[0]
	segLen := 1

	flush := func() {
		if segLen > 0 {
			segments = append(segments, streakPeriod{
				From:  segStart.Format("2006-01-02"),
				To:    segEnd.Format("2006-01-02"),
				Count: segLen,
			})
		}
	}

	for i := 1; i < len(days); i++ {
		prev := days[i-1].Truncate(24 * time.Hour)
		curr := days[i].Truncate(24 * time.Hour)
		if curr.Sub(prev) == 24*time.Hour {
			segEnd = curr
			segLen++
		} else {
			flush()
			segStart = curr
			segEnd = curr
			segLen = 1
		}
	}
	flush()

	// Max streak is the longest segment.
	for _, seg := range segments {
		if seg.Count > maxStreak {
			maxStreak = seg.Count
		}
	}

	// Current streak: the last segment must end on today or yesterday.
	if len(segments) > 0 {
		last := segments[len(segments)-1]
		lastDate, _ := time.Parse("2006-01-02", last.To)
		lastDate = lastDate.Truncate(24 * time.Hour)
		yesterday := today.AddDate(0, 0, -1)
		if lastDate.Equal(today) || lastDate.Equal(yesterday) {
			currentStreak = last.Count
		}
	}

	// Return last 20 streak segments for chart display.
	history = segments
	if len(history) > 20 {
		history = history[len(history)-20:]
	}

	return currentStreak, maxStreak, history
}

// calculateWeekStreaks computes current and max consecutive-week streaks from
// the same sorted active-day slice.
// A week is considered "active" if it contains at least one active day.
// Weeks are ISO weeks (Monday–Sunday).
func calculateWeekStreaks(days []time.Time) (currentWeekStreak, maxWeekStreak int) {
	if len(days) == 0 {
		return 0, 0
	}

	// Collect distinct ISO year+week numbers.
	type yearWeek struct{ year, week int }
	seen := map[yearWeek]bool{}
	for _, d := range days {
		year, week := d.ISOWeek()
		seen[yearWeek{year, week}] = true
	}

	// Sort the unique weeks.
	var weeks []yearWeek
	for yw := range seen {
		weeks = append(weeks, yw)
	}
	sort.Slice(weeks, func(i, j int) bool {
		if weeks[i].year != weeks[j].year {
			return weeks[i].year < weeks[j].year
		}
		return weeks[i].week < weeks[j].week
	})

	// Count streaks of consecutive weeks.
	cur := 1
	max := 1
	for i := 1; i < len(weeks); i++ {
		prev := weeks[i-1]
		curr := weeks[i]
		// Consecutive: same year, next week — or wrap from week 52/53 to 1.
		consecutive := false
		if curr.year == prev.year && curr.week == prev.week+1 {
			consecutive = true
		} else if curr.year == prev.year+1 && prev.week >= 52 && curr.week == 1 {
			consecutive = true
		}
		if consecutive {
			cur++
			if cur > max {
				max = cur
			}
		} else {
			cur = 1
		}
	}

	maxWeekStreak = max

	// Current week streak: check if last week in list is this week or last week.
	thisYear, thisWeek := time.Now().ISOWeek()
	lastYear, _ := time.Now().AddDate(0, 0, -7).ISOWeek()
	lastWeekNum := weeks[len(weeks)-1]
	if (lastWeekNum.year == thisYear && lastWeekNum.week == thisWeek) ||
		(lastWeekNum.year == lastYear) {
		currentWeekStreak = cur
	}

	return currentWeekStreak, maxWeekStreak
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// lifetimeResponse is the JSON response for GET /lifetime-stats.
type lifetimeResponse struct {
	TotalActivities   int64   `json:"total_activities"`
	TotalDistanceM    float64 `json:"total_distance_m"`
	TotalElevationM   float64 `json:"total_elevation_m"`
	TotalMovingTimeS  int64   `json:"total_moving_time_s"`
	TotalElapsedTimeS int64   `json:"total_elapsed_time_s"`
	TotalCalories     int64   `json:"total_calories"`
	ActiveDays        int64   `json:"active_days"`
	Countries         int64   `json:"countries"`
	// Fun metrics
	EarthLaps    float64 `json:"earth_laps"`
	MoonTrips    float64 `json:"moon_trips"`
	EverestCount float64 `json:"everest_count"`
	// Streaks
	CurrentDayStreak int `json:"current_day_streak"`
	MaxDayStreak     int `json:"max_day_streak"`
}

func (h *Handler) GetLifetimeStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "missing user identity", r.URL.Path)
		return
	}

	filter := ParseFilters(r)
	whereClause, args := filter.BuildWhereClause(2)

	stats, err := h.DB.GetLifetimeStats(ctx, userID, whereClause, args)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
		return
	}

	// Streak data (no filters — always lifetime).
	activeDays, err := h.DB.GetActiveDays(ctx, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
		return
	}
	currentStreak, maxStreak, _ := calculateDayStreaks(activeDays)

	resp := lifetimeResponse{
		TotalActivities:   stats.TotalActivities,
		TotalDistanceM:    stats.TotalDistanceM,
		TotalElevationM:   stats.TotalElevationM,
		TotalMovingTimeS:  stats.TotalMovingTimeS,
		TotalElapsedTimeS: stats.TotalElapsedTimeS,
		TotalCalories:     stats.TotalCalories,
		ActiveDays:        stats.ActiveDays,
		Countries:         stats.Countries,
		EarthLaps:         stats.TotalDistanceM / 40_075_000,
		MoonTrips:         stats.TotalDistanceM / 384_400_000,
		EverestCount:      stats.TotalElevationM / 8849,
		CurrentDayStreak:  currentStreak,
		MaxDayStreak:      maxStreak,
	}

	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Eddington
// ---------------------------------------------------------------------------

type eddingtonNextLevel struct {
	Level     int `json:"level"`
	Remaining int `json:"remaining"`
}

type eddingtonResponse struct {
	Metric     string               `json:"metric"`
	Number     int                  `json:"number"`
	NextLevels []eddingtonNextLevel `json:"next_levels"`
	TopValues  []float64            `json:"top_values"`
}

func (h *Handler) GetEddington(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "missing user identity", r.URL.Path)
		return
	}

	metric := r.URL.Query().Get("metric")
	if metric == "" {
		metric = "km"
	}

	filter := ParseFilters(r)
	whereClause, args := filter.BuildWhereClause(2)

	values, err := h.DB.GetEddingtonData(ctx, userID, metric, whereClause, args)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
		return
	}

	// values are already sorted descending by the DB query.
	e := calculateEddington(values)

	// Compute how many days are needed to reach the next N levels.
	targetOffsets := []int{1, 2, 3, 5, 10}
	var nextLevels []eddingtonNextLevel
	for _, offset := range targetOffsets {
		target := e + offset
		// We need `target` days each with value >= target.
		// Count how many existing days already satisfy value >= target.
		have := 0
		for _, v := range values {
			if v >= float64(target) {
				have++
			} else {
				break // sorted descending
			}
		}
		remaining := max(target-have, 0)
		nextLevels = append(nextLevels, eddingtonNextLevel{
			Level:     target,
			Remaining: remaining,
		})
	}

	// Top 50 values for bar chart visualization.
	topValues := values
	if len(topValues) > 50 {
		topValues = topValues[:50]
	}

	writeJSON(w, http.StatusOK, eddingtonResponse{
		Metric:     metric,
		Number:     e,
		NextLevels: nextLevels,
		TopValues:  topValues,
	})
}

// ---------------------------------------------------------------------------
// Distributions
// ---------------------------------------------------------------------------

func (h *Handler) GetDistributions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "missing user identity", r.URL.Path)
		return
	}

	distType := r.URL.Query().Get("type")
	if distType == "" {
		distType = "sport"
	}
	metric := r.URL.Query().Get("metric")
	if metric == "" {
		metric = "count"
	}

	filter := ParseFilters(r)
	whereClause, args := filter.BuildWhereClause(2)

	rows, err := h.DB.GetDistribution(ctx, userID, distType, metric, whereClause, args)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"type":   distType,
		"metric": metric,
		"rows":   rows,
	})
}

// ---------------------------------------------------------------------------
// Yearly overview
// ---------------------------------------------------------------------------

func (h *Handler) GetYearlyOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "missing user identity", r.URL.Path)
		return
	}

	rows, err := h.DB.GetYearlyOverview(ctx, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"rows": rows,
	})
}

// ---------------------------------------------------------------------------
// Streaks
// ---------------------------------------------------------------------------

type streakResponse struct {
	CurrentStreak     int            `json:"current_streak"`
	MaxStreak         int            `json:"max_streak"`
	CurrentWeekStreak int            `json:"current_week_streak"`
	MaxWeekStreak     int            `json:"max_week_streak"`
	StreakHistory     []streakPeriod `json:"streak_history"`
}

func (h *Handler) GetStreaks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "missing user identity", r.URL.Path)
		return
	}

	days, err := h.DB.GetActiveDays(ctx, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
		return
	}

	currentStreak, maxStreak, history := calculateDayStreaks(days)
	currentWeekStreak, maxWeekStreak := calculateWeekStreaks(days)

	writeJSON(w, http.StatusOK, streakResponse{
		CurrentStreak:     currentStreak,
		MaxStreak:         maxStreak,
		CurrentWeekStreak: currentWeekStreak,
		MaxWeekStreak:     maxWeekStreak,
		StreakHistory:     history,
	})
}

// Ensure math import is used (referenced in potential future helpers).
var _ = math.Round
