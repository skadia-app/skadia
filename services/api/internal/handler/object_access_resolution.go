package handler

import (
	"net/http"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

func (h *Handler) resolveVisibleAccessScopesForRequest(w http.ResponseWriter, r *http.Request, userID string) []string {
	linkToken := strings.TrimSpace(r.URL.Query().Get("link_token"))
	if linkToken == "" {
		linkToken = strings.TrimSpace(r.Header.Get("X-Skadia-Link-Token"))
	}
	var linkTokenHash *string
	if linkToken != "" {
		hash := hashObjectAccessLinkToken(linkToken)
		linkTokenHash = &hash
	}
	if strings.TrimSpace(userID) == "" && linkTokenHash == nil {
		return []string{}
	}
	scopes, err := h.DB.ResolveVisibleAccessScopes(r.Context(), userID, linkTokenHash, nil)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to resolve access scopes", r.URL.Path)
		return nil
	}
	if scopes == nil {
		return []string{}
	}
	return scopes
}
