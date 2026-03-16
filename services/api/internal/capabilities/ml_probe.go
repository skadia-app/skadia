package capabilities

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

// mlServiceAvailable ist der gecachte Status des ML-Service.
// Wird von der Background-Goroutine alle 30s aktualisiert.
var mlServiceAvailable atomic.Bool

// StartMLProbe startet eine Background-Goroutine, die alle 30s den ML-Service
// probt (GET <url>/health, 2s Timeout). Läuft bis ctx abgebrochen wird.
func StartMLProbe(ctx context.Context, mlServiceURL string) {
	go func() {
		client := &http.Client{Timeout: 2 * time.Second}
		probe := func() {
			resp, err := client.Get(mlServiceURL + "/health")
			if err != nil {
				mlServiceAvailable.Store(false)
				slog.Debug("ml probe: unavailable", "error", err)
				return
			}
			resp.Body.Close()
			available := resp.StatusCode == http.StatusOK
			mlServiceAvailable.Store(available)
		}
		probe() // sofort beim Start einmal ausführen
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				probe()
			}
		}
	}()
}

// MLServiceAvailable gibt den letzten gecachten Probe-Status zurück.
func MLServiceAvailable() bool {
	return mlServiceAvailable.Load()
}
