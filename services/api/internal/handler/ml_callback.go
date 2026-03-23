package handler

import (
	"encoding/json"
	"io"
	"net/http"
)

// MLCallbackPayload is the payload from the ML service.
type MLCallbackPayload struct {
	Type   string          `json:"type"` // "enrichment", "analytics", "chat"
	UserID string          `json:"user_id"`
	TaskID string          `json:"task_id"`
	Status string          `json:"status"` // "done", "error"
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// HandleMLCallback handles callbacks from the ML service for remote operation.
// POST /api/v1/internal/ml-callback
// Auth: X-ML-Callback-Secret header must match ML_CALLBACK_SECRET env var
func (h *Handler) HandleMLCallback(w http.ResponseWriter, r *http.Request) {
	secret := ""
	if h.Config != nil {
		secret = h.Config.MLCallbackSecret
	}
	if secret != "" && r.Header.Get("X-ML-Callback-Secret") != secret {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		http.Error(w, "read failed", http.StatusBadRequest)
		return
	}

	var payload MLCallbackPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Route based on type
	switch payload.Type {
	case "analytics":
		// TODO: invalidate analytics_cache for user, notify via notification system
	case "enrichment":
		// TODO: update activity enrichment status
	default:
		// Unknown type — log and accept
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}
