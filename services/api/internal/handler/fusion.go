package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/go-chi/chi/v5"
)

// SetActivityFusion godoc
// POST /api/v1/activities/{id}/fusion
// Body: {"gps": "uuid-A", "hr": "uuid-B", ...}
func (h *Handler) SetActivityFusion(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")

	var sources database.FusionSources
	if err := json.NewDecoder(r.Body).Decode(&sources); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON", r.URL.Path)
		return
	}

	// Mark all referenced source activities as fusion sources so they are
	// hidden from the normal activity list.
	for _, srcID := range sources {
		if srcID != activityID {
			if err := h.DB.MarkAsFusionSource(r.Context(), srcID); err != nil {
				slog.Warn("mark fusion source failed", "src_id", srcID, "error", err)
			}
		}
	}

	if err := h.DB.SetFusionSources(r.Context(), activityID, sources); err != nil {
		slog.Error("set fusion sources", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to set fusion sources", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetFusionCandidates godoc
// GET /api/v1/activities/{id}/fusion-candidates
// Returns activities that are temporally close and share the same sport type,
// making them potential candidates for stream fusion.
func (h *Handler) GetFusionCandidates(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")

	settings, err := h.DB.GetImportSettings(r.Context(), userID)
	timeWindowMin := 30
	if err == nil && settings != nil && settings.FusionTimeWindowMin > 0 {
		timeWindowMin = settings.FusionTimeWindowMin
	}

	ids, err := h.DB.FindFusionCandidates(r.Context(), userID, activityID, timeWindowMin)
	if err != nil {
		slog.Error("find fusion candidates", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to find fusion candidates", r.URL.Path)
		return
	}

	type candidate struct {
		ID        string  `json:"id"`
		Title     string  `json:"title"`
		StartedAt string  `json:"started_at"`
		DistanceM float64 `json:"distance_m"`
		SportType string  `json:"sport_type"`
	}
	result := make([]candidate, 0, len(ids))
	for _, id := range ids {
		a, err := h.DB.GetActivity(r.Context(), id)
		if err != nil {
			slog.Warn("get fusion candidate activity failed", "id", id, "error", err)
			continue
		}
		title := ""
		if a.Title != nil {
			title = *a.Title
		} else if a.TitleAuto != nil {
			title = *a.TitleAuto
		}
		dist := 0.0
		if a.DistanceM != nil {
			dist = *a.DistanceM
		}
		result = append(result, candidate{
			ID:        a.ID,
			Title:     title,
			StartedAt: a.StartedAt.Format("2006-01-02T15:04:05Z"),
			DistanceM: dist,
			SportType: a.SportType,
		})
	}
	writeJSON(w, http.StatusOK, result)
}
