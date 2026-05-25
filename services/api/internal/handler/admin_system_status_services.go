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

	natsgo "github.com/nats-io/nats.go"
	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/systemstatus"
)

type expectedNATSQueue struct {
	stream    string
	subject   string
	consumers []string
}

var expectedNATSCoreQueues = []expectedNATSQueue{
	{stream: "activity", subject: "skadia.activity.uploaded", consumers: []string{"pmc-calculator"}},
	{stream: "photo", subject: "skadia.photo.embed", consumers: []string{"ml-photo-embed"}},
}

var expectedNATSMLQueues = []expectedNATSQueue{
	{stream: "stage2", subject: "skadia.stage2.high", consumers: []string{"ml-stage2-high", "ml-stage2-normal", "ml-stage2-low"}},
	{stream: "document", subject: "skadia.document.uploaded", consumers: []string{"ml-document-processor"}},
	{stream: "analytics", subject: "skadia.analytics.invalidate", consumers: []string{"ml-analytics-invalidate"}},
}

type jetStreamProvider interface {
	JetStream() natsgo.JetStreamContext
}

type natsStreamInspector interface {
	StreamInfo(stream string, opts ...natsgo.JSOpt) (*natsgo.StreamInfo, error)
	StreamNameBySubject(subject string, opts ...natsgo.JSOpt) (string, error)
	ConsumerInfo(stream, consumer string, opts ...natsgo.JSOpt) (*natsgo.ConsumerInfo, error)
}

func (h *Handler) collectNATSComponent(ctx context.Context, expectMLConsumers bool) systemstatus.ComponentStatus {
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
	collectNATSJetStreamSnapshot(&snap, js, expectMLConsumers)
	return systemstatus.NATSStatusFromSnapshot(snap)
}

func collectNATSJetStreamSnapshot(snap *systemstatus.NATSSnapshot, js natsStreamInspector, expectMLConsumers bool) {
	snap.JetStreamOK = true
	snap.IntrospectionAvailable = true
	snap.StreamPresent = true
	snap.ConsumersPresent = true
	queues := append([]expectedNATSQueue{}, expectedNATSCoreQueues...)
	if expectMLConsumers {
		queues = append(queues, expectedNATSMLQueues...)
	}
	for _, queue := range queues {
		stream, ok := resolveNATSStream(js, queue)
		if !ok {
			snap.StreamPresent = false
			continue
		}
		if !expectMLConsumers {
			continue
		}
		for _, consumer := range queue.consumers {
			info, err := js.ConsumerInfo(stream, consumer)
			if err != nil {
				snap.ConsumersPresent = false
				continue
			}
			snap.PendingMessages += int(info.NumPending)
			snap.Redelivered += int(info.NumRedelivered)
		}
	}
}

func resolveNATSStream(js natsStreamInspector, queue expectedNATSQueue) (string, bool) {
	if stream, err := js.StreamNameBySubject(queue.subject); err == nil && stream != "" {
		if _, infoErr := js.StreamInfo(stream); infoErr == nil {
			return stream, true
		}
	}
	if _, err := js.StreamInfo(queue.stream); err == nil {
		return queue.stream, true
	}
	return "", false
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

func (h *Handler) collectBRouterComponent(ctx context.Context, caps capabilities.Capabilities) systemstatus.ComponentStatus {
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

func (h *Handler) collectBadRouterComponent(ctx context.Context, caps capabilities.Capabilities) systemstatus.ComponentStatus {
	if disabled, ok := disabledComponentFromCapability(caps, capabilities.KeyBadRouter, "badrouter", "Skadia routing graph"); ok {
		return disabled
	}
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

func (h *Handler) collectOSMComponent(ctx context.Context, caps capabilities.Capabilities) systemstatus.ComponentStatus {
	if disabled, ok := disabledComponentFromCapability(caps, capabilities.KeyOSMImport, "osm", "OSM geodata"); ok {
		return disabled
	}
	snap := systemstatus.OSMSnapshot{CheckedAt: time.Now().UTC()}
	if h.DB != nil && h.DB.Pool != nil {
		_ = h.DB.Pool.QueryRow(ctx, `
			SELECT COUNT(*),
			       COUNT(*) FILTER (WHERE status = 'imported'),
			       COUNT(*) FILTER (WHERE status = 'failed'),
			       COUNT(*) FILTER (WHERE status = 'importing'),
			       COUNT(*) FILTER (WHERE status = 'needs_redownload')
			FROM osm_import_regions`).Scan(
			&snap.RegionCount,
			&snap.ImportedCount,
			&snap.FailedCount,
			&snap.ImportingCount,
			&snap.NeedsRedownloadCount,
		)
	}
	return systemstatus.OSMStatusFromSnapshot(snap)
}

func (h *Handler) collectDEMComponent(ctx context.Context, caps capabilities.Capabilities) systemstatus.ComponentStatus {
	if disabled, ok := disabledComponentFromCapability(caps, capabilities.KeyDEMCorrection, "dem", "DEM"); ok {
		return disabled
	}
	snap := systemstatus.DEMSnapshot{CheckedAt: time.Now().UTC()}
	if h.DB != nil && h.DB.Pool != nil {
		_ = h.DB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM dem.raster_tiles`).Scan(&snap.TileCount)
	}
	return systemstatus.DEMStatusFromSnapshot(snap)
}

func (h *Handler) collectCoverageComponent(ctx context.Context, caps capabilities.Capabilities) systemstatus.ComponentStatus {
	if disabled, ok := disabledComponentFromCapability(caps, capabilities.KeyCoverage, "coverage", "Coverage"); ok {
		return disabled
	}
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

func (h *Handler) collectTilesComponent(ctx context.Context, caps capabilities.Capabilities) systemstatus.ComponentStatus {
	if disabled, ok := disabledComponentFromCapability(caps, capabilities.KeyOfflineMap, "tiles", "Map tiles"); ok {
		return disabled
	}
	snap := systemstatus.PMTilesSnapshot{CheckedAt: time.Now().UTC()}
	if h != nil && h.Config != nil {
		if info, err := os.Stat(h.Config.TilesDir); err == nil && info.IsDir() {
			snap.DirPresent = true
		}
		snap.FileCount = countFilesWithSuffix(h.Config.TilesDir, ".pmtiles", false)
	}
	return systemstatus.PMTilesStatusFromSnapshot(snap)
}

func disabledComponentFromCapability(caps capabilities.Capabilities, featureKey, componentKey, label string) (systemstatus.ComponentStatus, bool) {
	feature, ok := caps.Features[featureKey]
	if !ok || feature.Configured {
		return systemstatus.ComponentStatus{}, false
	}
	reason := feature.Reason
	if reason == "" {
		reason = capabilities.ReasonNotConfigured
	}
	return systemstatus.DisabledComponent(componentKey, label, reason), true
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
