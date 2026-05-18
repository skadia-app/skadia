package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/systemstatus"
	natsgo "github.com/nats-io/nats.go"
)

var expectedNATSStreams = []string{"SKADIA", "stage2", "document", "analytics"}

var expectedNATSConsumers = map[string][]string{
	"stage2":    {"ml-stage2-high", "ml-stage2-normal", "ml-stage2-low"},
	"document":  {"ml-document-processor"},
	"analytics": {"ml-analytics-invalidate"},
	"SKADIA":   {"ml-photo-embed"},
}

type jetStreamProvider interface {
	JetStream() natsgo.JetStreamContext
}

func (h *Handler) collectNATSComponent(ctx context.Context) systemstatus.ComponentStatus {
	snap := systemstatus.NATSSnapshot{CheckedAt: time.Now().UTC()}
	if h != nil && h.Config != nil && strings.TrimSpace(h.Config.NatsURL) != "" {
		snap.Configured = true
	}
	if h.EventConn == nil {
		return systemstatus.NATSStatusFromSnapshot(snap)
	}
	snap.Configured = true
	snap.Connected = h.EventConn.IsConnected()
	if !snap.Connected {
		return systemstatus.NATSStatusFromSnapshot(snap)
	}
	provider, ok := h.EventConn.(jetStreamProvider)
	if !ok {
		snap.JetStreamOK = true
		snap.IntrospectionAvailable = false
		return systemstatus.NATSStatusFromSnapshot(snap)
	}
	js := provider.JetStream()
	if js == nil {
		return systemstatus.NATSStatusFromSnapshot(snap)
	}
	snap.JetStreamOK = true
	snap.IntrospectionAvailable = true
	snap.StreamPresent = true
	snap.ConsumersPresent = true
	for _, stream := range expectedNATSStreams {
		if _, err := js.StreamInfo(stream); err != nil {
			snap.StreamPresent = false
			continue
		}
		for _, consumer := range expectedNATSConsumers[stream] {
			info, err := js.ConsumerInfo(stream, consumer)
			if err != nil {
				snap.ConsumersPresent = false
				continue
			}
			snap.PendingMessages += int(info.NumPending)
			snap.Redelivered += int(info.NumRedelivered)
		}
	}
	return systemstatus.NATSStatusFromSnapshot(snap)
}

func (h *Handler) collectMLComponent(ctx context.Context) systemstatus.ComponentStatus {
	snap, _ := h.collectMLSnapshot(ctx)
	return systemstatus.MLStatusFromSnapshot(snap)
}

func (h *Handler) collectMLSnapshot(ctx context.Context) (systemstatus.MLSnapshot, *systemstatus.BuildInfo) {
	snap := systemstatus.MLSnapshot{CheckedAt: time.Now().UTC()}
	if h == nil || h.Config == nil || h.Config.MLServiceURL == "" || h.coreProfile() {
		return snap, nil
	}
	snap.Configured = true
	client := h.AIBrokerHTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	base := strings.TrimRight(h.Config.MLServiceURL, "/")
	snap.Live = probeHTTPStatusOK(ctx, client, base+"/livez", "")
	snap.Ready = probeHTTPStatusOK(ctx, client, base+"/readyz", "")
	if snap.Live {
		snap.StatusChecked = true
		if h.Config.MLCallbackSecret == "" {
			return snap, nil
		}
		var payload struct {
			Service       string `json:"service"`
			GitCommit     string `json:"git_commit"`
			CommitTime    string `json:"commit_time"`
			BuildTime     string `json:"build_time"`
			ImageDigest   string `json:"image_digest"`
			ConfigVersion string `json:"config_version"`
			Metrics       any    `json:"metrics"`
			LLMBroker     struct {
				Status string            `json:"status"`
				Roles  map[string]string `json:"roles"`
			} `json:"llm_broker"`
		}
		if getJSONWithInternalSecret(ctx, client, base+"/status", h.Config.MLCallbackSecret, &payload) == nil {
			snap.StatusAccessible = true
			snap.Version = payload.GitCommit
			snap.Build = systemstatus.BuildInfo{
				Service:       payload.Service,
				GitCommit:     payload.GitCommit,
				CommitTime:    payload.CommitTime,
				BuildTime:     payload.BuildTime,
				ImageDigest:   payload.ImageDigest,
				ConfigVersion: payload.ConfigVersion,
			}
			if snap.Build.Service == "" {
				snap.Build.Service = "ml"
			}
			snap.MetricsAvailable = payload.Metrics != nil
			snap.LLMBrokerStatus = payload.LLMBroker.Status
			snap.RoleStatuses = payload.LLMBroker.Roles
			for role, state := range payload.LLMBroker.Roles {
				if state != "configured" {
					snap.MissingRoles = append(snap.MissingRoles, role)
				}
			}
		}
	}
	if snap.StatusAccessible {
		return snap, &snap.Build
	}
	return snap, nil
}

func (h *Handler) collectBRouterComponent(ctx context.Context) systemstatus.ComponentStatus {
	snap := systemstatus.BRouterSnapshot{CheckedAt: time.Now().UTC()}
	if h != nil && h.Config != nil {
		caps := capabilities.Detect(ctx, nil, h.detectInputForGate())
		if state, ok := caps.Features[capabilities.KeyBRouter]; ok {
			snap.Reachable = state.Available
		}
		snap.ProfileCount = countFilesWithSuffix(h.Config.BRouterProfilesDir, ".brf", false)
		snap.SegmentCount = countFilesWithSuffix(h.Config.BRouterSegmentsDir, ".rd5", true)
	}
	return systemstatus.BRouterStatusFromSnapshot(snap)
}

func (h *Handler) collectBadRouterComponent(ctx context.Context) systemstatus.ComponentStatus {
	snap := systemstatus.BadRouterSnapshot{CheckedAt: time.Now().UTC()}
	if h != nil && h.Config != nil {
		tileDir := filepath.Join(filepath.Dir(h.Config.RoutingGraphPath), "tiles")
		snap.TileCount = countFilesWithPrefixSuffix(tileDir, "tile_", ".bin")
		snap.TileFilesPresent = snap.TileCount > 0
		snap.ProfileCount = countFilesWithSuffix(h.Config.BRouterProfilesDir, ".brf", false)
	}
	if h != nil && h.GraphManager != nil && h.GraphManager.UnifiedRouter() != nil {
		snap.VerifiedSampleRoute = true
		if tm := h.GraphManager.TileManager(); tm != nil {
			snap.TileCount = tm.TileCount()
			snap.TileFilesPresent = snap.TileCount > 0
		}
	}
	return systemstatus.BadRouterStatusFromSnapshot(snap)
}

func (h *Handler) collectOSMComponent(ctx context.Context) systemstatus.ComponentStatus {
	snap := systemstatus.OSMSnapshot{CheckedAt: time.Now().UTC()}
	if h.DB != nil && h.DB.Pool != nil {
		_ = h.DB.Pool.QueryRow(ctx, `
			SELECT COUNT(*), COALESCE(MAX(status), '')
			FROM osm_import_regions`).Scan(&snap.RegionCount, &snap.LastImportStatus)
	}
	return systemstatus.OSMStatusFromSnapshot(snap)
}

func (h *Handler) collectDEMComponent(ctx context.Context) systemstatus.ComponentStatus {
	snap := systemstatus.DEMSnapshot{CheckedAt: time.Now().UTC()}
	if h.DB != nil && h.DB.Pool != nil {
		_ = h.DB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM dem.raster_tiles`).Scan(&snap.TileCount)
	}
	return systemstatus.DEMStatusFromSnapshot(snap)
}

func (h *Handler) collectCoverageComponent(ctx context.Context) systemstatus.ComponentStatus {
	snap := systemstatus.CoverageSnapshot{CheckedAt: time.Now().UTC()}
	if h != nil {
		snap.LayerCount = countFilesWithSuffix(h.CoverageBaseDir, ".bin", true)
		if h.DEMSvc != nil {
			health := h.DEMSvc.Health()
			snap.CorruptCellCount = len(health.CorruptCells)
		}
	}
	return systemstatus.CoverageStatusFromSnapshot(snap)
}

func (h *Handler) collectTilesComponent(ctx context.Context) systemstatus.ComponentStatus {
	snap := systemstatus.PMTilesSnapshot{CheckedAt: time.Now().UTC()}
	if h != nil && h.Config != nil {
		snap.FileCount = countFilesWithSuffix(h.Config.TilesDir, ".pmtiles", false)
	}
	return systemstatus.PMTilesStatusFromSnapshot(snap)
}

func probeHTTPStatusOK(ctx context.Context, client *http.Client, url, token string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	if token != "" {
		req.Header.Set("X-ML-Callback-Secret", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
}

func getJSONWithInternalSecret(ctx context.Context, client *http.Client, url, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-ML-Callback-Secret", token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return http.ErrNotSupported
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(out)
}

func countFilesWithPrefixSuffix(dir, prefix, suffix string) int {
	if dir == "" {
		return 0
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var count int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) {
			count++
		}
	}
	return count
}

func countFilesWithSuffix(dir, suffix string, recursive bool) int {
	if dir == "" {
		return 0
	}
	if !recursive {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return 0
		}
		var count int
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
				count++
			}
		}
		return count
	}
	var count int
	_ = filepath.WalkDir(dir, func(_ string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(entry.Name(), suffix) {
			count++
		}
		return nil
	})
	return count
}
