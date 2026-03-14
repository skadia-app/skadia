package analytics

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ---------------------------------------------------------------------------
// PMC
// ---------------------------------------------------------------------------

// PMCResponse is the full response for the Performance Management Chart endpoint.
type PMCResponse struct {
	Days       []model.PMCDay `json:"days"`
	ACWR       float64        `json:"acwr"`
	ACWRStatus string         `json:"acwr_status"` // "fresh"|"optimal"|"overreach"|"danger"
	RampRate   float64        `json:"ramp_rate"`
	Monotony   float64        `json:"monotony"`
	Strain     float64        `json:"strain"`
	Projection []model.PMCDay `json:"projection"`
}

func acwrStatus(acwr float64) string {
	switch {
	case acwr < 0.8:
		return "fresh"
	case acwr < 1.0:
		return "optimal"
	case acwr <= 1.3:
		return "overreach"
	default:
		return "danger"
	}
}

// calcMonotony computes Training Monotony for the last n days of TSS values.
// Returns 0 if the mean is 0 or fewer than 2 data points.
func calcMonotony(tssSlice []float64) float64 {
	n := len(tssSlice)
	if n < 2 {
		return 0
	}
	var sum float64
	for _, v := range tssSlice {
		sum += v
	}
	mean := sum / float64(n)
	if mean == 0 {
		return 0
	}
	var variance float64
	for _, v := range tssSlice {
		d := v - mean
		variance += d * d
	}
	stddev := math.Sqrt(variance / float64(n))
	if stddev == 0 {
		return 0
	}
	return mean / stddev
}

// linearProjection extrapolates CTL/ATL/TSB for the next `horizon` days using
// a simple linear regression on the last `window` PMC days (at most).
// Returns an empty slice if not enough data is available.
func linearProjection(days []model.PMCDay, window, horizon int) []model.PMCDay {
	if len(days) < 2 {
		return []model.PMCDay{}
	}
	// Take up to the last `window` days.
	start := max(len(days)-window, 0)
	slice := days[start:]
	n := float64(len(slice))

	// Fit linear regression for CTL over position index.
	var sumX, sumY, sumXY, sumX2 float64
	for i, d := range slice {
		x := float64(i)
		sumX += x
		sumY += d.CTL
		sumXY += x * d.CTL
		sumX2 += x * x
	}
	denom := n*sumX2 - sumX*sumX
	if denom == 0 {
		return []model.PMCDay{}
	}
	slope := (n*sumXY - sumX*sumY) / denom
	intercept := (sumY - slope*sumX) / n

	// Use last known day as anchor for date.
	lastDay := slice[len(slice)-1]
	lastDate, err := time.Parse("2006-01-02", lastDay.Date)
	if err != nil {
		return []model.PMCDay{}
	}

	projection := make([]model.PMCDay, horizon)
	baseIdx := float64(len(slice) - 1)
	for i := range horizon {
		x := baseIdx + float64(i+1)
		ctl := intercept + slope*x
		projection[i] = model.PMCDay{
			Date: lastDate.AddDate(0, 0, i+1).Format("2006-01-02"),
			CTL:  math.Max(0, ctl),
		}
	}
	return projection
}

func (h *Handler) GetPMC(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "missing user identity", r.URL.Path)
		return
	}

	filter := ParseFilters(r)

	days, err := h.DB.GetPMCRange(ctx, userID, filter.From, filter.To)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
		return
	}

	// Build response metrics from the loaded days.
	var currentACWR float64
	var currentCTL float64
	var ctl7DaysAgo float64

	// Collect last 7 TSS values for monotony / strain.
	var last7TSS []float64

	for i, d := range days {
		if i == len(days)-1 {
			currentACWR = d.ACWR
			currentCTL = d.CTL
		}
		if i == len(days)-8 {
			ctl7DaysAgo = d.CTL
		}
		if i >= len(days)-7 {
			last7TSS = append(last7TSS, d.TSS)
		}
	}

	// RampRate: change in CTL per week (CTL_now - CTL_7daysAgo).
	rampRate := currentCTL - ctl7DaysAgo

	// Monotony and Strain.
	monotony := calcMonotony(last7TSS)
	var weekTSS float64
	for _, v := range last7TSS {
		weekTSS += v
	}
	strain := weekTSS * monotony

	// Projection over next 28 days based on last 28 CTL values.
	projection := linearProjection(days, 28, 28)

	resp := PMCResponse{
		Days:       days,
		ACWR:       currentACWR,
		ACWRStatus: acwrStatus(currentACWR),
		RampRate:   math.Round(rampRate*100) / 100,
		Monotony:   math.Round(monotony*1000) / 1000,
		Strain:     math.Round(strain*10) / 10,
		Projection: projection,
	}

	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Year Progression
// ---------------------------------------------------------------------------

type YearProgressionYear struct {
	Year       int       `json:"year"`
	Monthly    []float64 `json:"monthly"`    // 12 values, index 0 = Jan
	Cumulative []float64 `json:"cumulative"` // 12 cumulative values
	Total      float64   `json:"total"`
}

type YearProgressionResponse struct {
	Metric   string                `json:"metric"`
	Years    []YearProgressionYear `json:"years"`
	BestYear int                   `json:"best_year"`
}

func (h *Handler) GetYearProgression(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "missing user identity", r.URL.Path)
		return
	}

	metric := r.URL.Query().Get("metric")
	if metric == "" {
		metric = "distance_m"
	}

	filter := ParseFilters(r)
	// $1 = userID (consumed in DB method). Filter args start at index 2.
	whereClause, filterArgs := filter.BuildWhereClause(2)

	rows, err := h.DB.GetYearProgression(ctx, userID, metric, whereClause, filterArgs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
		return
	}

	// Pivot rows into per-year structs.
	yearMap := make(map[int]*YearProgressionYear)
	for _, row := range rows {
		y, ok := yearMap[row.Year]
		if !ok {
			y = &YearProgressionYear{
				Year:       row.Year,
				Monthly:    make([]float64, 12),
				Cumulative: make([]float64, 12),
			}
			yearMap[row.Year] = y
		}
		if row.Month >= 1 && row.Month <= 12 {
			y.Monthly[row.Month-1] = row.Value
		}
	}

	// Compute cumulative and total, find best year.
	var yearsList []YearProgressionYear
	bestYear := 0
	bestTotal := -1.0
	for _, y := range yearMap {
		var cum float64
		for i, v := range y.Monthly {
			cum += v
			y.Cumulative[i] = math.Round(cum*100) / 100
		}
		y.Total = math.Round(cum*100) / 100
		if y.Total > bestTotal {
			bestTotal = y.Total
			bestYear = y.Year
		}
		yearsList = append(yearsList, *y)
	}

	// Sort years ascending.
	for i := 1; i < len(yearsList); i++ {
		for j := i; j > 0 && yearsList[j].Year < yearsList[j-1].Year; j-- {
			yearsList[j], yearsList[j-1] = yearsList[j-1], yearsList[j]
		}
	}

	if yearsList == nil {
		yearsList = []YearProgressionYear{}
	}

	resp := YearProgressionResponse{
		Metric:   metric,
		Years:    yearsList,
		BestYear: bestYear,
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Training Volume
// ---------------------------------------------------------------------------

type TrainingVolumePeriod struct {
	Period  string             `json:"period"`
	BySport map[string]float64 `json:"by_sport"`
	Total   float64            `json:"total"`
}

type TrainingVolumeResponse struct {
	Metric      string                 `json:"metric"`
	Granularity string                 `json:"granularity"`
	Periods     []TrainingVolumePeriod `json:"periods"`
}

func (h *Handler) GetTrainingVolume(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "missing user identity", r.URL.Path)
		return
	}

	granularity := r.URL.Query().Get("granularity")
	if granularity == "" {
		granularity = "week"
	}
	metric := r.URL.Query().Get("metric")
	if metric == "" {
		metric = "distance_m"
	}

	filter := ParseFilters(r)
	// $1=userID in DB method. Filter args start at $2.
	whereClause, filterArgs := filter.BuildWhereClause(2)

	rows, err := h.DB.GetTrainingVolume(ctx, userID, granularity, metric, whereClause, filterArgs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
		return
	}

	// Pivot into period-keyed map.
	type periodEntry struct {
		period  string
		bySport map[string]float64
	}
	var periodOrder []string
	periodMap := make(map[string]map[string]float64)

	for _, row := range rows {
		key := row.Period.Format("2006-01-02")
		if _, ok := periodMap[key]; !ok {
			periodMap[key] = make(map[string]float64)
			periodOrder = append(periodOrder, key)
		}
		periodMap[key][row.SportType] += row.Value
	}

	periods := make([]TrainingVolumePeriod, 0, len(periodOrder))
	for _, key := range periodOrder {
		bySport := periodMap[key]
		var total float64
		for _, v := range bySport {
			total += v
		}
		periods = append(periods, TrainingVolumePeriod{
			Period:  key,
			BySport: bySport,
			Total:   math.Round(total*100) / 100,
		})
	}

	writeJSON(w, http.StatusOK, TrainingVolumeResponse{
		Metric:      metric,
		Granularity: granularity,
		Periods:     periods,
	})
}

// ---------------------------------------------------------------------------
// Zone Distribution
// ---------------------------------------------------------------------------

type ZoneDistributionPeriod struct {
	Period string           `json:"period"`
	Zones  map[string]int64 `json:"zones"` // key = "zone_N", value = seconds
}

type ZoneDistributionResponse struct {
	ZoneType    string                   `json:"zone_type"`
	Granularity string                   `json:"granularity"`
	Periods     []ZoneDistributionPeriod `json:"periods"`
}

func (h *Handler) GetZoneDistribution(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "missing user identity", r.URL.Path)
		return
	}

	zoneType := r.URL.Query().Get("zone_type")
	if zoneType == "" {
		zoneType = "hr"
	}
	granularity := r.URL.Query().Get("granularity")
	if granularity == "" {
		granularity = "week"
	}

	filter := ParseFilters(r)
	// $1=userID, $2=zoneType in DB method. Filter args start at $3.
	whereClause, filterArgs := filter.BuildWhereClause(3)

	rows, err := h.DB.GetZoneDistribution(ctx, userID, zoneType, granularity, whereClause, filterArgs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
		return
	}

	var periodOrder []string
	periodMap := make(map[string]map[string]int64)

	for _, row := range rows {
		key := row.Period.Format("2006-01-02")
		if _, ok := periodMap[key]; !ok {
			periodMap[key] = make(map[string]int64)
			periodOrder = append(periodOrder, key)
		}
		zoneKey := "zone_" + strconv.Itoa(row.ZoneIndex)
		periodMap[key][zoneKey] += row.TotalSeconds
	}

	periods := make([]ZoneDistributionPeriod, 0, len(periodOrder))
	for _, key := range periodOrder {
		periods = append(periods, ZoneDistributionPeriod{
			Period: key,
			Zones:  periodMap[key],
		})
	}

	writeJSON(w, http.StatusOK, ZoneDistributionResponse{
		ZoneType:    zoneType,
		Granularity: granularity,
		Periods:     periods,
	})
}

// ---------------------------------------------------------------------------
// Personal Records
// ---------------------------------------------------------------------------

func (h *Handler) GetPersonalRecords(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "missing user identity", r.URL.Path)
		return
	}

	records, err := h.DB.GetPersonalRecords(ctx, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"records": records,
	})
}
