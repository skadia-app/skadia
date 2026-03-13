package handler

import (
	"encoding/json"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/storage"
)

type Handler struct {
	DB            *database.DB
	Storage       *storage.S3Client
	Events        *event.Publisher
	Auth          *AuthConfig              // nil when auth is not configured
	ShadowFS      *shadowfs.ShadowFSConfig // nil when shadow FS is not configured
	Config        *HandlerConfig           // LLM/LiteLLM configuration
	ImportService *importer.Service        // shared post-processing pipeline
	GraphManager  *routing.GraphManager    // nil when routing graph is not configured

	recomputeJob recomputeJobState // in-memory state for async recompute-all
}

type HandlerConfig struct {
	LiteLLMConfigPath  string
	LiteLLMInternalURL string
	LLMEncryptionKey   string
	MLServiceURL       string // e.g. http://ml:8000
	MLCallbackSecret   string // ML_CALLBACK_SECRET env var
	BRouterURL         string // e.g. http://brouter:17777
	KomootEncryptionKey string // 32-byte hex for AES-256-GCM Komoot password encryption
	RoutingGraphPath   string // path to binary graph file
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	status := map[string]string{
		"status":  "ok",
		"db":      "ok",
		"storage": "ok",
		"nats":    "ok",
	}

	if err := h.DB.Ping(r.Context()); err != nil {
		status["db"] = "error"
		status["status"] = "degraded"
	}

	if h.Events == nil {
		status["nats"] = "not configured"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, title, detail, instance string) {
	problem := model.ProblemDetail{
		Type:     "https://skadia.io/errors/" + http.StatusText(status),
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: instance,
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(problem)
}
