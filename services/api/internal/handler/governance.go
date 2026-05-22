package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (h *Handler) ListGovernanceScopes(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	scopes, err := h.DB.ListGovernanceScopes(r.Context(), userID, middleware.GetIsAdmin(r.Context()), model.GovernanceScopeFilter{
		Kind:               r.URL.Query().Get("kind"),
		AnchorTargetKind:   r.URL.Query().Get("anchor_target_kind"),
		AnchorTargetID:     r.URL.Query().Get("anchor_target_id"),
		AnchorCollectionID: r.URL.Query().Get("anchor_collection_id"),
		OwnerScope:         r.URL.Query().Get("owner_scope"),
		Limit:              parseIntDefault(r.URL.Query().Get("limit"), 100),
		Offset:             parseIntDefault(r.URL.Query().Get("offset"), 0),
	})
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"scopes": scopes})
}

func (h *Handler) CreateGovernanceScope(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	input, ok := httputil.DecodeJSON[model.GovernanceScopeCreate](w, r)
	if !ok {
		return
	}
	scope, err := h.DB.CreateGovernanceScopeAsActor(r.Context(), userID, middleware.GetIsAdmin(r.Context()), input)
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"scope": scope})
}

func (h *Handler) GetGovernanceScope(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	scope, err := h.DB.GetGovernanceScope(r.Context(), userID, middleware.GetIsAdmin(r.Context()), chi.URLParam(r, "id"))
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"scope": scope})
}

func (h *Handler) UpdateGovernanceScope(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	input, ok := httputil.DecodeJSON[model.GovernanceScopeUpdate](w, r)
	if !ok {
		return
	}
	scope, err := h.DB.UpdateGovernanceScope(r.Context(), userID, chi.URLParam(r, "id"), input)
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"scope": scope})
}

func (h *Handler) AddGovernanceRole(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	input, ok := httputil.DecodeJSON[model.GovernanceActorRoleCreate](w, r)
	if !ok {
		return
	}
	role, err := h.DB.AddGovernanceActorRole(r.Context(), userID, chi.URLParam(r, "id"), input)
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"role": role})
}

func (h *Handler) UpdateGovernanceRole(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	input, ok := httputil.DecodeJSON[model.GovernanceActorRoleUpdate](w, r)
	if !ok {
		return
	}
	role, err := h.DB.UpdateGovernanceActorRole(r.Context(), userID, chi.URLParam(r, "id"), chi.URLParam(r, "role_id"), input)
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"role": role})
}

func (h *Handler) AddGovernanceBinding(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	input, ok := httputil.DecodeJSON[model.GovernanceScopeTargetBindingCreate](w, r)
	if !ok {
		return
	}
	binding, err := h.DB.AddGovernanceScopeTargetBinding(r.Context(), userID, chi.URLParam(r, "id"), input)
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"binding": binding})
}

func (h *Handler) UpdateGovernanceBinding(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	input, ok := httputil.DecodeJSON[model.GovernanceScopeTargetBindingUpdate](w, r)
	if !ok {
		return
	}
	binding, err := h.DB.UpdateGovernanceScopeTargetBinding(r.Context(), userID, chi.URLParam(r, "id"), chi.URLParam(r, "binding_id"), input)
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"binding": binding})
}

func (h *Handler) ListGovernanceEffectiveTargets(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	targets, err := h.DB.ListGovernanceEffectiveTargets(r.Context(), model.GovernanceActor{
		ActorType:   "user",
		ActorUserID: &userID,
	}, model.GovernanceEffectiveTargetFilter{
		TargetKind:       r.URL.Query().Get("target_kind"),
		TargetID:         r.URL.Query().Get("target_id"),
		Permission:       r.URL.Query().Get("permission"),
		PermissionsScope: r.URL.Query().Get("permissions_scope"),
		Limit:            parseIntDefault(r.URL.Query().Get("limit"), 100),
		Offset:           parseIntDefault(r.URL.Query().Get("offset"), 0),
	})
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"targets": targets})
}
