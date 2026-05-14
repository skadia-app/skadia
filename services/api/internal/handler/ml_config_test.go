package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/go-chi/chi/v5"
)

func TestPutMLUserSettingRejectsInvalidChatProfileContextWindow(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)

	req := mlUserSettingRequest(userID, "chat_profile", []byte(`{"enabled":true,"base_profile":"custom","overrides":{"max_context_tokens":512}}`))
	rec := httptest.NewRecorder()
	h.PutMLUserSetting(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
}

func TestPutMLUserSettingAcceptsValidChatProfileContextWindow(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)

	req := mlUserSettingRequest(userID, "chat_profile", []byte(`{"enabled":true,"base_profile":"custom","overrides":{"max_context_tokens":16000}}`))
	rec := httptest.NewRecorder()
	h.PutMLUserSetting(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
}

func mlUserSettingRequest(userID, key string, body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPut, "/settings/ml/user/"+key, bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("key", key)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	return req
}
