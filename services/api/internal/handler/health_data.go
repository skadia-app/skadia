package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
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
	userID := middleware.GetUserID(r.Context())

	var req healthIngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid request body", r.URL.Path)
		return
	}
	if req.Type == "" || req.Date == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "type and date required", r.URL.Path)
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
			writeError(w, http.StatusBadRequest, "Bad Request", "invalid hrv data", r.URL.Path)
			return
		}
		ts, err := time.Parse("2006-01-02", req.Date)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Bad Request", "invalid date format, use YYYY-MM-DD", r.URL.Path)
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
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to save hrv data", r.URL.Path)
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
			writeError(w, http.StatusBadRequest, "Bad Request", "invalid sleep data", r.URL.Path)
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
			SleepStartUTC: sleepStart,
			SleepEndUTC:   sleepEnd,
			DeepSeconds:   d.DeepSeconds,
			LightSeconds:  d.LightSeconds,
			RemSeconds:    d.RemSeconds,
			AwakeSeconds:  d.AwakeSeconds,
			ScoreOverall:  d.ScoreOverall,
			SourceType:    "mobile",
		}
		if err := h.DB.UpsertHealthSleep(r.Context(), sleep); err != nil {
			slog.Error("ingest sleep failed", "error", err)
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to save sleep data", r.URL.Path)
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
			writeError(w, http.StatusBadRequest, "Bad Request", "invalid weight data", r.URL.Path)
			return
		}
		if d.WeightG <= 0 {
			writeError(w, http.StatusBadRequest, "Bad Request", "weight_g must be positive", r.URL.Path)
			return
		}
		ts, err := time.Parse("2006-01-02", req.Date)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Bad Request", "invalid date format, use YYYY-MM-DD", r.URL.Path)
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
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to save weight data", r.URL.Path)
			return
		}

	case "resting_hr":
		// Store resting HR in health_daily_summary using UPSERT.
		var d struct {
			RestingHR *int `json:"resting_hr_bpm"`
		}
		if err := json.Unmarshal(req.Data, &d); err != nil {
			writeError(w, http.StatusBadRequest, "Bad Request", "invalid resting_hr data", r.URL.Path)
			return
		}
		summary := &model.HealthDailySummary{
			UserID:          userID,
			CalendarDate:    req.Date,
			RestingHeartRate: d.RestingHR,
			SourceType:      "mobile",
		}
		if err := h.DB.UpsertHealthDailySummary(r.Context(), summary); err != nil {
			slog.Error("ingest resting_hr failed", "error", err)
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to save resting hr data", r.URL.Path)
			return
		}

	default:
		// Not yet implemented type — accept gracefully with 202 Accepted
		writeJSON(w, http.StatusAccepted, map[string]string{
			"status": "accepted",
			"note":   "type '" + req.Type + "' not yet implemented",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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

// GET /health/daily-summary?from=&to=
func (h *Handler) GetHealthDailySummaries(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	from, to := defaultDateRange(r)

	items, err := h.DB.GetHealthDailySummaries(r.Context(), userID, from, to)
	if err != nil {
		slog.Error("get health daily summaries failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get health daily summaries", r.URL.Path)
		return
	}
	if items == nil {
		items = []model.HealthDailySummary{}
	}
	writeJSON(w, http.StatusOK, items)
}

// GET /health/sleep?from=&to=
func (h *Handler) GetHealthSleep(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	from, to := defaultDateRange(r)

	items, err := h.DB.GetHealthSleep(r.Context(), userID, from, to)
	if err != nil {
		slog.Error("get health sleep failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get health sleep", r.URL.Path)
		return
	}
	if items == nil {
		items = []model.HealthSleep{}
	}
	writeJSON(w, http.StatusOK, items)
}

// GET /health/hrv?from=&to=
func (h *Handler) GetHealthHRV(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	from, to := defaultTimestampRange(r)

	items, err := h.DB.GetHealthHRV(r.Context(), userID, from, to)
	if err != nil {
		slog.Error("get health hrv failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get health hrv", r.URL.Path)
		return
	}
	if items == nil {
		items = []model.HealthHRVSnapshot{}
	}
	writeJSON(w, http.StatusOK, items)
}

// GET /health/weight?from=&to=
func (h *Handler) GetHealthWeight(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	from, to := defaultTimestampRange(r)

	items, err := h.DB.GetHealthWeight(r.Context(), userID, from, to)
	if err != nil {
		slog.Error("get health weight failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get health weight", r.URL.Path)
		return
	}
	if items == nil {
		items = []model.HealthWeightEntry{}
	}
	writeJSON(w, http.StatusOK, items)
}

// POST /health/weight — manual weight entry creation
func (h *Handler) CreateHealthWeightEntry(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var create model.HealthWeightCreate
	if err := json.NewDecoder(r.Body).Decode(&create); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if create.WeightG <= 0 {
		writeError(w, http.StatusBadRequest, "Bad Request", "weight_g must be a positive integer", r.URL.Path)
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
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to save weight entry", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

// GET /health/readiness?from=&to=
func (h *Handler) GetHealthReadiness(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	from, to := defaultDateRange(r)

	items, err := h.DB.GetHealthReadiness(r.Context(), userID, from, to)
	if err != nil {
		slog.Error("get health readiness failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get health readiness", r.URL.Path)
		return
	}
	if items == nil {
		items = []model.HealthReadinessScore{}
	}
	writeJSON(w, http.StatusOK, items)
}

// GET /health/training-load?from=&to=
func (h *Handler) GetHealthTrainingLoad(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	from, to := defaultDateRange(r)

	items, err := h.DB.GetHealthTrainingLoad(r.Context(), userID, from, to)
	if err != nil {
		slog.Error("get health training load failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get health training load", r.URL.Path)
		return
	}
	if items == nil {
		items = []model.HealthTrainingLoad{}
	}
	writeJSON(w, http.StatusOK, items)
}

// GET /health/stress?from=&to=
func (h *Handler) GetHealthStress(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	from, to := defaultTimestampRange(r)

	items, err := h.DB.GetHealthStress(r.Context(), userID, from, to)
	if err != nil {
		slog.Error("get health stress failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get health stress", r.URL.Path)
		return
	}
	if items == nil {
		items = []model.HealthStressSnapshot{}
	}
	writeJSON(w, http.StatusOK, items)
}

// GET /health/spo2?from=&to=
func (h *Handler) GetHealthSpO2(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	from, to := defaultTimestampRange(r)

	items, err := h.DB.GetHealthSpO2(r.Context(), userID, from, to)
	if err != nil {
		slog.Error("get health spo2 failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get health spo2", r.URL.Path)
		return
	}
	if items == nil {
		items = []model.HealthSpO2Snapshot{}
	}
	writeJSON(w, http.StatusOK, items)
}

// GET /health/blood-pressure?from=&to=
func (h *Handler) GetHealthBloodPressure(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	from, to := defaultTimestampRange(r)

	items, err := h.DB.GetHealthBloodPressure(r.Context(), userID, from, to)
	if err != nil {
		slog.Error("get health blood pressure failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get health blood pressure", r.URL.Path)
		return
	}
	if items == nil {
		items = []model.HealthBloodPressure{}
	}
	writeJSON(w, http.StatusOK, items)
}

// GET /health/fitness-age?from=&to=
func (h *Handler) GetHealthFitnessAge(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	from, to := defaultDateRange(r)

	items, err := h.DB.GetHealthFitnessAge(r.Context(), userID, from, to)
	if err != nil {
		slog.Error("get health fitness age failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get health fitness age", r.URL.Path)
		return
	}
	if items == nil {
		items = []model.HealthFitnessAge{}
	}
	writeJSON(w, http.StatusOK, items)
}

// GET /health/overview
func (h *Handler) GetHealthOverview(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	overview, err := h.DB.GetHealthOverview(r.Context(), userID)
	if err != nil {
		slog.Error("get health overview failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get health overview", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}
