package auth

import (
	"encoding/json"
	"net/http"
)

// ExchangeCode handles POST /auth/exchange-code.
// Consumes a short-lived auth code (from social login callback) and issues
// JWT + refresh token pair. This is the BFF auth-code exchange pattern.
func (h *Handler) ExchangeCode(w http.ResponseWriter, r *http.Request) {
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

	h.issueTokensAndRespond(w, r, userID.String())
}
