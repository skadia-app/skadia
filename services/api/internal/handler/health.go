package handler

import (
	"encoding/json"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/storage"
)

type Handler struct {
	DB      *database.DB
	Storage *storage.S3Client
	Events  *event.Publisher
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
