package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (h *Handler) GetLaps(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	_, activityID, ok := h.requireActivity(w, r, userID)
	if !ok {
		return
	}

	laps, err := h.DB.GetLaps(r.Context(), activityID)
	if err != nil {
		slog.Error("get laps failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get laps", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, laps)
}

func (h *Handler) GetSplits(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	_, activityID, ok := h.requireActivity(w, r, userID)
	if !ok {
		return
	}

	splits, err := h.DB.GetSplits(r.Context(), activityID)
	if err != nil {
		slog.Error("get splits failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get splits", r.URL.Path)
		return
	}

	summaries, err := h.DB.GetSplitSummaries(r.Context(), activityID)
	if err != nil {
		slog.Error("get split summaries failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get split summaries", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"splits":    splits,
		"summaries": summaries,
	})
}

func (h *Handler) GetLengths(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	_, activityID, ok := h.requireActivity(w, r, userID)
	if !ok {
		return
	}

	lengths, err := h.DB.GetLengths(r.Context(), activityID)
	if err != nil {
		slog.Error("get lengths failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get lengths", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, lengths)
}

func (h *Handler) GetClimbProEvents(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	_, activityID, ok := h.requireActivity(w, r, userID)
	if !ok {
		return
	}

	events, err := h.DB.GetClimbProEvents(r.Context(), activityID)
	if err != nil {
		slog.Error("get climb pro events failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get climb pro events", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, events)
}

// PatchSplit updates mutable split fields (currently only climbing_grade).
// PATCH /api/v1/activities/{id}/splits/{splitId}
func (h *Handler) PatchSplit(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	_, activityID, ok := h.requireActivity(w, r, userID)
	if !ok {
		return
	}
	splitID := chi.URLParam(r, "splitId")

	update, ok := httputil.DecodeJSON[model.SplitUpdate](w, r)
	if !ok {
		return
	}

	found, err := h.DB.UpdateSplitClimbingGrade(r.Context(), activityID, splitID, update.ClimbingGrade)
	if err != nil {
		slog.Error("update split climbing grade failed", "error", err, "split_id", splitID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update split", r.URL.Path)
		return
	}
	if !found {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "split not found", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
