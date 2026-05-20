package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

type CreateObjectAccessScopeRequest struct {
	ScopeKind string                           `json:"scope_kind,omitempty"`
	Label     string                           `json:"label,omitempty"`
	Grants    []CreateObjectAccessGrantRequest `json:"grants"`
}

type CreateObjectAccessGrantRequest struct {
	PrincipalKind     string         `json:"principal_kind"`
	PrincipalUserID   *string        `json:"principal_user_id,omitempty"`
	PrincipalRef      *string        `json:"principal_ref,omitempty"`
	PrincipalSelector map[string]any `json:"principal_selector,omitempty"`
	LinkToken         *string        `json:"link_token,omitempty"`
	Permission        string         `json:"permission,omitempty"`
}

type ObjectAccessScopeResponse struct {
	Scope      model.ObjectAccessScope      `json:"scope"`
	LinkTokens []ObjectAccessLinkTokenValue `json:"link_tokens,omitempty"`
}

type ObjectAccessLinkTokenValue struct {
	GrantID string `json:"grant_id"`
	Token   string `json:"token"`
}

type UpdateObjectAccessScopeRequest struct {
	RevokeScope    bool     `json:"revoke_scope,omitempty"`
	RevokeGrantIDs []string `json:"revoke_grant_ids,omitempty"`
}

func (h *Handler) CreateObjectAccessScope(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	req, ok := httputil.DecodeJSON[CreateObjectAccessScopeRequest](w, r)
	if !ok {
		return
	}
	input := model.ObjectAccessScopeInput{
		OwnerUserID: userID,
		ScopeKind:   req.ScopeKind,
		Label:       req.Label,
		CreatedBy:   userID,
		Grants:      make([]model.ObjectAccessGrantInput, 0, len(req.Grants)),
	}
	rawTokens := make([]string, 0, len(req.Grants))
	for _, grant := range req.Grants {
		permission := strings.TrimSpace(grant.Permission)
		if permission != "" && permission != "view" {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "object access grants are view-only", r.URL.Path)
			return
		}
		grantInput := model.ObjectAccessGrantInput{
			PrincipalKind:     grant.PrincipalKind,
			PrincipalUserID:   grant.PrincipalUserID,
			PrincipalRef:      grant.PrincipalRef,
			PrincipalSelector: grant.PrincipalSelector,
			Permission:        permission,
		}
		if strings.TrimSpace(grant.PrincipalKind) == "link_token" {
			token := ""
			if grant.LinkToken != nil {
				token = strings.TrimSpace(*grant.LinkToken)
			}
			if token == "" {
				generated, err := generateObjectAccessLinkToken()
				if err != nil {
					httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to generate link token", r.URL.Path)
					return
				}
				token = generated
			}
			hash := hashObjectAccessLinkToken(token)
			grantInput.LinkTokenHash = &hash
			rawTokens = append(rawTokens, token)
		}
		input.Grants = append(input.Grants, grantInput)
	}

	scope, err := h.DB.CreateObjectAccessScope(r.Context(), input)
	if errors.Is(err, database.ErrObjectAccessValidation) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}
	if httputil.HandleDBErr(w, r, err, "access scope not found", "user_id", userID) {
		return
	}
	response := ObjectAccessScopeResponse{Scope: *scope}
	linkIndex := 0
	for _, grant := range scope.Grants {
		if grant.PrincipalKind != "link_token" {
			continue
		}
		if linkIndex < len(rawTokens) {
			response.LinkTokens = append(response.LinkTokens, ObjectAccessLinkTokenValue{
				GrantID: grant.ID,
				Token:   rawTokens[linkIndex],
			})
			linkIndex++
		}
	}
	httputil.WriteJSON(w, http.StatusCreated, response)
}

func (h *Handler) ListObjectAccessScopes(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	scopes, err := h.DB.ListObjectAccessScopes(r.Context(), userID)
	if httputil.HandleDBErr(w, r, err, "access scopes not found", "user_id", userID) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"scopes": scopes})
}

func (h *Handler) UpdateObjectAccessScope(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	scopeID := chi.URLParam(r, "id")
	req, ok := httputil.DecodeJSON[UpdateObjectAccessScopeRequest](w, r)
	if !ok {
		return
	}
	if scopeID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "missing access scope id", r.URL.Path)
		return
	}
	if len(req.RevokeGrantIDs) > 0 {
		if err := h.DB.RevokeObjectAccessGrants(r.Context(), scopeID, userID, req.RevokeGrantIDs); err != nil {
			handleObjectAccessUpdateError(w, r, err)
			return
		}
	}
	if req.RevokeScope {
		if err := h.DB.RevokeObjectAccessScope(r.Context(), scopeID, userID); err != nil {
			handleObjectAccessUpdateError(w, r, err)
			return
		}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func handleObjectAccessUpdateError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, database.ErrObjectAccessValidation) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}
	if errors.Is(err, database.ErrObjectAccessNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "access scope not found", r.URL.Path)
		return
	}
	httputil.HandleDBErr(w, r, err, "access scope not found")
}

func generateObjectAccessLinkToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashObjectAccessLinkToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return "sha256:" + hex.EncodeToString(sum[:])
}
