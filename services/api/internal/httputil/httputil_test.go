package httputil

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

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

func TestHandleDBErr_Nil(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	handled := HandleDBErr(w, r, nil, "not found")
	if handled {
		t.Fatal("expected handled=false for nil error")
	}
}

func TestHandleDBErr_NoRows(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	handled := HandleDBErr(w, r, pgx.ErrNoRows, "route not found")
	if !handled {
		t.Fatal("expected handled=true for ErrNoRows")
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", w.Code)
	}
}

func TestHandleDBErr_OtherError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	handled := HandleDBErr(w, r, errors.New("connection refused"), "route not found", "op", "get_route")
	if !handled {
		t.Fatal("expected handled=true for other errors")
	}
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", w.Code)
	}
}

func TestParsePagination(t *testing.T) {
	tests := []struct {
		query      string
		defLimit   int
		maxLimit   int
		wantLimit  int
		wantOffset int
	}{
		{"", 50, 200, 50, 0},
		{"?limit=10&offset=20", 50, 200, 10, 20},
		{"?limit=999", 50, 200, 200, 0},
		{"?limit=-1", 50, 200, 50, 0},
		{"?limit=abc", 50, 200, 50, 0},
	}
	for _, tt := range tests {
		r := httptest.NewRequest("GET", "/test"+tt.query, nil)
		limit, offset := ParsePagination(r, tt.defLimit, tt.maxLimit)
		if limit != tt.wantLimit || offset != tt.wantOffset {
			t.Errorf("query=%q: got limit=%d offset=%d, want %d %d",
				tt.query, limit, offset, tt.wantLimit, tt.wantOffset)
		}
	}
}
