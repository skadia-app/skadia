package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

func TestChatPreferencesDefaultsAndPut(t *testing.T) {
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)
	h := newTestHandler(t, pool)

	req := chatPrefsRequest(t, http.MethodGet, "/preferences", userID, nil)
	w := httptest.NewRecorder()
	h.ChatRoutes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", w.Code, w.Body.String())
	}
	var defaults chatPreferencesResponse
	if err := json.NewDecoder(w.Body).Decode(&defaults); err != nil {
		t.Fatalf("decode defaults: %v", err)
	}
	if defaults.ChatExposeDocumentExtractedData || defaults.ChatWriteToolsEnabled || len(defaults.ChatAllowedWriteTools) != 0 {
		t.Fatalf("unexpected defaults: %#v", defaults)
	}

	body := []byte(`{
		"chat_expose_document_extracted_data": true,
		"chat_write_tools_enabled": true,
		"chat_allowed_write_tools": ["rename_activity", "set_activity_gear", "rename_activity"]
	}`)
	req = chatPrefsRequest(t, http.MethodPut, "/preferences", userID, body)
	w = httptest.NewRecorder()
	h.ChatRoutes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", w.Code, w.Body.String())
	}
	var saved chatPreferencesResponse
	if err := json.NewDecoder(w.Body).Decode(&saved); err != nil {
		t.Fatalf("decode saved: %v", err)
	}
	if !saved.ChatExposeDocumentExtractedData || !saved.ChatWriteToolsEnabled {
		t.Fatalf("saved flags not set: %#v", saved)
	}
	if got := saved.ChatAllowedWriteTools; len(got) != 2 || got[0] != "rename_activity" || got[1] != "set_activity_gear" {
		t.Fatalf("saved allowed tools = %#v", got)
	}
}

func TestChatPreferencesRejectsUnknownWriteTool(t *testing.T) {
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)
	h := newTestHandler(t, pool)

	body := []byte(`{
		"chat_write_tools_enabled": true,
		"chat_allowed_write_tools": ["rename_activity", "delete_activity"]
	}`)
	req := chatPrefsRequest(t, http.MethodPut, "/preferences", userID, body)
	w := httptest.NewRecorder()
	h.ChatRoutes().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PUT status = %d body=%s, want 400", w.Code, w.Body.String())
	}
}

func TestChatPreferencesRequiresUser(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)

	req := httptest.NewRequest(http.MethodGet, "/preferences", nil)
	w := httptest.NewRecorder()
	h.ChatRoutes().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("GET status = %d body=%s, want 401", w.Code, w.Body.String())
	}
}

func chatPrefsRequest(t *testing.T, method, target, userID string, body []byte) *http.Request {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if userID != "" {
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
		req = req.WithContext(ctx)
	}
	return req
}
