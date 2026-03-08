package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/go-chi/chi/v5"
)

// FusionMergeRequest beschreibt den Body für POST /activities/{id}/fusion-merge.
// Sources enthält bis zu 2 zusätzliche Quell-Aktivitäts-IDs.
// FieldMap mappt Stream-Felder auf Quell-IDs ("hr" → "uuid-B", "gps" → "fuse").
type FusionMergeRequest struct {
	Sources         []string          `json:"sources"`
	FieldMap        map[string]string `json:"field_map"`
	GPSFusionConfig *GPSFusionConfig  `json:"gps_fusion_config,omitempty"`
	TitleSource     *string           `json:"title_source,omitempty"`
	TagsMerge       bool              `json:"tags_merge"`
	GearSource      *string           `json:"gear_source,omitempty"`
}

// validStreamFields mappt logische Feld-Namen auf activity_streams-Spaltennamen.
var validStreamFields = map[string]string{
	"hr":          "heart_rate",
	"power":       "power_w",
	"cadence":     "cadence",
	"temperature": "temperature_c",
	"gps":         "gps",
	"altitude":    "altitude_m",
	"speed":       "speed_ms",
}

// FusionMerge führt eine 3-Quellen-Fusion durch:
// POST /api/v1/activities/{id}/fusion-merge
func (h *Handler) FusionMerge(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	targetID := chi.URLParam(r, "id")

	var req FusionMergeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON", r.URL.Path)
		return
	}

	if len(req.Sources) > 2 {
		writeError(w, http.StatusBadRequest, "Bad Request", "max 2 additional sources allowed", r.URL.Path)
		return
	}

	allIDs := append([]string{targetID}, req.Sources...)

	// Alle Aktivitäten laden + Ownership prüfen.
	activities := make(map[string]*model.Activity, len(allIDs))
	for _, id := range allIDs {
		act, err := h.DB.GetActivity(r.Context(), id)
		if err != nil || act == nil {
			writeError(w, http.StatusNotFound, "Not Found", fmt.Sprintf("activity %s not found", id), r.URL.Path)
			return
		}
		if act.UserID != userID {
			writeError(w, http.StatusForbidden, "Forbidden", "not your activity", r.URL.Path)
			return
		}
		activities[id] = act
	}

	// FieldMap validieren.
	for field, sourceID := range req.FieldMap {
		if _, ok := validStreamFields[field]; !ok {
			writeError(w, http.StatusBadRequest, "Bad Request", fmt.Sprintf("unknown field: %s", field), r.URL.Path)
			return
		}
		if sourceID != "fuse" && !slices.Contains(allIDs, sourceID) {
			writeError(w, http.StatusBadRequest, "Bad Request", fmt.Sprintf("source %s not in sources list", sourceID), r.URL.Path)
			return
		}
		if sourceID == "fuse" && field != "gps" {
			writeError(w, http.StatusBadRequest, "Bad Request", "fuse only allowed for gps field", r.URL.Path)
			return
		}
	}

	ctx := r.Context()
	gpsModified := false

	// Stream-Felder zusammenführen.
	for field, sourceID := range req.FieldMap {
		if sourceID == targetID {
			// Quelle = Ziel → nichts zu tun.
			continue
		}

		if field == "gps" {
			if sourceID == "fuse" {
				// GPS-Track-Fusion (wird in Task 2b vollständig implementiert).
				if err := h.fuseGPSTracks(ctx, targetID, req.Sources, req.GPSFusionConfig); err != nil {
					writeError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
					return
				}
			} else {
				if err := h.mergeGPSFromSource(ctx, targetID, sourceID); err != nil {
					slog.Error("fusion-merge: GPS merge failed", "target", targetID, "source", sourceID, "error", err)
					writeError(w, http.StatusInternalServerError, "Internal Server Error", "GPS merge failed", r.URL.Path)
					return
				}
			}
			gpsModified = true
			continue
		}

		col := validStreamFields[field]
		if err := h.fusionMergeStreamColumn(ctx, targetID, sourceID, col); err != nil {
			slog.Error("fusion-merge: stream column merge failed", "field", field, "col", col, "error", err)
			writeError(w, http.StatusInternalServerError, "Internal Server Error", fmt.Sprintf("merge field %s failed", field), r.URL.Path)
			return
		}
	}

	// Titel aus Quell-Aktivität übernehmen.
	if req.TitleSource != nil && *req.TitleSource != targetID {
		if src, ok := activities[*req.TitleSource]; ok && src.Title != nil {
			if _, err := h.DB.Pool.Exec(ctx,
				`UPDATE activities SET title = $2, has_edits = true, updated_at = NOW() WHERE id = $1`,
				targetID, *src.Title,
			); err != nil {
				slog.Warn("fusion-merge: update title failed", "error", err)
			}
		}
	}

	// Gear aus Quell-Aktivität übernehmen.
	if req.GearSource != nil && *req.GearSource != targetID {
		if src, ok := activities[*req.GearSource]; ok && src.GearID != nil {
			if _, err := h.DB.Pool.Exec(ctx,
				`UPDATE activities SET gear_id = $2, has_edits = true, updated_at = NOW() WHERE id = $1`,
				targetID, *src.GearID,
			); err != nil {
				slog.Warn("fusion-merge: update gear failed", "error", err)
			}
		}
	}

	// Tags aus allen Quell-Aktivitäten zusammenführen.
	if req.TagsMerge {
		for _, srcID := range req.Sources {
			if err := h.mergeActivityTags(ctx, targetID, srcID, userID); err != nil {
				slog.Warn("fusion-merge: merge tags failed", "source", srcID, "error", err)
			}
		}
	}

	// Track + Heatmap neu bauen wenn GPS geändert wurde.
	if gpsModified {
		if err := h.DB.RebuildActivityTrack(ctx, targetID); err != nil {
			slog.Warn("fusion-merge: track rebuild failed", "id", targetID, "err", err)
		}
		if err := h.DB.RebuildHeatmapData(ctx, targetID); err != nil {
			slog.Warn("fusion-merge: heatmap rebuild failed", "id", targetID, "err", err)
		}
	}

	// Quell-Aktivitäten als is_fusion_source markieren.
	for _, srcID := range req.Sources {
		if _, err := h.DB.Pool.Exec(ctx,
			`UPDATE activities SET is_fusion_source = true, updated_at = NOW() WHERE id = $1`,
			srcID,
		); err != nil {
			slog.Warn("fusion-merge: mark fusion source failed", "source", srcID, "error", err)
		}
	}

	// fusion_sources persistieren.
	fusionSources := make(map[string]string, len(req.FieldMap))
	for field, srcID := range req.FieldMap {
		fusionSources[field] = srcID
	}
	if err := h.DB.SetFusionSources(ctx, targetID, fusionSources); err != nil {
		slog.Warn("fusion-merge: set fusion sources failed", "error", err)
	}

	// Audit-Log in activity_edits.
	auditData, _ := json.Marshal(map[string]any{
		"type":      "fusion_merge",
		"sources":   req.Sources,
		"field_map": req.FieldMap,
	})
	if _, err := h.DB.Pool.Exec(ctx,
		`INSERT INTO activity_edits (activity_id, user_id, edit_type, edit_data)
		 VALUES ($1, $2, 'fusion_merge', $3)`,
		targetID, userID, auditData,
	); err != nil {
		slog.Warn("fusion-merge: failed to insert audit record", "id", targetID, "error", err)
	}

	// Aktualisierte Aktivität zurückgeben.
	result, err := h.DB.GetActivity(ctx, targetID)
	if err != nil || result == nil {
		writeError(w, http.StatusInternalServerError, "Internal Server Error", "failed to reload activity", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// fusionMergeStreamColumn kopiert eine einzelne Stream-Spalte von sourceID → targetID
// (nur für Zeitpunkte die in beiden Aktivitäten vorhanden sind).
func (h *Handler) fusionMergeStreamColumn(ctx context.Context, targetID, sourceID, column string) error {
	query := fmt.Sprintf(`
		UPDATE activity_streams t
		SET %s = s.%s
		FROM activity_streams s
		WHERE t.activity_id = $1
		  AND s.activity_id = $2
		  AND t.time = s.time
		  AND s.%s IS NOT NULL`,
		column, column, column)
	_, err := h.DB.Pool.Exec(ctx, query, targetID, sourceID)
	return err
}

// mergeGPSFromSource kopiert lat/lng von sourceID → targetID (timestamp-matched).
func (h *Handler) mergeGPSFromSource(ctx context.Context, targetID, sourceID string) error {
	_, err := h.DB.Pool.Exec(ctx, `
		UPDATE activity_streams t
		SET lat = s.lat, lng = s.lng
		FROM activity_streams s
		WHERE t.activity_id = $1
		  AND s.activity_id = $2
		  AND t.time = s.time
		  AND s.lat IS NOT NULL
		  AND s.lng IS NOT NULL`,
		targetID, sourceID)
	return err
}

