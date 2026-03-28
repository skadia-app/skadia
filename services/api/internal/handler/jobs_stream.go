package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	natsgo "github.com/nats-io/nats.go"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

const maxSSEErrorLen = 200

func (h *Handler) collectJobsPayload(ctx context.Context, userID string, includeCounts bool) ([]byte, error) {
	jobs, err := h.DB.ListActiveBackgroundJobs(ctx, userID)
	if err != nil {
		return nil, err
	}
	if garminJobs, err := h.DB.ListActiveGarminImportJobs(ctx, userID); err == nil {
		jobs = append(jobs, garminJobs...)
	}
	if stravaJobs, err := h.DB.ListActiveStravaImportJobs(ctx, userID); err == nil {
		jobs = append(jobs, stravaJobs...)
	}

	for i := range jobs {
		if jobs[i].Error != nil && len(*jobs[i].Error) > maxSSEErrorLen {
			truncated := (*jobs[i].Error)[:maxSSEErrorLen] + "…"
			jobs[i].Error = &truncated
		}
	}

	payload := map[string]any{
		"jobs": jobs,
	}

	if includeCounts {
		if pending, err := h.DB.CountPendingEnrichments(ctx, userID); err == nil {
			payload["pending_enrichments"] = pending
		} else {
			slog.Warn("SSE: count pending enrichments failed", "error", err)
		}
		if unread, err := h.DB.GetUnreadCount(ctx, userID); err == nil {
			payload["unread_notifications"] = unread
		} else {
			slog.Warn("SSE: get unread notification count failed", "error", err)
		}
	}

	return json.Marshal(payload)
}

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

	// Initial send — always include counts so the client has full data immediately.
	if data, err := h.collectJobsPayload(r.Context(), userID, true); err == nil {
		fmt.Fprintf(w, "event: init\ndata: %s\n\n", data)
		flusher.Flush()
	}

	if h.Events == nil {
		jobTicker := time.NewTicker(5 * time.Second)
		defer jobTicker.Stop()
		keepaliveTicker := time.NewTicker(30 * time.Second)
		defer keepaliveTicker.Stop()
		var tickCount int
		for {
			select {
			case <-r.Context().Done():
				return
			case <-jobTicker.C:
				tickCount++
				includeCounts := tickCount%6 == 0
				if data, err := h.collectJobsPayload(r.Context(), userID, includeCounts); err == nil {
					if _, err := fmt.Fprintf(w, "event: init\ndata: %s\n\n", data); err != nil {
						return
					}
					flusher.Flush()
				}
			case <-keepaliveTicker.C:
				if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
					return
				}
				flusher.Flush()
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

	jobTicker := time.NewTicker(5 * time.Second)
	defer jobTicker.Stop()
	keepaliveTicker := time.NewTicker(30 * time.Second)
	defer keepaliveTicker.Stop()
	var tickCount int

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
			if _, err := fmt.Fprintf(w, "event: progress\ndata: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()

		case <-jobTicker.C:
			tickCount++
			includeCounts := tickCount%6 == 0
			if data, err := h.collectJobsPayload(r.Context(), userID, includeCounts); err == nil {
				if _, err := fmt.Fprintf(w, "event: init\ndata: %s\n\n", data); err != nil {
					return
				}
				flusher.Flush()
			}

		case <-keepaliveTicker.C:
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
