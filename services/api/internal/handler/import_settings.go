package handler

import (
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// GetImportSettings godoc
// GET /api/v1/settings/import
func (h *Handler) GetImportSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	s, err := h.DB.GetImportSettings(r.Context(), userID)
	if err != nil {
		slog.Error("get import settings", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, s)
}

// UpdateImportSettings godoc
// PUT /api/v1/settings/import
func (h *Handler) UpdateImportSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	s, ok := httputil.DecodeJSON[model.ImportSettings](w, r)
	if !ok {
		return
	}
	// Validate + sanitize
	valid := map[string]bool{"enrich_missing": true, "replace_all": true, "skip": true}
	if !valid[s.ConflictStrategy] {
		s.ConflictStrategy = "enrich_missing"
	}
	if s.GPSDuplicateThreshold < 60 || s.GPSDuplicateThreshold > 95 {
		s.GPSDuplicateThreshold = 75
	}
	if err := h.DB.UpdateImportSettings(r.Context(), userID, &s); err != nil {
		slog.Error("update import settings", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed", r.URL.Path)
		return
	}
	h.DB.InvalidateFusionCache(r.Context(), userID)
	httputil.WriteJSON(w, http.StatusOK, s)
}
