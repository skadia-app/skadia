package handler

import (
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// SetCacheBudget adjusts the tile cache budget at runtime.
// PUT /admin/routing/cache-budget
// Admin-Berechtigung wird durch OIDC-Proxy sichergestellt.
func (h *Handler) SetCacheBudget(w http.ResponseWriter, r *http.Request) {
	if h.GraphManager == nil {
		httputil.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "routing not configured"})
		return
	}
	if _, ok := httputil.RequireUserID(w, r); !ok {
		return
	}
	req, ok := httputil.DecodeJSON[struct {
		Bytes int64 `json:"bytes"`
	}](w, r)
	if !ok {
		return
	}
	if req.Bytes <= 0 {
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "bytes must be positive"})
		return
	}

	h.GraphManager.SetCacheBudget(req.Bytes)
	hits, misses, evictions, usedBytes, budgetBytes := h.GraphManager.CacheStats()
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"status":       "updated",
		"budget_bytes": budgetBytes,
		"used_bytes":   usedBytes,
		"hits":         hits,
		"misses":       misses,
		"evictions":    evictions,
	})
}
