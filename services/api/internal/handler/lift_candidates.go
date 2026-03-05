package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// GetLiftCandidates returns lift ride candidates for an activity.
// GET /api/v1/activities/{id}/lift-candidates
func (h *Handler) GetLiftCandidates(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "id")
	candidates, err := h.DB.GetLiftCandidates(r.Context(), activityID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, candidates)
}

// ConfirmLiftCandidate marks a lift candidate as confirmed.
// POST /api/v1/activities/{id}/lift-candidates/{cid}/confirm
func (h *Handler) ConfirmLiftCandidate(w http.ResponseWriter, r *http.Request) {
	candID := chi.URLParam(r, "cid")
	if err := h.DB.ConfirmLiftCandidate(r.Context(), candID); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RejectLiftCandidate deletes a lift candidate and recomputes activity lift stats.
// DELETE /api/v1/activities/{id}/lift-candidates/{cid}
func (h *Handler) RejectLiftCandidate(w http.ResponseWriter, r *http.Request) {
	candID := chi.URLParam(r, "cid")
	if err := h.DB.RejectLiftCandidate(r.Context(), candID); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
