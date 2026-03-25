package handler

import (
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// ServeMedia proxies authenticated media requests to Garage S3 storage.
// GET /api/v1/media/{bucket}/{key...}
// Used for tag cover images, custom map marker images, and other user-uploaded media.
func (h *Handler) ServeMedia(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	bucket := chi.URLParam(r, "bucket")
	// chi wildcard (*) captures the rest of the path including slashes
	key := chi.URLParam(r, "*")
	key = strings.TrimPrefix(key, "/")

	if bucket == "" || key == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "missing bucket or key", r.URL.Path)
		return
	}

	// Restrict to skadia-media bucket and validate key against an explicit allowlist of
	// known path prefixes that include the userID.
	// S3 key patterns: "{userID}/...", "tag-covers/{userID}/...", "gear/{userID}/...",
	// "marker-images/{userID}/...", "activity-photos/{userID}/..."
	if bucket != "skadia-media" {
		httputil.WriteError(w, http.StatusForbidden, "Forbidden", "access denied", r.URL.Path)
		return
	}
	allowedPrefixes := []string{
		userID + "/",
		"tag-covers/" + userID + "/",
		"gear/" + userID + "/",
		"marker-images/" + userID + "/",
		"activity-photos/" + userID + "/",
	}
	hasAccess := false
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(key, prefix) {
			hasAccess = true
			break
		}
	}
	if !hasAccess {
		httputil.WriteError(w, http.StatusForbidden, "Forbidden", "access denied", r.URL.Path)
		return
	}

	data, ct, err := h.Storage.DownloadFromBucket(r.Context(), bucket, key)
	if err != nil {
		slog.Error("media proxy: s3 download failed", "bucket", bucket, "key", key, "error", err)
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "media not found", r.URL.Path)
		return
	}

	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(data)
}

// ServeFile serves a file from the FileStore.
// GET /api/v1/files/{fileID}
func (h *Handler) ServeFile(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	fileID := chi.URLParam(r, "fileID")
	if fileID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "fileID required", r.URL.Path)
		return
	}

	resolved, err := h.FileStore.Resolve(r.Context(), userID, fileID)
	if err != nil {
		slog.Error("file serve failed", "file_id", fileID, "error", err)
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "file not found", r.URL.Path)
		return
	}

	// Security headers
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// Content-Disposition: inline for images/video, attachment for everything else
	disposition := "attachment"
	if strings.HasPrefix(resolved.ContentType, "image/") || strings.HasPrefix(resolved.ContentType, "video/") {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition",
		mime.FormatMediaType(disposition, map[string]string{"filename": resolved.FileName}))

	switch resolved.Type {
	case "local":
		http.ServeFile(w, r, resolved.Path)
	case "s3":
		// Sensitive types: server-side proxy (don't expose presigned URL)
		if resolved.ContentType == "" ||
			strings.HasPrefix(resolved.ContentType, "application/") ||
			resolved.ContentType == "text/plain" {
			resp, err := http.Get(resolved.URL) //nolint:noctx
			if err != nil {
				httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to fetch file", r.URL.Path)
				return
			}
			defer resp.Body.Close()
			w.Header().Set("Content-Type", resolved.ContentType)
			io.Copy(w, resp.Body) //nolint:errcheck
		} else {
			// Media: redirect to presigned URL
			http.Redirect(w, r, resolved.URL, http.StatusTemporaryRedirect)
		}
	}
}
