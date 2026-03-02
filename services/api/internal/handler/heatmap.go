package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

func (h *Handler) BackfillHeatmap(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	count, err := h.DB.BackfillHeatmap(r.Context(), userID)
	if err != nil {
		slog.Error("backfill heatmap failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to backfill heatmap", r.URL.Path)
		return
	}

	slog.Info("heatmap backfill complete", "user_id", userID, "processed", count)

	writeJSON(w, http.StatusOK, map[string]int{"processed": count})
}

func (h *Handler) BackfillHeatmapPoints(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	interval, _ := h.DB.GetUserHeatmapInterval(r.Context(), userID)

	count, err := h.DB.BackfillHeatmapPoints(r.Context(), userID, interval)
	if err != nil {
		slog.Error("backfill heatmap points failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to backfill heatmap points", r.URL.Path)
		return
	}

	slog.Info("heatmap points backfill complete", "user_id", userID, "processed", count)
	writeJSON(w, http.StatusOK, map[string]int{"processed": count})
}

func (h *Handler) GetHeatmapSettings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	interval, _ := h.DB.GetUserHeatmapInterval(r.Context(), userID)
	writeJSON(w, http.StatusOK, map[string]int{"heatmap_sample_interval_m": interval})
}

func (h *Handler) UpdateHeatmapSettings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var req struct {
		HeatmapSampleIntervalM int `json:"heatmap_sample_interval_m"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.HeatmapSampleIntervalM < 10 || req.HeatmapSampleIntervalM > 100 {
		writeError(w, http.StatusBadRequest, "Bad Request", "interval must be between 10 and 100", r.URL.Path)
		return
	}
	if err := h.DB.UpdateUserHeatmapInterval(r.Context(), userID, req.HeatmapSampleIntervalM); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update interval", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"heatmap_sample_interval_m": req.HeatmapSampleIntervalM})
}
