package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (h *Handler) ListTargetCollections(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	scopes := h.resolveTargetVisibleScopes(w, r, userID)
	if scopes == nil && userID != "" {
		return
	}
	visibilityMode, ok := parseTargetVisibilityMode(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Has("include_geometry") {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "include_geometry is not supported for target collection lists", r.URL.Path)
		return
	}
	collections, err := h.DB.ListTargetCollections(r.Context(), model.ListTargetCollectionsParams{
		UserID:                userID,
		IsAdmin:               middleware.GetIsAdmin(r.Context()),
		VisibilityMode:        visibilityMode,
		VisibleAccessScopeIDs: scopes,
		CollectionKinds:       splitCSV(r.URL.Query().Get("kinds")),
		BBox:                  parseTargetBBox(r.URL.Query().Get("bbox")),
		Search:                r.URL.Query().Get("search"),
		Limit:                 parseIntDefault(r.URL.Query().Get("limit"), 100),
		Offset:                parseIntDefault(r.URL.Query().Get("offset"), 0),
	})
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"collections": collections})
}

func (h *Handler) CreateTargetCollection(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	input, ok := httputil.DecodeJSON[model.TargetCollectionCreate](w, r)
	if !ok {
		return
	}
	collection, err := h.DB.CreateTargetCollection(r.Context(), userID, input)
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"collection": collection})
}

func (h *Handler) GetTargetCollection(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	isAdmin := middleware.GetIsAdmin(r.Context())
	scopes := h.resolveTargetVisibleScopes(w, r, userID)
	if scopes == nil && userID != "" {
		return
	}
	collection, err := h.DB.GetTargetCollectionWithAccess(r.Context(), userID, chi.URLParam(r, "id"), scopes, isAdmin, r.URL.Query().Get("include_members") == "true")
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	if r.URL.Query().Get("include_overlay_summary") == "true" && collection != nil {
		summary, err := h.DB.GetTargetOverlaySummary(r.Context(), "collection", collection.ID, nil)
		if handleTargetHandlerErr(w, r, err) {
			return
		}
		collection.OverlaySummary = summary
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"collection": collection})
}

func (h *Handler) UpdateTargetCollection(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	input, ok := httputil.DecodeJSON[model.TargetCollectionUpdate](w, r)
	if !ok {
		return
	}
	collection, err := h.DB.UpdateTargetCollection(r.Context(), userID, chi.URLParam(r, "id"), input)
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"collection": collection})
}

func (h *Handler) DeleteTargetCollection(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	if err := h.DB.ArchiveTargetCollection(r.Context(), userID, chi.URLParam(r, "id")); handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) AddTargetCollectionMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	input, ok := httputil.DecodeJSON[model.TargetCollectionMemberCreate](w, r)
	if !ok {
		return
	}
	member, err := h.DB.AddTargetCollectionMember(r.Context(), userID, chi.URLParam(r, "id"), input)
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"member": member})
}

func (h *Handler) UpdateTargetCollectionMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	input, ok := httputil.DecodeJSON[model.TargetCollectionMemberUpdate](w, r)
	if !ok {
		return
	}
	member, err := h.DB.UpdateTargetCollectionMember(r.Context(), userID, chi.URLParam(r, "id"), chi.URLParam(r, "member_id"), input)
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"member": member})
}

func (h *Handler) DeleteTargetCollectionMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	if err := h.DB.RemoveTargetCollectionMember(r.Context(), userID, chi.URLParam(r, "id"), chi.URLParam(r, "member_id")); handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
