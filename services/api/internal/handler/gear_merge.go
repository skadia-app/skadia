package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// GetGearDuplicateCandidates godoc
// GET /api/v1/gear/duplicate-candidates
func (h *Handler) GetGearDuplicateCandidates(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	pairs, err := h.DB.FindGearDuplicates(r.Context(), userID)
	if err != nil {
		slog.Error("find gear duplicates", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed", r.URL.Path)
		return
	}
	if pairs == nil {
		pairs = [][2]string{}
	}
	httputil.WriteJSON(w, http.StatusOK, pairs)
}

// MergeGear godoc
// POST /api/v1/gear/{id}/merge
// Body: {"merge_from_id": "uuid"}
func (h *Handler) MergeGear(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	primaryID := chi.URLParam(r, "id")

	var req struct {
		MergeFromID string `json:"merge_from_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MergeFromID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "merge_from_id required", r.URL.Path)
		return
	}

	if err := h.DB.MergeGear(r.Context(), userID, primaryID, req.MergeFromID); err != nil {
		slog.Error("merge gear failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "merge failed", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "merged", "primary_id": primaryID})
}
