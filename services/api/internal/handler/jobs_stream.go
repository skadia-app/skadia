package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	natsgo "github.com/nats-io/nats.go"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// StreamJobProgress streams enrichment progress via Server-Sent Events.
//
// GET /api/v1/jobs/stream
func (h *Handler) StreamJobProgress(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		httputil.WriteError(w, http.StatusInternalServerError, "Error", "streaming not supported", r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Initiale Jobs senden bevor wir subscriben, damit der Client sofort Daten hat.
	if jobs, err := h.DB.ListActiveBackgroundJobs(r.Context(), userID); err == nil && len(jobs) > 0 {
		data, _ := json.Marshal(jobs)
		fmt.Fprintf(w, "event: init\ndata: %s\n\n", data)
		flusher.Flush()
	}

	if h.Events == nil {
		// NATS nicht konfiguriert — re-send job list periodically.
		jobTicker := time.NewTicker(5 * time.Second)
		defer jobTicker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-jobTicker.C:
				if jobs, err := h.DB.ListActiveBackgroundJobs(r.Context(), userID); err == nil {
					data, _ := json.Marshal(jobs)
					fmt.Fprintf(w, "event: init\ndata: %s\n\n", data)
					flusher.Flush()
				}
			}
		}
	}

	// Kanal für eingehende NATS-Nachrichten (gepuffert damit der Handler nicht blockiert).
	msgCh := make(chan *natsgo.Msg, 32)

	sub, err := h.Events.Subscribe("skadia.enrichment.progress", func(msg *natsgo.Msg) {
		select {
		case msgCh <- msg:
		default:
			// Kanal voll — Nachricht verwerfen statt blockieren.
		}
	})
	if err != nil {
		slog.Error("SSE NATS subscribe failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Error", "subscription failed", r.URL.Path)
		return
	}
	defer sub.Unsubscribe()

	// Re-send job list periodically so the UI picks up status changes
	// (completed/failed) without relying solely on client-side polling.
	jobTicker := time.NewTicker(5 * time.Second)
	defer jobTicker.Stop()
	keepaliveTicker := time.NewTicker(30 * time.Second)
	defer keepaliveTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return

		case msg := <-msgCh:
			var payload struct {
				ActivityID string `json:"activity_id"`
				Stage      string `json:"stage"`
				Status     string `json:"status"`
				Detail     string `json:"detail"`
				UserID     string `json:"user_id"`
			}
			if err := json.Unmarshal(msg.Data, &payload); err != nil {
				continue
			}
			if payload.UserID != userID {
				continue
			}
			data, _ := json.Marshal(payload)
			fmt.Fprintf(w, "event: progress\ndata: %s\n\n", data)
			flusher.Flush()

		case <-jobTicker.C:
			if jobs, err := h.DB.ListActiveBackgroundJobs(r.Context(), userID); err == nil {
				data, _ := json.Marshal(jobs)
				fmt.Fprintf(w, "event: init\ndata: %s\n\n", data)
				flusher.Flush()
			}

		case <-keepaliveTicker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
