package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) GetLaps(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "id")

	laps, err := h.DB.GetLaps(r.Context(), activityID)
	if err != nil {
		slog.Error("get laps failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get laps", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, laps)
}

func (h *Handler) GetSplits(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "id")

	splits, err := h.DB.GetSplits(r.Context(), activityID)
	if err != nil {
		slog.Error("get splits failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get splits", r.URL.Path)
		return
	}

	summaries, err := h.DB.GetSplitSummaries(r.Context(), activityID)
	if err != nil {
		slog.Error("get split summaries failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get split summaries", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"splits":    splits,
		"summaries": summaries,
	})
}

func (h *Handler) GetLengths(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "id")

	lengths, err := h.DB.GetLengths(r.Context(), activityID)
	if err != nil {
		slog.Error("get lengths failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get lengths", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, lengths)
}

func (h *Handler) GetClimbProEvents(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "id")

	events, err := h.DB.GetClimbProEvents(r.Context(), activityID)
	if err != nil {
		slog.Error("get climb pro events failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get climb pro events", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, events)
}
