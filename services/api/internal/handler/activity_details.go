package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
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

// PatchSplit updates mutable split fields (currently only climbing_grade).
// PATCH /api/v1/activities/{id}/splits/{splitId}
func (h *Handler) PatchSplit(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "id")
	splitID := chi.URLParam(r, "splitId")

	var update model.SplitUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid request body", r.URL.Path)
		return
	}

	found, err := h.DB.UpdateSplitClimbingGrade(r.Context(), activityID, splitID, update.ClimbingGrade)
	if err != nil {
		slog.Error("update split climbing grade failed", "error", err, "split_id", splitID)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update split", r.URL.Path)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "Not Found", "split not found", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
