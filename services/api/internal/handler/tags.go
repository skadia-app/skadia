package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
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
		Lat             *float64        `json:"lat"`
		Lng             *float64        `json:"lng"`
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

	tag, err := h.DB.CreateTag(r.Context(), userID, req.Name, req.Color, req.TextColor, isAuto, []byte(req.Conditions), []byte(req.ConditionGroups), req.ConditionsLogic, req.Description, req.Lat, req.Lng)
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

	// Use json.RawMessage for lat/lng so we can distinguish "absent" from "null".
	var req struct {
		Name            string           `json:"name"`
		Color           string           `json:"color"`
		TextColor       string           `json:"text_color"`
		Conditions      *json.RawMessage `json:"conditions,omitempty"`
		ConditionGroups *json.RawMessage `json:"condition_groups,omitempty"`
		ConditionsLogic *string          `json:"conditions_logic,omitempty"`
		Description     *string          `json:"description,omitempty"`
		Lat             *json.RawMessage `json:"lat"`
		Lng             *json.RawMessage `json:"lng"`
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

	// Parse lat/lng from raw JSON — nil raw = field absent = don't update.
	// "null" raw = clear location. A number = set location.
	var lat, lng *float64
	locationPresent := req.Lat != nil || req.Lng != nil
	if req.Lat != nil && string(*req.Lat) != "null" {
		var f float64
		if err := json.Unmarshal(*req.Lat, &f); err == nil {
			lat = &f
		}
	}
	if req.Lng != nil && string(*req.Lng) != "null" {
		var f float64
		if err := json.Unmarshal(*req.Lng, &f); err == nil {
			lng = &f
		}
	}

	tag, err := h.DB.UpdateTag(r.Context(), tagID, userID, req.Name, req.Color, req.TextColor, isAuto, condBytes, condGroupBytes, req.ConditionsLogic, req.Description, lat, lng, locationPresent)
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

// GetTagActivities returns a list of activities that have the given tag.
func (h *Handler) GetTagActivities(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	tagID := chi.URLParam(r, "id")

	activities, err := h.DB.GetTagActivities(r.Context(), tagID, userID)
	if err != nil {
		slog.Error("get tag activities failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get tag activities", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, activities)
}

// MergeTag merges a source tag into a target tag.
// POST /api/v1/tags/{id}/merge   body: { "source_tag_id": "uuid" }
// The tag identified by {id} survives; source_tag_id is deleted.
func (h *Handler) MergeTag(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	targetTagID := chi.URLParam(r, "id")

	var req struct {
		SourceTagID string `json:"source_tag_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.SourceTagID == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "source_tag_id is required", r.URL.Path)
		return
	}
	if req.SourceTagID == targetTagID {
		writeError(w, http.StatusBadRequest, "Bad Request", "source and target tag must differ", r.URL.Path)
		return
	}

	tag, err := h.DB.MergeTag(r.Context(), targetTagID, req.SourceTagID, userID)
	if err != nil {
		errMsg := err.Error()
		if errMsg == "target tag not found" || errMsg == "source tag not found" {
			writeError(w, http.StatusNotFound, "Not Found", errMsg, r.URL.Path)
			return
		}
		slog.Error("merge tag failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to merge tags", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, tag)
}

// SyncTagLocation sets location_lat/location_lng on all activities linked to this tag,
// using the tag's own lat/lng coordinates. Only acts when the tag has a location set.
// POST /api/v1/tags/{id}/sync-location
func (h *Handler) SyncTagLocation(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	tagID := chi.URLParam(r, "id")

	// Load the tag to get its coordinates.
	tag, err := h.DB.GetTagByID(r.Context(), tagID, userID)
	if err != nil {
		slog.Error("sync tag location: get tag failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load tag", r.URL.Path)
		return
	}
	if tag == nil {
		writeError(w, http.StatusNotFound, "Not Found", "tag not found", r.URL.Path)
		return
	}
	if tag.Lat == nil || tag.Lng == nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "tag has no location coordinates", r.URL.Path)
		return
	}

	// Fetch all activity IDs linked to this tag.
	activityIDs, err := h.DB.GetTagActivityIDs(r.Context(), tagID)
	if err != nil {
		slog.Error("sync tag location: get activity ids failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get tag activities", r.URL.Path)
		return
	}

	// Update location_lat/location_lng for each linked activity.
	lat := *tag.Lat
	lng := *tag.Lng
	updated := 0
	for _, actID := range activityIDs {
		_, err := h.DB.UpdateActivity(r.Context(), actID, &model.ActivityUpdate{
			LocationLat: &lat,
			LocationLng: &lng,
		})
		if err != nil {
			slog.Warn("sync tag location: update activity failed", "activity_id", actID, "error", err)
			continue
		}
		updated++
	}

	writeJSON(w, http.StatusOK, map[string]int{"updated_count": updated})
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
