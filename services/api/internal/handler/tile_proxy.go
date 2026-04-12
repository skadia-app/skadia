package handler

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// hopByHopHeaders lists headers that must not be forwarded to clients per HTTP/1.1 spec.
var hopByHopHeaders = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
}

// martinClient is a reusable HTTP client for proxying tile requests to Martin.
// Timeout is generous because tile rendering for large areas can be slow.
// MaxIdleConnsPerHost is raised to match MapLibre's parallel tile load pattern (8-20+ concurrent).
var martinClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConnsPerHost: 32,
	},
}

// ProxyMartinTile reverse-proxies tile requests to the internal Martin tile server.
// It enforces per-user isolation by always overriding the user_id query parameter
// with the authenticated user's ID for every proxied source.
//
// GET /api/v1/tiles/*
func (h *Handler) ProxyMartinTile(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	// Extract the path after /api/v1/tiles/
	tilePath := chi.URLParam(r, "*")
	if tilePath == "" {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// #7: Path-traversal guard.
	if strings.Contains(tilePath, "..") {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// #4: Always inject the authenticated user_id for every proxied source.
	q := r.URL.Query()
	q.Set("user_id", userID)

	targetURL := "http://martin:3000/" + tilePath
	if encoded := q.Encode(); encoded != "" {
		targetURL += "?" + encoded
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL, nil)
	if err != nil {
		slog.Error("tile proxy: failed to create request", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	resp, err := martinClient.Do(req)
	if err != nil {
		slog.Error("tile proxy: martin request failed", "err", err, "url", targetURL)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// #10: Forward all response headers except hop-by-hop headers.
	for key, values := range resp.Header {
		if hopByHopHeaders[http.CanonicalHeaderKey(key)] {
			continue
		}
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}

	// #12: Cache-Control only on 200 responses.
	if resp.StatusCode == http.StatusOK {
		w.Header().Set("Cache-Control", "public, max-age=3600, s-maxage=86400")
	}

	// #2/#9: URL-rewriting for JSON responses (style.json, TileJSON).
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		// Determine the public base URL from forwarded headers or the request host.
		scheme := r.Header.Get("X-Forwarded-Proto")
		if scheme == "" {
			scheme = "https"
		}
		host := r.Header.Get("X-Forwarded-Host")
		if host == "" {
			host = r.Host
		}
		replacement := scheme + "://" + host + "/api/v1/tiles"

		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			slog.Error("tile proxy: failed to read JSON body for rewriting", "err", readErr)
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
			return
		}

		// Replace both with-trailing-slash and bare forms so order doesn't matter.
		body = bytes.ReplaceAll(body, []byte("http://martin:3000/"), []byte(replacement+"/"))
		body = bytes.ReplaceAll(body, []byte("http://martin:3000"), []byte(replacement))

		// Content-Length is now stale — remove it so the client doesn't truncate.
		w.Header().Del("Content-Length")

		w.WriteHeader(resp.StatusCode)
		if _, copyErr := w.Write(body); copyErr != nil {
			slog.Warn("tile proxy: failed to write rewritten JSON body", "err", copyErr)
		}
		return
	}

	w.WriteHeader(resp.StatusCode)

	// #11: Log io.Copy errors.
	if _, copyErr := io.Copy(w, resp.Body); copyErr != nil {
		slog.Warn("tile proxy: failed to copy tile response body", "err", copyErr)
	}
}
