package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInferenceEmbedText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embed/text" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req["text"] != "trail lake" {
			t.Fatalf("unexpected text: %#v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float64{0.1, 0.2}, "dim": 2})
	}))
	defer server.Close()

	got, err := embedText(context.Background(), server.URL, "trail lake")
	if err != nil {
		t.Fatalf("embedText: %v", err)
	}
	if len(got) != 2 || got[0] != 0.1 || got[1] != 0.2 {
		t.Fatalf("unexpected embedding: %#v", got)
	}
}

func TestPGVectorLiteral(t *testing.T) {
	got, err := pgVectorLiteral([]float64{0.1, -2, 3e-6})
	if err != nil {
		t.Fatalf("pgVectorLiteral: %v", err)
	}
	if got != "[0.1,-2,3e-06]" {
		t.Fatalf("unexpected vector literal: %s", got)
	}
}
