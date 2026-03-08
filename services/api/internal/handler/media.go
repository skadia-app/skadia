package handler

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/go-chi/chi/v5"
)

// ServeMedia proxies authenticated media requests to Garage S3 storage.
// GET /api/v1/media/{bucket}/{key...}
// Used for tag cover images, custom map marker images, and other user-uploaded media.
func (h *Handler) ServeMedia(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	bucket := chi.URLParam(r, "bucket")
	// chi wildcard (*) captures the rest of the path including slashes
	key := chi.URLParam(r, "*")
	key = strings.TrimPrefix(key, "/")

	if bucket == "" || key == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "missing bucket or key", r.URL.Path)
		return
	}

	// Restrict to skadia-media bucket and validate key contains userID as a path segment.
	// S3 keys follow patterns like: "tag-covers/{userID}/...", "gear/{userID}/...",
	// "marker-images/{userID}/...", "activity-photos/{userID}/...", "{userID}/..."
	if bucket != "skadia-media" || (!strings.HasPrefix(key, userID+"/") && !strings.Contains(key, "/"+userID+"/")) {
		writeError(w, http.StatusForbidden, "Forbidden", "access denied", r.URL.Path)
		return
	}

	data, ct, err := h.Storage.DownloadFromBucket(r.Context(), bucket, key)
	if err != nil {
		slog.Error("media proxy: s3 download failed", "bucket", bucket, "key", key, "error", err)
		writeError(w, http.StatusNotFound, "Not Found", "media not found", r.URL.Path)
		return
	}

	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(data)
}
