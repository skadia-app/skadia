package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	natsgo "github.com/nats-io/nats.go"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// sseConnections tracks active SSE connections per user to prevent resource
// exhaustion (each connection holds a goroutine + NATS subscription + DB polling).
var sseConnections struct {
	sync.Mutex
	count map[string]int
}

func init() {
	sseConnections.count = make(map[string]int)
}

const maxSSEPerUser = 3

func sseAcquire(userID string) bool {
	sseConnections.Lock()
	defer sseConnections.Unlock()
	if sseConnections.count[userID] >= maxSSEPerUser {
		return false
	}
	sseConnections.count[userID]++
	return true
}

func sseRelease(userID string) {
	sseConnections.Lock()
	defer sseConnections.Unlock()
	sseConnections.count[userID]--
	if sseConnections.count[userID] <= 0 {
		delete(sseConnections.count, userID)
	}
}

const maxSSEErrorLen = 200

func (h *Handler) collectJobsPayload(ctx context.Context, userID string, includeCounts bool) ([]byte, error) {
	jobs, err := h.DB.ListActiveBackgroundJobs(ctx, userID)
	if err != nil {
		return nil, err
	}
	if garminJobs, err := h.DB.ListActiveGarminImportJobs(ctx, userID); err == nil {
		jobs = append(jobs, garminJobs...)
	} else {
		slog.Warn("SSE: list garmin import jobs failed", "error", err)
	}
	if stravaJobs, err := h.DB.ListActiveStravaImportJobs(ctx, userID); err == nil {
		jobs = append(jobs, stravaJobs...)
	} else {
		slog.Warn("SSE: list strava import jobs failed", "error", err)
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

	// Limit concurrent SSE connections per user (prevents DoS via rapid reconnects)
	if !sseAcquire(userID) {
		httputil.WriteError(w, http.StatusTooManyRequests, "Too Many Requests",
			"too many SSE connections", r.URL.Path)
		return
	}
	defer sseRelease(userID)

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
		revalidateTicker := time.NewTicker(5 * time.Minute)
		defer revalidateTicker.Stop()
		var tickCount int
		for {
			select {
			case <-r.Context().Done():
				return
			case <-revalidateTicker.C:
				var isActive bool
				if err := h.DB.Pool.QueryRow(r.Context(),
					"SELECT is_active FROM users WHERE id = $1", userID,
				).Scan(&isActive); err != nil {
					// Transient DB error — log and continue, don't kill the stream
					slog.Warn("SSE: user revalidation query failed", "user_id", userID, "error", err)
				} else if !isActive {
					slog.Info("SSE: closing stream for deactivated user", "user_id", userID)
					return
				}
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
	revalidateTicker := time.NewTicker(5 * time.Minute)
	defer revalidateTicker.Stop()
	var tickCount int

	for {
		select {
		case <-r.Context().Done():
			return

		case <-revalidateTicker.C:
			var isActive bool
			if err := h.DB.Pool.QueryRow(r.Context(),
				"SELECT is_active FROM users WHERE id = $1", userID,
			).Scan(&isActive); err != nil {
				// Transient DB error — log and continue, don't kill the stream
				slog.Warn("SSE: user revalidation query failed", "user_id", userID, "error", err)
			} else if !isActive {
				slog.Info("SSE: closing stream for deactivated user", "user_id", userID)
				return
			}

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
