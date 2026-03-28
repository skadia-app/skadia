package auth

import (
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// LinkSocialAccount handles POST /auth/link/{provider} (authenticated).
// TODO: Implement social account linking — allows users to add social login
// to an existing email-based account. See spec Section 11.
func (h *Handler) LinkSocialAccount(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSONNoCache(w, http.StatusNotImplemented, map[string]string{
		"error":   "not_implemented",
		"message": "Account linking will be available in a future update",
	})
}

// UnlinkSocialAccount handles DELETE /auth/link/{provider} (authenticated).
func (h *Handler) UnlinkSocialAccount(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSONNoCache(w, http.StatusNotImplemented, map[string]string{
		"error":   "not_implemented",
		"message": "Account unlinking will be available in a future update",
	})
}
