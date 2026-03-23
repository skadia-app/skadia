package httputil

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"key": "value"}

	WriteJSON(w, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %s", ct)
	}

	var got map[string]string
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if got["key"] != "value" {
		t.Fatalf("expected key=value, got key=%s", got["key"])
	}
}

func TestWriteJSON_CustomStatus(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusCreated, map[string]int{"id": 42})

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, w.Code)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()

	WriteError(w, http.StatusNotFound, "Not Found", "resource not found", "/api/v1/test")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("expected Content-Type application/problem+json, got %s", ct)
	}

	var problem model.ProblemDetail
	if err := json.NewDecoder(w.Body).Decode(&problem); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if problem.Status != http.StatusNotFound {
		t.Fatalf("expected problem status %d, got %d", http.StatusNotFound, problem.Status)
	}
	if problem.Title != "Not Found" {
		t.Fatalf("expected title 'Not Found', got '%s'", problem.Title)
	}
	if problem.Detail != "resource not found" {
		t.Fatalf("expected detail 'resource not found', got '%s'", problem.Detail)
	}
	if problem.Instance != "/api/v1/test" {
		t.Fatalf("expected instance '/api/v1/test', got '%s'", problem.Instance)
	}
	if problem.Type != "https://skadia.io/errors/Not Found" {
		t.Fatalf("expected type 'https://skadia.io/errors/Not Found', got '%s'", problem.Type)
	}
}

func TestRequireUserID_Present(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(r.Context(), middleware.UserIDKey, "user-123")
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()

	userID, ok := RequireUserID(w, r)
	if !ok || userID != "user-123" {
		t.Fatalf("got userID=%q ok=%v", userID, ok)
	}
}

func TestRequireUserID_Missing(t *testing.T) {
	r := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	_, ok := RequireUserID(w, r)
	if ok {
		t.Fatal("expected ok=false for missing userID")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got status %d, want 401", w.Code)
	}
}

func TestDecodeJSON_Valid(t *testing.T) {
	body := strings.NewReader(`{"name":"test"}`)
	r := httptest.NewRequest("POST", "/", body)
	w := httptest.NewRecorder()

	type req struct {
		Name string `json:"name"`
	}
	val, ok := DecodeJSON[req](w, r)
	if !ok || val.Name != "test" {
		t.Fatalf("got val=%+v ok=%v", val, ok)
	}
}

func TestDecodeJSON_Invalid(t *testing.T) {
	body := strings.NewReader(`{invalid`)
	r := httptest.NewRequest("POST", "/test", body)
	w := httptest.NewRecorder()

	type req struct{ Name string }
	_, ok := DecodeJSON[req](w, r)
	if ok {
		t.Fatal("expected ok=false for invalid JSON")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", w.Code)
	}
}
