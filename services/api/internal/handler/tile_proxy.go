package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
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

	// #12: Cache-Control only on 200 responses. Use `private` because every
	// proxied tile request is scoped to the authenticated user_id.
	if resp.StatusCode == http.StatusOK {
		w.Header().Set("Cache-Control", "private, max-age=3600")
	}

	// #2/#9: URL-rewriting for JSON responses (style.json, TileJSON).
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		// Determine and strictly validate the public base URL from forwarded
		// headers or the request host so untrusted header values cannot inject
		// malformed JSON, HTML/XSS, or foreign origins.
		scheme := sanitizeTileProxyScheme(r.Header.Get("X-Forwarded-Proto"))
		rawHost := r.Header.Get("X-Forwarded-Host")
		if rawHost == "" {
			rawHost = r.Host
		}
		host, hostOK := sanitizeTileProxyHost(rawHost)
		if !hostOK {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		replacement := scheme + "://" + host + "/api/v1/tiles"

		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			slog.Error("tile proxy: failed to read JSON body for rewriting", "err", readErr)
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
			return
		}

		var parsed any
		if unmarshalErr := json.Unmarshal(body, &parsed); unmarshalErr != nil {
			slog.Error("tile proxy: upstream JSON body invalid", "err", unmarshalErr)
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
			return
		}
		rewritten := rewriteMartinJSONURLs(parsed, replacement)
		encoded, marshalErr := json.Marshal(rewritten)
		if marshalErr != nil {
			slog.Error("tile proxy: failed to marshal rewritten JSON body", "err", marshalErr)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Content-Length is now stale — remove it so the client doesn't truncate.
		w.Header().Del("Content-Length")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Add("Vary", "Host")
		w.Header().Add("Vary", "X-Forwarded-Host")
		w.Header().Add("Vary", "X-Forwarded-Proto")

		w.WriteHeader(resp.StatusCode)
		if _, copyErr := w.Write(encoded); copyErr != nil {
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

func sanitizeTileProxyScheme(raw string) string {
	// X-Forwarded-Proto may contain a comma-separated proxy chain; take the first hop.
	if first, _, ok := strings.Cut(raw, ","); ok {
		raw = first
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "http":
		return "http"
	default:
		return "https"
	}
}

func sanitizeTileProxyHost(raw string) (string, bool) {
	if first, _, ok := strings.Cut(raw, ","); ok {
		raw = first
	}
	host := strings.TrimSpace(raw)
	if host == "" || len(host) > 255 {
		return "", false
	}
	for i := range len(host) {
		c := host[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '.' || c == '-' || c == ':' || c == '[' || c == ']') {
			return "", false
		}
	}
	u, err := url.Parse("https://" + host)
	if err != nil || u.Host != host || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	return host, true
}

func rewriteMartinJSONURLs(v any, replacement string) any {
	switch val := v.(type) {
	case string:
		s := strings.ReplaceAll(val, "http://martin:3000/", replacement+"/")
		return strings.ReplaceAll(s, "http://martin:3000", replacement)
	case []any:
		for i := range val {
			val[i] = rewriteMartinJSONURLs(val[i], replacement)
		}
		return val
	case map[string]any:
		for k, elem := range val {
			val[k] = rewriteMartinJSONURLs(elem, replacement)
		}
		return val
	default:
		return v
	}
}
