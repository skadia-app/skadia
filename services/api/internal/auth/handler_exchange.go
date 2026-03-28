package auth

import (
	"encoding/json"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// ExchangeCode handles POST /auth/exchange-code.
// Consumes a short-lived auth code (from social login callback) and issues
// JWT + refresh token pair. This is the BFF auth-code exchange pattern.
func (h *Handler) ExchangeCode(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r)

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "code required"})
		return
	}

	codeHash := HashToken(req.Code)
	userID, _, _, err := h.DB.ConsumeAuthCode(r.Context(), codeHash)
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired code"})
		return
	}

	// Check is_active after code consumption (C6).
	user, err := h.DB.GetUserByID(r.Context(), userID.String())
	if err != nil || !user.IsActive {
		httputil.WriteJSONNoCache(w, http.StatusForbidden, map[string]string{"error": "account_inactive"})
		return
	}
	h.issueTokensAndRespond(w, r, user)
}
