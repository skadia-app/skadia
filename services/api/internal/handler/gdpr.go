package handler

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// ExportUserData handles GET /api/v1/me/export — builds a GDPR data archive ZIP.
func (h *Handler) ExportUserData(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	var buf zippedBuffer
	zw := zip.NewWriter(&buf)

	// Recording Groups
	groups, _ := h.DB.GetRecordingGroupsForUser(ctx, userID, 0, 0)
	if len(groups) > 0 {
		writeJSON(zw, "recording_groups.json", groups)
	}

	// Developer Apps
	devApps, err := h.DB.GetDeveloperAppsForGDPR(ctx, userID)
	if err != nil {
		slog.Error("gdpr export: get developer apps failed", "user", userID, "error", err)
	} else if len(devApps) > 0 {
		writeJSON(zw, "developer_apps.json", devApps)
	}

	// Routing Profiles
	routingProfiles, _ := h.DB.ExportRoutingProfiles(ctx, userID)
	if len(routingProfiles) > 0 {
		writeJSON(zw, "routing_profiles.json", routingProfiles)
	}

	if err := zw.Close(); err != nil {
		slog.Error("gdpr export: close zip writer", "user_id", userID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to build export archive", r.URL.Path)
		return
	}

	dateStr := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("skadia-gdpr-export-%s.zip", dateStr)

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", buf.Len()))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(buf.Bytes()); err != nil {
		slog.Warn("gdpr export: write response failed", "user_id", userID, "error", err)
	}
}

// writeJSON marshals v as JSON and adds it as a file inside the ZIP archive.
// Errors are logged and silently skipped so a partial failure doesn't abort the whole export.
func writeJSON(zw *zip.Writer, name string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		slog.Warn("gdpr export: marshal JSON failed", "file", name, "error", err)
		return
	}
	f, err := zw.Create(name)
	if err != nil {
		slog.Warn("gdpr export: zip create entry failed", "file", name, "error", err)
		return
	}
	if _, err := f.Write(data); err != nil {
		slog.Warn("gdpr export: zip write failed", "file", name, "error", err)
	}
}

// zippedBuffer is a simple byte-buffer that satisfies io.Writer for zip.NewWriter.
type zippedBuffer struct {
	data []byte
}

func (b *zippedBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *zippedBuffer) Bytes() []byte { return b.data }
func (b *zippedBuffer) Len() int      { return len(b.data) }
