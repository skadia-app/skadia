package analytics

import (
	"math"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// GetHRV returns HRV chart data with baseline statistics and anomaly detection.
func (h *Handler) GetHRV(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	filter := ParseFilters(r)

	points, err := h.DB.GetHRVAnalysis(r.Context(), userID, filter.From, filter.To)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "HRV Query Failed",
			err.Error(), r.URL.Path)
		return
	}

	// Collect non-nil RMSSD values.
	var rmssdVals []float64
	for _, p := range points {
		if p.RMSSD != nil {
			rmssdVals = append(rmssdVals, *p.RMSSD)
		}
	}

	baselineMean, baselineStd := meanStd(rmssdVals)

	// 7-day average: last 7 non-nil values.
	sevenDayAvg := tailMean(rmssdVals, 7)

	// Anomaly dates: RMSSD < mean - 2*std.
	threshold := baselineMean - 2*baselineStd
	var anomalies []string
	for _, p := range points {
		if p.RMSSD != nil && *p.RMSSD < threshold {
			anomalies = append(anomalies, p.Date)
		}
	}
	if anomalies == nil {
		anomalies = []string{}
	}

	type response struct {
		Points       []database.HRVPoint `json:"points"`
		BaselineMean float64             `json:"baseline_mean"`
		BaselineStd  float64             `json:"baseline_std"`
		SevenDayAvg  float64             `json:"seven_day_avg"`
		Anomalies    []string            `json:"anomalies"`
	}

	httputil.WriteJSON(w, http.StatusOK, response{
		Points:       points,
		BaselineMean: baselineMean,
		BaselineStd:  baselineStd,
		SevenDayAvg:  sevenDayAvg,
		Anomalies:    anomalies,
	})
}

// GetHRVCorrelation returns paired HRV/TSS data with a Pearson correlation coefficient.
func (h *Handler) GetHRVCorrelation(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	filter := ParseFilters(r)

	pts, err := h.DB.GetHRVCorrelation(r.Context(), userID, filter.From, filter.To)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "HRV Correlation Query Failed",
			err.Error(), r.URL.Path)
		return
	}

	corr := pearsonCorrelation(pts)
	interpretation := interpretCorrelation(corr)

	type response struct {
		Points         []database.HRVCorrelationPoint `json:"points"`
		Correlation    float64                        `json:"correlation"`
		Interpretation string                         `json:"interpretation"`
	}

	httputil.WriteJSON(w, http.StatusOK, response{
		Points:         pts,
		Correlation:    corr,
		Interpretation: interpretation,
	})
}

// GetHRVDFAAlpha1 returns raw R-R interval data for DFA-alpha1 analysis.
// Full DFA computation is deferred to the ML service (Phase 9).
// Returns 204 No Content when no R-R data exists for the activity.
func (h *Handler) GetHRVDFAAlpha1(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := r.URL.Query().Get("activity_id")
	if activityID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Missing Parameter",
			"activity_id query parameter is required", r.URL.Path)
		return
	}

	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil || activity == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}
	if activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	intervals, err := h.DB.GetRRIntervals(r.Context(), activityID)
	if err != nil {
		// activity_rr_intervals table may not exist yet.
		httputil.WriteError(w, http.StatusInternalServerError, "R-R Query Failed",
			err.Error(), r.URL.Path)
		return
	}

	if len(intervals) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	type response struct {
		ActivityID string  `json:"activity_id"`
		Count      int     `json:"count"`
		RRMs       []int16 `json:"rr_ms"`
		Note       string  `json:"note"`
	}

	httputil.WriteJSON(w, http.StatusOK, response{
		ActivityID: activityID,
		Count:      len(intervals),
		RRMs:       intervals,
		Note:       "Full DFA-alpha1 calculation is performed by the ML service.",
	})
}

// GetHRVRecovery computes a composite recovery score from HRV, sleep, and ATL.
func (h *Handler) GetHRVRecovery(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	// Last 7 days HRV trend.
	filter := ParseFilters(r)
	hrvPts, err := h.DB.GetHRVAnalysis(r.Context(), userID, filter.From, filter.To)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "HRV Query Failed",
			err.Error(), r.URL.Path)
		return
	}

	// Latest sleep.
	latestSleep, err := h.DB.GetLatestHealthSleep(r.Context(), userID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Sleep Query Failed",
			err.Error(), r.URL.Path)
		return
	}

	// Latest training load (ATL).
	latestLoad, err := h.DB.GetLatestHealthTrainingLoad(r.Context(), userID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Training Load Query Failed",
			err.Error(), r.URL.Path)
		return
	}

	// HRV factor (0–1): compare last 7-day average to overall baseline.
	var rmssdVals []float64
	for _, p := range hrvPts {
		if p.RMSSD != nil {
			rmssdVals = append(rmssdVals, *p.RMSSD)
		}
	}
	baselineMean, baselineStd := meanStd(rmssdVals)
	sevenDayAvg := tailMean(rmssdVals, 7)

	var hrvFactor float64
	if baselineMean > 0 {
		// Normalize: 1.0 = at baseline, 0.0 = at baseline - 2*std.
		low := baselineMean - 2*baselineStd
		if low < 0 {
			low = 0
		}
		hrvFactor = clamp((sevenDayAvg-low)/(baselineMean-low+1), 0, 1)
	}

	// Sleep factor (0–1): based on last night's score.
	var sleepFactor float64
	if latestSleep != nil && latestSleep.ScoreOverall != nil {
		sleepFactor = clamp(float64(*latestSleep.ScoreOverall)/100.0, 0, 1)
	}

	// Fatigue factor (0–1): inverse of normalized ATL.
	// ATL > 100 = high fatigue (factor near 0); ATL = 0 = no fatigue (factor = 1).
	var fatigueFactor float64 = 1.0
	if latestLoad != nil && latestLoad.ATLAcute != nil {
		atl := *latestLoad.ATLAcute
		// Normalize against a practical max of 150 TSS/day ATL.
		fatigueFactor = clamp(1.0-(atl/150.0), 0, 1)
	}

	// Composite score: HRV 40%, sleep 30%, fatigue 30%.
	compositeRaw := (hrvFactor*0.40 + sleepFactor*0.30 + fatigueFactor*0.30) * 100
	score := int(math.Round(compositeRaw))

	level, recommendation := recoveryLevel(score)

	type response struct {
		Score          int     `json:"score"`
		Level          string  `json:"level"`
		HRVFactor      float64 `json:"hrv_factor"`
		SleepFactor    float64 `json:"sleep_factor"`
		FatigueFactor  float64 `json:"fatigue_factor"`
		Recommendation string  `json:"recommendation"`
	}

	httputil.WriteJSON(w, http.StatusOK, response{
		Score:          score,
		Level:          level,
		HRVFactor:      round2(hrvFactor),
		SleepFactor:    round2(sleepFactor),
		FatigueFactor:  round2(fatigueFactor),
		Recommendation: recommendation,
	})
}

// GetSleep returns sleep analytics with 7-day rolling averages.
func (h *Handler) GetSleep(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	filter := ParseFilters(r)

	pts, err := h.DB.GetSleepAnalytics(r.Context(), userID, filter.From, filter.To)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Sleep Query Failed",
			err.Error(), r.URL.Path)
		return
	}

	// Average duration and score over the entire period.
	var totalDur, totalScore float64
	var cntDur, cntScore int
	for _, p := range pts {
		if p.DurationMin != nil {
			totalDur += float64(*p.DurationMin)
			cntDur++
		}
		if p.ScoreOverall != nil {
			totalScore += float64(*p.ScoreOverall)
			cntScore++
		}
	}
	var avgDur, avgScore float64
	if cntDur > 0 {
		avgDur = totalDur / float64(cntDur)
	}
	if cntScore > 0 {
		avgScore = totalScore / float64(cntScore)
	}

	// Simple trend: compare most recent 7 vs. previous 7 average duration.
	trend := sleepTrend(pts)

	type response struct {
		Points         []database.SleepAnalyticsPoint `json:"points"`
		AvgDurationMin float64                        `json:"avg_duration_min"`
		AvgScore       float64                        `json:"avg_score"`
		Trend          string                         `json:"trend"`
	}

	httputil.WriteJSON(w, http.StatusOK, response{
		Points:         pts,
		AvgDurationMin: round2(avgDur),
		AvgScore:       round2(avgScore),
		Trend:          trend,
	})
}

// GetWeight returns body weight analytics with 30-day change calculation.
func (h *Handler) GetWeight(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	filter := ParseFilters(r)

	pts, err := h.DB.GetWeightAnalytics(r.Context(), userID, filter.From, filter.To)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Weight Query Failed",
			err.Error(), r.URL.Path)
		return
	}

	if len(pts) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	currentKg := pts[len(pts)-1].WeightKg

	// 30-day change: difference between most recent and oldest entry in range.
	change30d := 0.0
	if len(pts) >= 2 {
		change30d = round2(currentKg - pts[0].WeightKg)
	}

	type response struct {
		Points    []database.WeightPoint `json:"points"`
		CurrentKg float64                `json:"current_kg"`
		Change30d float64                `json:"change_30d"`
	}

	httputil.WriteJSON(w, http.StatusOK, response{
		Points:    pts,
		CurrentKg: round2(currentKg),
		Change30d: change30d,
	})
}

// GetActivityGlucose returns per-activity average blood glucose values for scatter chart display.
// Returns an empty array (not 204) when no data exists, so the frontend can decide whether to render the chart.
func (h *Handler) GetActivityGlucose(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	filter := ParseFilters(r)

	points, err := h.DB.GetActivityGlucose(r.Context(), userID, filter.From, filter.To)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Glucose Query Failed",
			err.Error(), r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, points)
}

// GetHealthMetrics returns the health metrics grid (RHR, HRV, sleep, weight).
func (h *Handler) GetHealthMetrics(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	filter := ParseFilters(r)

	metrics, err := h.DB.GetHealthMetrics(r.Context(), userID, filter.From, filter.To)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Health Metrics Query Failed",
			err.Error(), r.URL.Path)
		return
	}

	type response struct {
		Metrics []database.HealthMetric `json:"metrics"`
	}

	httputil.WriteJSON(w, http.StatusOK, response{Metrics: metrics})
}

// ---- helper functions ----

// meanStd computes mean and population standard deviation of a float64 slice.
func meanStd(vals []float64) (mean, std float64) {
	if len(vals) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	mean = sum / float64(len(vals))
	var variance float64
	for _, v := range vals {
		d := v - mean
		variance += d * d
	}
	variance /= float64(len(vals))
	std = math.Sqrt(variance)
	return
}

// tailMean returns the mean of the last n values in vals (or all values if fewer than n).
func tailMean(vals []float64, n int) float64 {
	if len(vals) == 0 {
		return 0
	}
	start := max(len(vals)-n, 0)
	tail := vals[start:]
	var sum float64
	for _, v := range tail {
		sum += v
	}
	return sum / float64(len(tail))
}

// pearsonCorrelation computes the Pearson r between RMSSD and DailyTSS.
func pearsonCorrelation(pts []database.HRVCorrelationPoint) float64 {
	n := len(pts)
	if n < 2 {
		return 0
	}
	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for _, p := range pts {
		x, y := p.RMSSD, p.DailyTSS
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
		sumY2 += y * y
	}
	fn := float64(n)
	num := fn*sumXY - sumX*sumY
	den := math.Sqrt((fn*sumX2 - sumX*sumX) * (fn*sumY2 - sumY*sumY))
	if den == 0 {
		return 0
	}
	return round2(num / den)
}

// interpretCorrelation maps a Pearson r value to a human-readable label.
func interpretCorrelation(r float64) string {
	abs := math.Abs(r)
	dir := "positive"
	if r < 0 {
		dir = "negative"
	}
	switch {
	case abs >= 0.7:
		return "strong " + dir + " correlation"
	case abs >= 0.4:
		return "moderate " + dir + " correlation"
	case abs >= 0.2:
		return "weak " + dir + " correlation"
	default:
		return "no meaningful correlation"
	}
}

// recoveryLevel converts a composite score (0–100) to a named level and recommendation.
func recoveryLevel(score int) (string, string) {
	switch {
	case score >= 80:
		return "excellent", "Sehr gute Erholung. Intensive Trainingseinheiten sind heute problemlos möglich."
	case score >= 60:
		return "good", "Gute Erholung. Moderate bis intensive Einheiten sind geeignet."
	case score >= 40:
		return "moderate", "Leichte Ermüdung erkannt. Eine lockere Einheit oder Tempolauf empfehlenswert."
	case score >= 20:
		return "low", "Erholung unter Durchschnitt. Einen ruhigen Tag oder Pause einplanen."
	default:
		return "very_low", "Erhebliche Ermüdung oder schlechter Schlaf. Ruhetag oder aktive Erholung empfohlen."
	}
}

// sleepTrend compares the last 7 nights to the prior 7 nights by average duration.
func sleepTrend(pts []database.SleepAnalyticsPoint) string {
	n := len(pts)
	if n < 2 {
		return "stable"
	}
	var sumRecent, sumPrior float64
	var cntRecent, cntPrior int
	for i, p := range pts {
		if p.DurationMin == nil {
			continue
		}
		v := float64(*p.DurationMin)
		if i >= n-7 {
			sumRecent += v
			cntRecent++
		} else if i >= n-14 {
			sumPrior += v
			cntPrior++
		}
	}
	if cntRecent == 0 || cntPrior == 0 {
		return "stable"
	}
	avgR := sumRecent / float64(cntRecent)
	avgP := sumPrior / float64(cntPrior)
	diff := avgR - avgP
	switch {
	case diff > 15:
		return "improving"
	case diff < -15:
		return "declining"
	default:
		return "stable"
	}
}

// clamp restricts v to [lo, hi].
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// round2 rounds a float64 to 2 decimal places.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
