package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMLCallbackFailsClosedWithoutSecret(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/ml-callback", strings.NewReader(`{"type":"analytics","status":"done"}`))
	rec := httptest.NewRecorder()

	h.HandleMLCallback(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestMLCallbackRequiresConfiguredSecret(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{MLCallbackSecret: "secret"}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/ml-callback", strings.NewReader(`{"type":"analytics","status":"done"}`))
	req.Header.Set("X-ML-Callback-Secret", "wrong")
	rec := httptest.NewRecorder()

	h.HandleMLCallback(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
