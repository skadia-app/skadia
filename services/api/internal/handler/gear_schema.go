package handler

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// gearSchemaResponse is the wire format for GET /api/gear/schema.
// Version 1 aligns with the initial gear schema generation (Phase 1).
// Top-level keys use camelCase to match the frontend GearSchema TypeScript interface.
type gearSchemaResponse struct {
	Version         int         `json:"version"`
	AssignmentRoles interface{} `json:"assignmentRoles"`
	OptionSets      interface{} `json:"optionSets"`
	Attributes      interface{} `json:"attributes"`
	GearTypes       interface{} `json:"gearTypes"`
}

// GetGearSchema returns the effective gear schema for the authenticated user.
//
// The effective schema is the generated base schema overlaid with the user's
// own customizations from gear_schema_definitions. The response includes an
// ETag derived from both the base schema content and the user-override content,
// so the client cache is invalidated on either change.
//
// Supports If-None-Match for conditional GET (RFC 7232).
func (h *Handler) GetGearSchema(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	schema, err := h.DB.GetEffectiveGearSchema(r.Context(), userID)
	if err != nil {
		slog.Error("get effective gear schema failed", "user_id", userID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load gear schema", r.URL.Path)
		return
	}

	resp := gearSchemaResponse{
		Version:         1,
		AssignmentRoles: schema.AssignmentRoles,
		OptionSets:      schema.OptionSets,
		Attributes:      schema.Attributes,
		GearTypes:       schema.GearTypes,
	}

	body, err := json.Marshal(resp)
	if err != nil {
		slog.Error("marshal gear schema failed", "user_id", userID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to encode gear schema", r.URL.Path)
		return
	}

	sum := sha256.Sum256(body)
	etag := fmt.Sprintf(`"%x"`, sum[:])

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-cache, must-revalidate")
	w.Header().Set("ETag", etag)

	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		slog.Error("write gear schema response failed", "user_id", userID, "error", err)
	}
}
