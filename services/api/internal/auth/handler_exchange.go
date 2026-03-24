package auth

import (
	"encoding/json"
	"net/http"
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code required"})
		return
	}

	codeHash := HashToken(req.Code)
	userID, _, _, err := h.DB.ConsumeAuthCode(r.Context(), codeHash)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired code"})
		return
	}

	// Check is_active/email_verified after code consumption (C6).
	var isActive, emailVerified bool
	err = h.DB.Pool.QueryRow(r.Context(),
		"SELECT is_active, email_verified FROM users WHERE id = $1", userID).Scan(&isActive, &emailVerified)
	if err != nil || !isActive {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "account_inactive"})
		return
	}
	if h.Config.SMTPHost != "" && !emailVerified {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "email_not_verified"})
		return
	}

	h.issueTokensAndRespond(w, r, userID.String())
}
