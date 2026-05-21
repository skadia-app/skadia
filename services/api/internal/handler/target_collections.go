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
	collections, err := h.DB.ListTargetCollections(r.Context(), model.ListTargetCollectionsParams{
		UserID:                userID,
		IsAdmin:               middleware.GetIsAdmin(r.Context()),
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
	scopes := h.resolveTargetVisibleScopes(w, r, userID)
	if scopes == nil && userID != "" {
		return
	}
	collection, err := h.DB.GetTargetCollection(r.Context(), userID, chi.URLParam(r, "id"), scopes, r.URL.Query().Get("include_members") == "true")
	if handleTargetHandlerErr(w, r, err) {
		return
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
