package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

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
