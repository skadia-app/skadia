package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// GetRecordingGroups godoc
// GET /api/v1/recording-groups?limit=50&offset=0
func (h *Handler) GetRecordingGroups(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	groups, err := h.DB.GetRecordingGroupsForUser(r.Context(), userID, limit, offset)
	if err != nil {
		slog.Error("get recording groups", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get recording groups", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, groups)
}

// GetRecordingGroup godoc
// GET /api/v1/recording-groups/{id}
func (h *Handler) GetRecordingGroup(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	groupID := chi.URLParam(r, "id")

	group, err := h.DB.GetRecordingGroup(r.Context(), groupID)
	if err != nil {
		slog.Error("get recording group", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get recording group", r.URL.Path)
		return
	}
	if group == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "recording group not found", r.URL.Path)
		return
	}
	if group.UserID != userID {
		httputil.WriteError(w, http.StatusForbidden, "Forbidden", "access denied", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, group)
}

// GetRecordingGroupMembers godoc
// GET /api/v1/recording-groups/{id}/members
func (h *Handler) GetRecordingGroupMembers(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	groupID := chi.URLParam(r, "id")

	group, err := h.DB.GetRecordingGroup(r.Context(), groupID)
	if err != nil {
		slog.Error("get recording group for members", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get recording group", r.URL.Path)
		return
	}
	if group == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "recording group not found", r.URL.Path)
		return
	}
	if group.UserID != userID {
		httputil.WriteError(w, http.StatusForbidden, "Forbidden", "access denied", r.URL.Path)
		return
	}

	members, err := h.DB.GetRecordingGroupMembers(r.Context(), groupID)
	if err != nil {
		slog.Error("get recording group members", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get recording group members", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, members)
}

// DissolveRecordingGroup godoc
// POST /api/v1/recording-groups/{id}/dissolve
func (h *Handler) DissolveRecordingGroup(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	groupID := chi.URLParam(r, "id")

	group, err := h.DB.GetRecordingGroup(r.Context(), groupID)
	if err != nil {
		slog.Error("get recording group for dissolve", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get recording group", r.URL.Path)
		return
	}
	if group == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "recording group not found", r.URL.Path)
		return
	}
	if group.UserID != userID {
		httputil.WriteError(w, http.StatusForbidden, "Forbidden", "access denied", r.URL.Path)
		return
	}

	if err := h.DB.DissolveRecordingGroup(r.Context(), groupID); err != nil {
		slog.Error("dissolve recording group", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to dissolve recording group", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type setRecordingGroupPrimaryRequest struct {
	ActivityID string `json:"activity_id"`
}

// SetRecordingGroupPrimary godoc
// PATCH /api/v1/recording-groups/{id}/primary
// Body: {"activity_id": "uuid"}
func (h *Handler) SetRecordingGroupPrimary(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	groupID := chi.URLParam(r, "id")

	body, ok := httputil.DecodeJSON[setRecordingGroupPrimaryRequest](w, r)
	if !ok {
		return
	}

	group, err := h.DB.GetRecordingGroup(r.Context(), groupID)
	if err != nil {
		slog.Error("get recording group for set primary", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get recording group", r.URL.Path)
		return
	}
	if group == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "recording group not found", r.URL.Path)
		return
	}
	if group.UserID != userID {
		httputil.WriteError(w, http.StatusForbidden, "Forbidden", "access denied", r.URL.Path)
		return
	}

	if err := h.DB.SetRecordingGroupPrimary(r.Context(), groupID, body.ActivityID); err != nil {
		slog.Error("set recording group primary", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to set primary activity", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type setRecordingGroupSportTypeRequest struct {
	SportType string `json:"sport_type"`
}

// SetRecordingGroupSportType godoc
// PATCH /api/v1/recording-groups/{id}/sport-type
// Body: {"sport_type": "running"}
func (h *Handler) SetRecordingGroupSportType(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	groupID := chi.URLParam(r, "id")

	body, ok := httputil.DecodeJSON[setRecordingGroupSportTypeRequest](w, r)
	if !ok {
		return
	}

	group, err := h.DB.GetRecordingGroup(r.Context(), groupID)
	if err != nil {
		slog.Error("get recording group for set sport type", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get recording group", r.URL.Path)
		return
	}
	if group == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "recording group not found", r.URL.Path)
		return
	}
	if group.UserID != userID {
		httputil.WriteError(w, http.StatusForbidden, "Forbidden", "access denied", r.URL.Path)
		return
	}

	if err := h.DB.SetRecordingGroupSportType(r.Context(), groupID, body.SportType); err != nil {
		slog.Error("set recording group sport type", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to set sport type", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
