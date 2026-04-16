package handler

import (
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

func (h *Handler) GetActivityDeveloperFields(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	_, activityID, ok := h.requireActivity(w, r, userID)
	if !ok {
		return
	}

	fields, err := h.DB.GetDeveloperFieldsForActivity(r.Context(), activityID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get developer fields", r.URL.Path)
		return
	}
	if fields == nil {
		fields = []database.DeveloperFieldInfo{}
	}

	httputil.WriteJSON(w, http.StatusOK, fields)
}

func (h *Handler) GetStreams(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	_, activityID, ok := h.requireActivity(w, r, userID)
	if !ok {
		return
	}

	streams, err := h.DB.GetStreams(r.Context(), activityID)
	if err != nil {
		slog.Error("get streams failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get streams", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, streams)
}
