package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ── Mobile Health Data Ingest ────────────────────────────────────────────────

// healthIngestRequest is the body for POST /import/health-data.
type healthIngestRequest struct {
	Type string          `json:"type"` // steps|resting_hr|sleep|hrv|spo2|weight
	Date string          `json:"date"` // YYYY-MM-DD
	Data json.RawMessage `json:"data"`
}

// IngestHealthData accepts health data payloads from the mobile app and stores
// them using the same UPSERT logic as the Garmin importer.
//
// POST /api/v1/import/health-data
func (h *Handler) IngestHealthData(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	req, ok := httputil.DecodeJSON[healthIngestRequest](w, r)
	if !ok {
		return
	}
	if req.Type == "" || req.Date == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "type and date required", r.URL.Path)
		return
	}

	switch req.Type {

	case "hrv":
		// Parse HRV fields: rmssd_ms (float64), time_of_day (string, optional)
		var d struct {
			RmssdMS   *float64 `json:"rmssd_ms"`
			SdrrMS    *float64 `json:"sdrr_ms"`
			TimeOfDay *string  `json:"time_of_day"`
		}
		if err := json.Unmarshal(req.Data, &d); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid hrv data", r.URL.Path)
			return
		}
		ts, err := time.Parse("2006-01-02", req.Date)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid date format, use YYYY-MM-DD", r.URL.Path)
			return
		}
		timeOfDay := "daily_avg"
		if d.TimeOfDay != nil && *d.TimeOfDay != "" {
			timeOfDay = *d.TimeOfDay
		}
		snap := &model.HealthHRVSnapshot{
			UserID:        userID,
			MeasuredAtUTC: ts,
			RmssdMS:       d.RmssdMS,
			SdrrMS:        d.SdrrMS,
			TimeOfDay:     &timeOfDay,
			SourceType:    "mobile",
		}
		if err := h.DB.UpsertHealthHRVSnapshot(r.Context(), snap); err != nil {
			slog.Error("ingest hrv failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to save hrv data", r.URL.Path)
			return
		}

	case "sleep":
		// Parse sleep fields matching model.HealthSleep.
		// SleepStartUTC/SleepEndUTC are optional strings in RFC3339; default to midnight of date.
		var d struct {
			SleepStartUTC string `json:"sleep_start_utc"`
			SleepEndUTC   string `json:"sleep_end_utc"`
			DeepSeconds   *int   `json:"deep_seconds"`
			LightSeconds  *int   `json:"light_seconds"`
			RemSeconds    *int   `json:"rem_seconds"`
			AwakeSeconds  *int   `json:"awake_seconds"`
			ScoreOverall  *int   `json:"score_overall"`
		}
		if err := json.Unmarshal(req.Data, &d); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid sleep data", r.URL.Path)
			return
		}
		midnight, _ := time.Parse("2006-01-02", req.Date)
		sleepStart := midnight
		sleepEnd := midnight
		if d.SleepStartUTC != "" {
			if t, err := time.Parse(time.RFC3339, d.SleepStartUTC); err == nil {
				sleepStart = t
			}
		}
		if d.SleepEndUTC != "" {
			if t, err := time.Parse(time.RFC3339, d.SleepEndUTC); err == nil {
				sleepEnd = t
			}
		}
		sleep := &model.HealthSleep{
			UserID:        userID,
			CalendarDate:  req.Date,
			SleepStartUTC: &sleepStart,
			SleepEndUTC:   &sleepEnd,
			DeepSeconds:   d.DeepSeconds,
			LightSeconds:  d.LightSeconds,
			RemSeconds:    d.RemSeconds,
			AwakeSeconds:  d.AwakeSeconds,
			ScoreOverall:  d.ScoreOverall,
			SourceType:    "mobile",
		}
		if err := h.DB.UpsertHealthSleep(r.Context(), sleep); err != nil {
			slog.Error("ingest sleep failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to save sleep data", r.URL.Path)
			return
		}

	case "weight":
		// Parse weight fields matching model.HealthWeightEntry
		var d struct {
			WeightG     int      `json:"weight_g"`
			BMI         *float64 `json:"bmi"`
			BodyFatPct  *float64 `json:"body_fat_pct"`
			MuscleMassG *int     `json:"muscle_mass_g"`
			BoneMassG   *int     `json:"bone_mass_g"`
		}
		if err := json.Unmarshal(req.Data, &d); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid weight data", r.URL.Path)
			return
		}
		if d.WeightG <= 0 {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "weight_g must be positive", r.URL.Path)
			return
		}
		ts, err := time.Parse("2006-01-02", req.Date)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid date format, use YYYY-MM-DD", r.URL.Path)
			return
		}
		entry := &model.HealthWeightEntry{
			UserID:        userID,
			MeasuredAtUTC: ts,
			WeightG:       d.WeightG,
			BMI:           d.BMI,
			BodyFatPct:    d.BodyFatPct,
			MuscleMassG:   d.MuscleMassG,
			BoneMassG:     d.BoneMassG,
			SourceType:    "mobile",
		}
		if err := h.DB.UpsertHealthWeightEntry(r.Context(), entry); err != nil {
			slog.Error("ingest weight failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to save weight data", r.URL.Path)
			return
		}

	case "resting_hr":
		// Store resting HR in health_daily_summary using UPSERT.
		var d struct {
			RestingHR *int `json:"resting_hr_bpm"`
		}
		if err := json.Unmarshal(req.Data, &d); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid resting_hr data", r.URL.Path)
			return
		}
		summary := &model.HealthDailySummary{
			UserID:           userID,
			CalendarDate:     req.Date,
			RestingHeartRate: d.RestingHR,
			SourceType:       "mobile",
		}
		if err := h.DB.UpsertHealthDailySummary(r.Context(), summary); err != nil {
			slog.Error("ingest resting_hr failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to save resting hr data", r.URL.Path)
			return
		}

	default:
		// Not yet implemented type — accept gracefully with 202 Accepted
		httputil.WriteJSON(w, http.StatusAccepted, map[string]string{
			"status": "accepted",
			"note":   "type '" + req.Type + "' not yet implemented",
		})
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// defaultDateRange returns from/to query params, defaulting to the last 30 days.
func defaultDateRange(r *http.Request) (from, to string) {
	from = r.URL.Query().Get("from")
	to = r.URL.Query().Get("to")
	if from == "" {
		from = time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	}
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}
	return from, to
}

// defaultTimestampRange returns from/to as timestamps defaulting to last 30 days.
func defaultTimestampRange(r *http.Request) (from, to string) {
	from = r.URL.Query().Get("from")
	to = r.URL.Query().Get("to")
	if from == "" {
		from = time.Now().AddDate(0, -1, 0).Format(time.RFC3339)
	}
	if to == "" {
		to = time.Now().Format(time.RFC3339)
	}
	return from, to
}

// healthListHandler creates a handler that fetches a typed health list with date range filtering.
func healthListHandler[T any](
	h *Handler,
	fetch func(ctx context.Context, userID, from, to string) ([]T, error),
	useTimestamp bool,
	label string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httputil.RequireUserID(w, r)
		if !ok {
			return
		}
		var from, to string
		if useTimestamp {
			from, to = defaultTimestampRange(r)
		} else {
			from, to = defaultDateRange(r)
		}
		items, err := fetch(r.Context(), userID, from, to)
		if err != nil {
			slog.Error("get "+label+" failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get "+label, r.URL.Path)
			return
		}
		if items == nil {
			items = []T{}
		}
		httputil.WriteJSON(w, http.StatusOK, items)
	}
}

func (h *Handler) GetHealthDailySummaries(w http.ResponseWriter, r *http.Request) {
	healthListHandler(h, h.DB.GetHealthDailySummaries, false, "health daily summaries")(w, r)
}
func (h *Handler) GetHealthSleep(w http.ResponseWriter, r *http.Request) {
	healthListHandler(h, h.DB.GetHealthSleep, false, "health sleep")(w, r)
}
func (h *Handler) GetHealthHRV(w http.ResponseWriter, r *http.Request) {
	healthListHandler(h, h.DB.GetHealthHRV, true, "health hrv")(w, r)
}
func (h *Handler) GetHealthWeight(w http.ResponseWriter, r *http.Request) {
	healthListHandler(h, h.DB.GetHealthWeight, true, "health weight")(w, r)
}

// POST /health/weight — manual weight entry creation
func (h *Handler) CreateHealthWeightEntry(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	create, ok := httputil.DecodeJSON[model.HealthWeightCreate](w, r)
	if !ok {
		return
	}
	if create.WeightG <= 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "weight_g must be a positive integer", r.URL.Path)
		return
	}

	measuredAt, err := time.Parse(time.RFC3339, create.MeasuredAtUTC)
	if err != nil {
		measuredAt = time.Now().UTC()
	}

	sourceType := create.SourceType
	if sourceType == "" {
		sourceType = "manual"
	}

	entry := &model.HealthWeightEntry{
		UserID:        userID,
		MeasuredAtUTC: measuredAt,
		WeightG:       create.WeightG,
		BMI:           create.BMI,
		BodyFatPct:    create.BodyFatPct,
		MuscleMassG:   create.MuscleMassG,
		BoneMassG:     create.BoneMassG,
		SourceType:    sourceType,
	}

	if err := h.DB.UpsertHealthWeightEntry(r.Context(), entry); err != nil {
		slog.Error("create health weight entry failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to save weight entry", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, entry)
}

func (h *Handler) GetHealthReadiness(w http.ResponseWriter, r *http.Request) {
	healthListHandler(h, h.DB.GetHealthReadiness, false, "health readiness")(w, r)
}
func (h *Handler) GetHealthTrainingLoad(w http.ResponseWriter, r *http.Request) {
	healthListHandler(h, h.DB.GetHealthTrainingLoad, false, "health training load")(w, r)
}
func (h *Handler) GetHealthStress(w http.ResponseWriter, r *http.Request) {
	healthListHandler(h, h.DB.GetHealthStress, true, "health stress")(w, r)
}
func (h *Handler) GetHealthSpO2(w http.ResponseWriter, r *http.Request) {
	healthListHandler(h, h.DB.GetHealthSpO2, true, "health spo2")(w, r)
}
func (h *Handler) GetHealthBloodPressure(w http.ResponseWriter, r *http.Request) {
	healthListHandler(h, h.DB.GetHealthBloodPressure, true, "health blood pressure")(w, r)
}
func (h *Handler) GetHealthFitnessAge(w http.ResponseWriter, r *http.Request) {
	healthListHandler(h, h.DB.GetHealthFitnessAge, false, "health fitness age")(w, r)
}

// GET /health/overview
func (h *Handler) GetHealthOverview(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	overview, err := h.DB.GetHealthOverview(r.Context(), userID)
	if err != nil {
		slog.Error("get health overview failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get health overview", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, overview)
}
