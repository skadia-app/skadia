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
	root, err := os.OpenRoot(realDir)
	if err != nil {
		slog.Warn("frontend static directory could not be opened", "dir", realDir, "error", err)
		return false
	}
	if info, err := root.Stat("index.html"); err != nil || info.IsDir() {
		_ = root.Close()
		slog.Warn("frontend static directory does not contain index.html", "dir", realDir, "error", err)
		return false
	}

	r.NotFound(frontendStaticHandler(root))
	return true
}

func frontendStaticHandler(root *os.Root) http.HandlerFunc {
	serveFromRoot := func(w http.ResponseWriter, r *http.Request, rel string) bool {
		f, err := root.Open(rel)
		if err != nil {
			return false
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || info.IsDir() {
			return false
		}
		http.ServeContent(w, r, info.Name(), info.ModTime(), f)
		return true
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		if isBackendPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}

		if rel, ok := frontendStaticRelPath(r.URL.Path); ok {
			if strings.HasPrefix(r.URL.Path, "/assets/") {
				if info, err := root.Stat(rel); err == nil && !info.IsDir() {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
			}
			if serveFromRoot(w, r, rel) {
				return
			}
		}
		if path.Ext(r.URL.Path) != "" {
			http.NotFound(w, r)
			return
		}
		if !serveFromRoot(w, r, "index.html") {
			http.NotFound(w, r)
		}
	}
}

func frontendStaticRelPath(requestPath string) (string, bool) {
	clean := strings.TrimPrefix(path.Clean("/"+requestPath), "/")
	if clean == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
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
