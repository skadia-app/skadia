package capabilities

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

// mlServiceAvailable ist der gecachte Status des ML-Service.
// Wird von der Background-Goroutine alle 30s aktualisiert.
var mlServiceAvailable atomic.Bool
var mlLocalStorageReadAvailable atomic.Bool
var mlS3StorageReadAvailable atomic.Bool
var mlProbeVersion atomic.Uint64

// StartMLProbe startet eine Background-Goroutine, die alle 30s die ML-Readiness
// probt (GET <url>/readyz, 2s Timeout). Läuft bis ctx abgebrochen wird.
func StartMLProbe(ctx context.Context, mlServiceURL string) {
	go func() {
		client := &http.Client{Timeout: 2 * time.Second}
		probe := func() {
			resp, err := client.Get(mlServiceURL + "/readyz")
			if err != nil {
				storeMLProbeState(false, false, false)
				slog.Debug("ml probe: unavailable", "error", err)
				return
			}
			available := resp.StatusCode == http.StatusOK
			localRead, s3Read := parseMLStorageReadiness(resp.Body)
			resp.Body.Close()
			storeMLProbeState(available, localRead, s3Read)
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

func MLLocalStorageReadAvailable() bool {
	return mlLocalStorageReadAvailable.Load()
}

func MLS3StorageReadAvailable() bool {
	return mlS3StorageReadAvailable.Load()
}

func MLProbeVersion() uint64 {
	return mlProbeVersion.Load()
}

func SetMLServiceAvailableForTest(available bool) func() {
	previous := mlServiceAvailable.Load()
	previousLocal := mlLocalStorageReadAvailable.Load()
	previousS3 := mlS3StorageReadAvailable.Load()
	mlServiceAvailable.Store(available)
	mlLocalStorageReadAvailable.Store(available)
	mlS3StorageReadAvailable.Store(available)
	mlProbeVersion.Add(1)
	InvalidateCache()
	return func() {
		mlServiceAvailable.Store(previous)
		mlLocalStorageReadAvailable.Store(previousLocal)
		mlS3StorageReadAvailable.Store(previousS3)
		mlProbeVersion.Add(1)
		InvalidateCache()
	}
}

func storeMLProbeState(service, localRead, s3Read bool) {
	mlServiceAvailable.Store(service)
	mlLocalStorageReadAvailable.Store(service && localRead)
	mlS3StorageReadAvailable.Store(service && s3Read)
	mlProbeVersion.Add(1)
	InvalidateCache()
}

func parseMLStorageReadiness(body io.Reader) (bool, bool) {
	var payload struct {
		ShadowFS struct {
			Status string `json:"status"`
		} `json:"shadowfs"`
		StorageRead map[string]struct {
			Status string `json:"status"`
		} `json:"storage_read"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 64<<10)).Decode(&payload); err != nil {
		return false, false
	}
	localRead := payload.ShadowFS.Status == "ready"
	if state, ok := payload.StorageRead["local"]; ok {
		localRead = state.Status == "ready"
	}
	s3Read := false
	if state, ok := payload.StorageRead["s3"]; ok {
		s3Read = state.Status == "ready"
	}
	return localRead, s3Read
}
