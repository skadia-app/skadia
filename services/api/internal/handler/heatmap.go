package handler

import (
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (h *Handler) BackfillHeatmap(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	interval, _ := h.DB.GetUserHeatmapInterval(r.Context(), userID)

	count, err := h.DB.BackfillHeatmapAll(r.Context(), userID, interval)
	if err != nil {
		slog.Error("backfill heatmap failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to backfill heatmap", r.URL.Path)
		return
	}

	slog.Info("heatmap backfill complete", "user_id", userID, "processed", count)
	httputil.WriteJSON(w, http.StatusOK, map[string]int{"processed": count})
}

func (h *Handler) BackfillHeatmapPoints(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	interval, _ := h.DB.GetUserHeatmapInterval(r.Context(), userID)

	count, err := h.DB.BackfillHeatmapAll(r.Context(), userID, interval)
	if err != nil {
		slog.Error("backfill heatmap points failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to backfill heatmap points", r.URL.Path)
		return
	}

	slog.Info("heatmap points backfill complete", "user_id", userID, "processed", count)
	httputil.WriteJSON(w, http.StatusOK, map[string]int{"processed": count})
}

func (h *Handler) GetHeatmapSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	s, err := h.DB.GetHeatmapSettings(r.Context(), userID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get heatmap settings", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, s)
}

func (h *Handler) UpdateHeatmapSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	req, ok := httputil.DecodeJSON[model.HeatmapSettings](w, r)
	if !ok {
		return
	}
	if req.SamplingIntervalM < 10 || req.SamplingIntervalM > 100 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "sampling_interval_m must be 10–100", r.URL.Path)
		return
	}
	if req.TeleportThresholdM < 100 || req.TeleportThresholdM > 50000 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "teleport_threshold_m must be 100–50000", r.URL.Path)
		return
	}
	if err := h.DB.UpdateHeatmapSettings(r.Context(), userID, &req); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update heatmap settings", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, req)
}
