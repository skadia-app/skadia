package main

import (
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

func mountFrontendStatic(r chi.Router, dir string) bool {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return false
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		slog.Warn("frontend static directory is invalid", "dir", dir, "error", err)
		return false
	}
	realDir, err := filepath.EvalSymlinks(absDir)
	if err != nil {
		slog.Warn("frontend static directory is invalid", "dir", absDir, "error", err)
		return false
	}
	indexPath := filepath.Join(realDir, "index.html")
	if info, err := os.Stat(indexPath); err != nil || info.IsDir() {
		slog.Warn("frontend static directory does not contain index.html", "dir", realDir, "error", err)
		return false
	}

	r.NotFound(frontendStaticHandler(realDir, indexPath))
	return true
}

func frontendStaticHandler(dir, indexPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		if isBackendPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}

		candidate, ok := frontendStaticCandidate(dir, r.URL.Path)
		if ok {
			if resolved, ok := resolveFrontendStaticFile(dir, candidate); ok {
				if strings.HasPrefix(r.URL.Path, "/assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				http.ServeFile(w, r, resolved)
				return
			}
		}
		if path.Ext(r.URL.Path) != "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, indexPath)
	}
}

func frontendStaticCandidate(dir, requestPath string) (string, bool) {
	clean := strings.TrimPrefix(path.Clean("/"+requestPath), "/")
	if clean == "" || clean == "." {
		return "", false
	}
	candidate := filepath.Join(dir, filepath.FromSlash(clean))
	rel, err := filepath.Rel(dir, candidate)
	if err != nil || rel == "." || rel == "" || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", false
	}
	return candidate, true
}

func resolveFrontendStaticFile(dir, candidate string) (string, bool) {
	info, err := os.Stat(candidate)
	if err != nil || info.IsDir() {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(dir, resolved)
	if err != nil || rel == "." || rel == "" || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", false
	}
	return resolved, true
}

func isBackendPath(p string) bool {
	switch {
	case p == "/api" || strings.HasPrefix(p, "/api/"):
		return true
	case p == "/internal" || strings.HasPrefix(p, "/internal/"):
		return true
	case p == "/livez", p == "/readyz", p == "/healthz", p == "/health", p == "/metrics":
		return true
	default:
		return false
	}
}
