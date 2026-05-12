package agent

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPSSESinkSendFormatsSSEFrame(t *testing.T) {
	rec := httptest.NewRecorder()
	sink := NewHTTPSSESink(rec, rec)

	if err := sink.Send(SSEEvent{Type: EventThinking, ID: "msg-1", Step: ThinkingAnalyze}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	want := `data: {"type":"chat.thinking","id":"msg-1","step":"Analysiere Frage..."}`
	got := strings.TrimSpace(rec.Body.String())
	if got != want {
		t.Fatalf("frame = %q, want %q", got, want)
	}
}

func TestHTTPSSESinkOnChunkReceivesDeltasInOrder(t *testing.T) {
	rec := httptest.NewRecorder()
	sink := NewHTTPSSESink(rec, rec)

	var chunks []string
	sink.OnChunk(func(delta string) {
		chunks = append(chunks, delta)
	})

	for _, delta := range []string{"Hallo", " ", "Demo"} {
		if err := sink.Send(SSEEvent{Type: EventChunk, ID: "msg-1", Delta: delta}); err != nil {
			t.Fatalf("Send %q: %v", delta, err)
		}
	}
	if got, want := strings.Join(chunks, ""), "Hallo Demo"; got != want {
		t.Fatalf("chunks = %q, want %q", got, want)
	}
}

func TestHTTPSSESinkWriteRawFlushesCommentFrame(t *testing.T) {
	rec := httptest.NewRecorder()
	sink := NewHTTPSSESink(rec, rec)

	if err := sink.WriteRaw(": heartbeat\n\n"); err != nil {
		t.Fatalf("WriteRaw: %v", err)
	}
	if got, want := rec.Body.String(), ": heartbeat\n\n"; got != want {
		t.Fatalf("raw frame = %q, want %q", got, want)
	}
}
