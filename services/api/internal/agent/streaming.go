package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

const (
	ThinkingTemplate = "Template-Antwort..."
	ThinkingAnalyze  = "Analysiere Frage..."
	ThinkingHopFmt   = "Schritt %d/%d..."
)

type SSESink interface {
	Send(event SSEEvent) error
	WriteRaw(frame string) error
	OnChunk(fn func(delta string))
}

type httpSSESink struct {
	mu   sync.Mutex
	w    http.ResponseWriter
	f    http.Flusher
	taps []func(string)
}

func NewHTTPSSESink(w http.ResponseWriter, f http.Flusher) SSESink {
	if f == nil {
		if flusher, ok := w.(http.Flusher); ok {
			f = flusher
		}
	}
	return &httpSSESink{w: w, f: f}
}

func (s *httpSSESink) Send(event SSEEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	var taps []func(string)
	s.mu.Lock()
	if _, err := fmt.Fprintf(s.w, "data: %s\n\n", payload); err != nil {
		s.mu.Unlock()
		return err
	}
	if s.f != nil {
		s.f.Flush()
	}
	if event.Type == EventChunk && event.Delta != "" && len(s.taps) > 0 {
		taps = append(taps, s.taps...)
	}
	s.mu.Unlock()

	for _, tap := range taps {
		tap(event.Delta)
	}
	return nil
}

func (s *httpSSESink) WriteRaw(frame string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := fmt.Fprint(s.w, frame); err != nil {
		return err
	}
	if s.f != nil {
		s.f.Flush()
	}
	return nil
}

func (s *httpSSESink) OnChunk(fn func(delta string)) {
	if fn == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taps = append(s.taps, fn)
}
