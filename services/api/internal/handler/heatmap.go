package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (h *Handler) BackfillHeatmap(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	interval, _ := h.DB.GetUserHeatmapInterval(r.Context(), userID)

	count, err := h.DB.BackfillHeatmapAll(r.Context(), userID, interval)
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

	count, err := h.DB.BackfillHeatmapAll(r.Context(), userID, interval)
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
	s, err := h.DB.GetHeatmapSettings(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get heatmap settings", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *Handler) UpdateHeatmapSettings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var req model.HeatmapSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.SamplingIntervalM < 10 || req.SamplingIntervalM > 100 {
		writeError(w, http.StatusBadRequest, "Bad Request", "sampling_interval_m must be 10–100", r.URL.Path)
		return
	}
	if req.TeleportThresholdM < 100 || req.TeleportThresholdM > 50000 {
		writeError(w, http.StatusBadRequest, "Bad Request", "teleport_threshold_m must be 100–50000", r.URL.Path)
		return
	}
	if err := h.DB.UpdateHeatmapSettings(r.Context(), userID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update heatmap settings", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, req)
}
