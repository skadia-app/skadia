package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

type importLogResponse struct {
	Entries interface{} `json:"entries"`
	Total   int         `json:"total"`
}

// GetImports godoc
// GET /api/v1/imports
// Query params: status (comma-separated), source_platform, limit, offset
func (h *Handler) GetImports(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	q := r.URL.Query()

	var statusFilter []string
	if raw := q.Get("status"); raw != "" {
		for _, s := range strings.Split(raw, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				statusFilter = append(statusFilter, s)
			}
		}
	}

	sourcePlatform := q.Get("source_platform")

	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	entries, total, err := h.DB.ListImportLog(r.Context(), userID, statusFilter, sourcePlatform, limit, offset)
	if err != nil {
		slog.Error("list import log", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load import log", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, importLogResponse{
		Entries: entries,
		Total:   total,
	})
}
