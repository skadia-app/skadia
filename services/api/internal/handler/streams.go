package handler

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) GetStreams(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	activityID := chi.URLParam(r, "id")

	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil || activity == nil {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}
	if activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	streams, err := h.DB.GetStreams(r.Context(), activityID)
	if err != nil {
		slog.Error("get streams failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get streams", r.URL.Path)
		return
	}

	// If fusion sources are configured, overlay fields from the referenced activities.
	fusionSources, _ := h.DB.GetFusionSources(r.Context(), activityID)
	if len(fusionSources) > 0 {
		streams = applyFusionSources(r.Context(), h.DB, streams, fusionSources, activityID)
	}

	writeJSON(w, http.StatusOK, streams)
}

// applyFusionSources overlays stream fields from referenced source activities onto
// the primary activity's streams. For each fusion key (e.g. "hr", "gps") it loads
// the source activity's streams and replaces the corresponding fields in the
// primary streams by nearest-time matching (within 30 s).
//
// Supported fusion keys and the StreamResponse fields they control:
//
//	"gps"         → Lat, Lng, AltitudeM
//	"hr"          → HeartRate
//	"power"       → PowerW
//	"cadence"     → Cadence
//	"temperature" → TemperatureC
func applyFusionSources(
	ctx context.Context,
	db *database.DB,
	streams []model.StreamResponse,
	sources database.FusionSources,
	selfID string,
) []model.StreamResponse {
	// Load each unique source activity once.
	sourceStreams := map[string][]model.StreamResponse{}
	for _, srcID := range sources {
		if srcID == selfID {
			continue
		}
		if _, loaded := sourceStreams[srcID]; loaded {
			continue
		}
		ss, err := db.GetStreams(ctx, srcID)
		if err != nil {
			slog.Warn("applyFusionSources: load source streams failed", "src_id", srcID, "error", err)
			continue
		}
		sourceStreams[srcID] = ss
	}

	// For each primary stream point, replace fields as directed by fusion sources.
	const maxDeltaSec = 30.0

	result := make([]model.StreamResponse, len(streams))
	for i, s := range streams {
		result[i] = s

		for fusionKey, srcID := range sources {
			if srcID == selfID {
				continue
			}
			ss, ok := sourceStreams[srcID]
			if !ok || len(ss) == 0 {
				continue
			}
			// Find the nearest time point in the source streams.
			nearest, delta := nearestStreamPoint(ss, s.Time)
			if delta > maxDeltaSec {
				continue
			}
			applyFusionField(&result[i], fusionKey, nearest)
		}
	}
	return result
}

// nearestStreamPoint finds the StreamResponse in ss whose Time is closest to t.
// Returns the nearest point and the absolute time difference in seconds.
func nearestStreamPoint(ss []model.StreamResponse, t time.Time) (model.StreamResponse, float64) {
	best := ss[0]
	bestDelta := math.Abs(ss[0].Time.Sub(t).Seconds())
	for _, s := range ss[1:] {
		d := math.Abs(s.Time.Sub(t).Seconds())
		if d < bestDelta {
			best = s
			bestDelta = d
		}
	}
	return best, bestDelta
}

// applyFusionField copies the relevant fields from src into dst based on fusionKey.
func applyFusionField(dst *model.StreamResponse, fusionKey string, src model.StreamResponse) {
	switch fusionKey {
	case "gps":
		dst.Lat = src.Lat
		dst.Lng = src.Lng
		dst.AltitudeM = src.AltitudeM
	case "hr":
		dst.HeartRate = src.HeartRate
	case "power":
		dst.PowerW = src.PowerW
	case "cadence":
		dst.Cadence = src.Cadence
	case "temperature":
		dst.TemperatureC = src.TemperatureC
	}
}
