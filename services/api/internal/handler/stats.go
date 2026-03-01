package handler

import (
	"net/http"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

func (h *Handler) GetPMC(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	if fromStr == "" || toStr == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "from and to query parameters are required (YYYY-MM-DD)", r.URL.Path)
		return
	}

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid from date, expected YYYY-MM-DD", r.URL.Path)
		return
	}

	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid to date, expected YYYY-MM-DD", r.URL.Path)
		return
	}

	if to.Before(from) {
		writeError(w, http.StatusBadRequest, "Bad Request", "to must be after from", r.URL.Path)
		return
	}

	days, err := h.DB.GetPMCRange(r.Context(), userID, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to fetch PMC data", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, days)
}
