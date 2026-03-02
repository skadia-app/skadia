package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// ListTags returns all tags for the authenticated user.
func (h *Handler) ListTags(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	tags, err := h.DB.ListTags(r.Context(), userID)
	if err != nil {
		slog.Error("list tags failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list tags", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, tags)
}

// CreateTag creates a new tag for the authenticated user.
func (h *Handler) CreateTag(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	var req struct {
		Name            string          `json:"name"`
		Color           string          `json:"color"`
		TextColor       string          `json:"text_color"`
		Conditions      json.RawMessage `json:"conditions,omitempty"`
		ConditionGroups json.RawMessage `json:"condition_groups,omitempty"`
		ConditionsLogic string          `json:"conditions_logic,omitempty"`
		Description     *string         `json:"description,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}

	// Apply defaults
	if req.Color == "" {
		req.Color = "#6366F1"
	}
	if req.TextColor == "" {
		req.TextColor = "#FFFFFF"
	}

	// Derive is_auto from presence of conditions — never trust client-supplied flag
	isAuto := len(req.ConditionGroups) > 2 || len(req.Conditions) > 2

	tag, err := h.DB.CreateTag(r.Context(), userID, req.Name, req.Color, req.TextColor, isAuto, []byte(req.Conditions), []byte(req.ConditionGroups), req.ConditionsLogic, req.Description)
	if err != nil {
		slog.Error("create tag failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create tag", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusCreated, tag)
}

// UpdateTag updates an existing tag.
func (h *Handler) UpdateTag(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	tagID := chi.URLParam(r, "id")

	var req struct {
		Name            string           `json:"name"`
		Color           string           `json:"color"`
		TextColor       string           `json:"text_color"`
		Conditions      *json.RawMessage `json:"conditions,omitempty"`
		ConditionGroups *json.RawMessage `json:"condition_groups,omitempty"`
		ConditionsLogic *string          `json:"conditions_logic,omitempty"`
		Description     *string          `json:"description,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	var condBytes *[]byte
	if req.Conditions != nil {
		b := []byte(*req.Conditions)
		condBytes = &b
	}

	var condGroupBytes *[]byte
	if req.ConditionGroups != nil {
		b := []byte(*req.ConditionGroups)
		condGroupBytes = &b
	}

	// Derive is_auto from updated conditions — never trust client-supplied flag.
	// Only recompute when at least one conditions field is being updated.
	var isAuto *bool
	if req.Conditions != nil || req.ConditionGroups != nil {
		var condGroupLen, condLen int
		if req.ConditionGroups != nil {
			condGroupLen = len(*req.ConditionGroups)
		}
		if req.Conditions != nil {
			condLen = len(*req.Conditions)
		}
		v := condGroupLen > 2 || condLen > 2
		isAuto = &v
	}

	tag, err := h.DB.UpdateTag(r.Context(), tagID, userID, req.Name, req.Color, req.TextColor, isAuto, condBytes, condGroupBytes, req.ConditionsLogic, req.Description)
	if err != nil {
		slog.Error("update tag failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update tag", r.URL.Path)
		return
	}
	if tag == nil {
		writeError(w, http.StatusNotFound, "Not Found", "tag not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, tag)
}

// DeleteTag deletes a tag.
func (h *Handler) DeleteTag(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	tagID := chi.URLParam(r, "id")

	if err := h.DB.DeleteTag(r.Context(), tagID, userID); err != nil {
		if err.Error() == "tag not found" {
			writeError(w, http.StatusNotFound, "Not Found", "tag not found", r.URL.Path)
			return
		}
		slog.Error("delete tag failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete tag", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SeedDefaultTags creates the default autotag definitions if they don't exist.
func (h *Handler) SeedDefaultTags(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	if err := h.DB.SeedDefaultAutoTags(r.Context(), userID); err != nil {
		slog.Error("seed default tags failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to seed default tags", r.URL.Path)
		return
	}

	tags, err := h.DB.ListTags(r.Context(), userID)
	if err != nil {
		slog.Error("list tags after seed failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list tags", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, tags)
}
