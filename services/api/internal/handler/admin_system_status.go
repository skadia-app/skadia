package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/buildinfo"
	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/systemstatus"
)

var processStartedAt = time.Now()

func (h *Handler) GetAdminSystemStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status := h.collectAdminSystemStatus(ctx)
	httputil.WriteJSON(w, http.StatusOK, systemstatus.SanitizeSystemStatusForUI(status))
}

func (h *Handler) collectAdminSystemStatus(ctx context.Context) systemstatus.SystemStatus {
	build := buildinfo.Snapshot()
	buildMap := map[string]systemstatus.BuildInfo{
		"api": {
			Service:       "api",
			GitCommit:     build.GitCommit,
			CommitTime:    build.CommitTime,
			BuildTime:     build.BuildTime,
			ImageDigest:   build.ImageDigest,
			ConfigVersion: build.ConfigVersion,
			StartedAt:     processStartedAt.Format(time.RFC3339),
			UptimeSeconds: int64(time.Since(processStartedAt).Seconds()),
		},
	}
	if h.Config != nil {
		if frontend, ok := buildinfo.ReadFrontendManifest(ctx, h.Config.FrontendStaticDir); ok {
			buildMap["frontend"] = systemstatus.BuildInfo{
				Service:       "frontend",
				GitCommit:     frontend.GitCommit,
				CommitTime:    frontend.CommitTime,
				BuildTime:     frontend.BuildTime,
				ImageDigest:   frontend.ImageDigest,
				ConfigVersion: frontend.ConfigVersion,
			}
		}
	}
	featureMatrix := map[string]systemstatus.ComponentStatus{}
	if h.DB != nil && h.DB.Pool != nil && h.Config != nil {
		caps := capabilities.Detect(ctx, h.DB.Pool, h.detectInputForGate())
		featureMatrix = systemstatus.FeatureMatrixFromCapabilities(caps)
	}
	collector := systemstatus.Collector{
		Build: buildMap,
		ComponentCollectors: []systemstatus.ComponentCollector{
			h.collectAPIComponent,
			h.collectDBComponent,
		},
		FeatureMatrix: featureMatrix,
	}
	return collector.Collect(ctx)
}

func (h *Handler) collectAPIComponent(ctx context.Context) systemstatus.ComponentStatus {
	return systemstatus.ComponentStatus{
		Key:       "api",
		Label:     "API",
		State:     systemstatus.StatusOK,
		Required:  true,
		CheckedAt: time.Now().UTC(),
	}
}

func (h *Handler) collectDBComponent(ctx context.Context) systemstatus.ComponentStatus {
	st := systemstatus.ComponentStatus{
		Key:       "db",
		Label:     "Database",
		State:     systemstatus.StatusOK,
		Required:  true,
		CheckedAt: time.Now().UTC(),
	}
	if h.DB == nil || h.DB.Pool == nil {
		st.State = systemstatus.StatusNotConfigured
		st.ReasonCode = "db_not_configured"
		return st
	}
	if err := h.DB.Ping(ctx); err != nil {
		st.State = systemstatus.StatusUnavailable
		st.ReasonCode = "db_unreachable"
		st.Summary = "Database connection failed."
	}
	return st
}
