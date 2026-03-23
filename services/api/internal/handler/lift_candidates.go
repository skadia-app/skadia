package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// GetLiftCandidates returns lift ride candidates for an activity.
// GET /api/v1/activities/{id}/lift-candidates
func (h *Handler) GetLiftCandidates(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("get activity for lift candidates failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	candidates, err := h.DB.GetLiftCandidates(r.Context(), activityID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, candidates)
}

// ConfirmLiftCandidate marks a lift candidate as confirmed.
// POST /api/v1/activities/{id}/lift-candidates/{cid}/confirm
func (h *Handler) ConfirmLiftCandidate(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("get activity for confirm lift candidate failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	candID := chi.URLParam(r, "cid")
	if err := h.DB.ConfirmLiftCandidate(r.Context(), candID, activityID); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RejectLiftCandidate deletes a lift candidate and recomputes activity lift stats.
// DELETE /api/v1/activities/{id}/lift-candidates/{cid}
func (h *Handler) RejectLiftCandidate(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("get activity for reject lift candidate failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	candID := chi.URLParam(r, "cid")
	if err := h.DB.RejectLiftCandidate(r.Context(), candID, activityID); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
