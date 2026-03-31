package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/fusion"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
	// FitLevel: wenn true, werden die Original-FIT-Dateien aus S3 geladen,
	// auf Protokoll-Ebene zusammengeführt und als neue FIT-Datei gespeichert.
	// Zusätzlich zum normalen DB-Level-Merge.
	FitLevel bool `json:"fit_level"`
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

// FusionMerge führt eine 3-Quellen-Fusion durch und erstellt eine neue Aktivität:
// POST /api/v1/activities/{id}/fusion-merge
func (h *Handler) FusionMerge(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	targetID := chi.URLParam(r, "id")

	req, ok := httputil.DecodeJSON[FusionMergeRequest](w, r)
	if !ok {
		return
	}

	if len(req.Sources) > 2 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "max 2 additional sources allowed", r.URL.Path)
		return
	}

	allIDs := append([]string{targetID}, req.Sources...)

	// Alle Aktivitäten laden + Ownership prüfen.
	activities := make(map[string]*model.Activity, len(allIDs))
	for _, id := range allIDs {
		act, err := h.DB.GetActivity(r.Context(), id)
		if err != nil || act == nil {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", fmt.Sprintf("activity %s not found", id), r.URL.Path)
			return
		}
		if act.UserID != userID {
			httputil.WriteError(w, http.StatusForbidden, "Forbidden", "not your activity", r.URL.Path)
			return
		}
		activities[id] = act
	}

	// FieldMap validieren.
	for field, sourceID := range req.FieldMap {
		if _, ok := validStreamFields[field]; !ok {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", fmt.Sprintf("unknown field: %s", field), r.URL.Path)
			return
		}
		if sourceID != "fuse" && !slices.Contains(allIDs, sourceID) {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", fmt.Sprintf("source %s not in sources list", sourceID), r.URL.Path)
			return
		}
		if sourceID == "fuse" && field != "gps" {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "fuse only allowed for gps field", r.URL.Path)
			return
		}
	}

	ctx := r.Context()

	// Begin transaction — all DB mutations are wrapped here.
	tx, err := h.DB.Pool.Begin(ctx)
	if err != nil {
		slog.Error("fusion-merge: begin transaction failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to begin transaction", r.URL.Path)
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// 1. Neue Aktivität erstellen (Kopie der Target-Metadaten).
	merged, err := createMergedActivity(ctx, tx, activities[targetID])
	if err != nil {
		slog.Error("fusion-merge: create merged activity failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to create merged activity", r.URL.Path)
		return
	}

	// 2. Streams der Target-Aktivität in neue Aktivität kopieren (alle Spalten).
	if _, err := tx.Exec(ctx,
		`INSERT INTO activity_streams (
		     activity_id, time,
		     lat, lng, altitude_m, speed_ms, distance_m,
		     heart_rate, power_w, cadence, temperature_c,
		     respiration_rate, stress_level,
		     ground_contact_time_ms, vertical_oscillation_mm, stance_time_percent, left_right_balance,
		     grade_pct, vertical_speed_ms, gps_accuracy_m, calories, resistance,
		     total_cycles, accumulated_power_w, absolute_pressure_pa, ascent_rate_ms,
		     time_from_course_s, activity_type, device_index, step_length_mm,
		     vertical_ratio_pct, stance_time_balance_pct, cycle_length_m, fractional_cadence,
		     left_torque_effectiveness_pct, right_torque_effectiveness_pct,
		     left_pedal_smoothness_pct, right_pedal_smoothness_pct, combined_pedal_smoothness_pct,
		     left_pco_mm, right_pco_mm,
		     left_power_phase_start_deg, left_power_phase_end_deg,
		     left_power_phase_peak_start_deg, left_power_phase_peak_end_deg,
		     right_power_phase_start_deg, right_power_phase_end_deg,
		     right_power_phase_peak_start_deg, right_power_phase_peak_end_deg,
		     total_hemoglobin_conc, total_hemoglobin_conc_min, total_hemoglobin_conc_max,
		     saturated_hemoglobin_pct, saturated_hemoglobin_pct_min, saturated_hemoglobin_pct_max,
		     motor_power_w, ebike_travel_range_km, ebike_battery_level_pct,
		     ebike_assist_mode, ebike_assist_level_pct,
		     core_temperature_c, enhanced_respiration_rate, current_stress,
		     battery_soc_pct, grit, flow, stroke_type, zone, ball_speed_ms,
		     depth_m, next_stop_depth_m, next_stop_time_s, time_to_surface_s,
		     ndl_time_s, cns_load_pct, n2_load_pct, air_time_remaining_s,
		     pressure_sac, volume_sac, rmv, po2,
		     developer_fields, heading, potential_stamina, stamina, gap_speed_ms
		 )
		 SELECT
		     $2, time,
		     lat, lng, altitude_m, speed_ms, distance_m,
		     heart_rate, power_w, cadence, temperature_c,
		     respiration_rate, stress_level,
		     ground_contact_time_ms, vertical_oscillation_mm, stance_time_percent, left_right_balance,
		     grade_pct, vertical_speed_ms, gps_accuracy_m, calories, resistance,
		     total_cycles, accumulated_power_w, absolute_pressure_pa, ascent_rate_ms,
		     time_from_course_s, activity_type, device_index, step_length_mm,
		     vertical_ratio_pct, stance_time_balance_pct, cycle_length_m, fractional_cadence,
		     left_torque_effectiveness_pct, right_torque_effectiveness_pct,
		     left_pedal_smoothness_pct, right_pedal_smoothness_pct, combined_pedal_smoothness_pct,
		     left_pco_mm, right_pco_mm,
		     left_power_phase_start_deg, left_power_phase_end_deg,
		     left_power_phase_peak_start_deg, left_power_phase_peak_end_deg,
		     right_power_phase_start_deg, right_power_phase_end_deg,
		     right_power_phase_peak_start_deg, right_power_phase_peak_end_deg,
		     total_hemoglobin_conc, total_hemoglobin_conc_min, total_hemoglobin_conc_max,
		     saturated_hemoglobin_pct, saturated_hemoglobin_pct_min, saturated_hemoglobin_pct_max,
		     motor_power_w, ebike_travel_range_km, ebike_battery_level_pct,
		     ebike_assist_mode, ebike_assist_level_pct,
		     core_temperature_c, enhanced_respiration_rate, current_stress,
		     battery_soc_pct, grit, flow, stroke_type, zone, ball_speed_ms,
		     depth_m, next_stop_depth_m, next_stop_time_s, time_to_surface_s,
		     ndl_time_s, cns_load_pct, n2_load_pct, air_time_remaining_s,
		     pressure_sac, volume_sac, rmv, po2,
		     developer_fields, heading, potential_stamina, stamina, gap_speed_ms
		 FROM activity_streams WHERE activity_id = $1
		 ORDER BY time ASC`,
		targetID, merged.ID,
	); err != nil {
		slog.Error("fusion-merge: copy streams failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to copy streams", r.URL.Path)
		return
	}

	// 3. Stream-Felder zusammenführen (auf neue Aktivität angewendet).
	for field, sourceID := range req.FieldMap {
		if sourceID == merged.ID || sourceID == targetID {
			// Ziel = neue Aktivität oder original Target → Streams bereits kopiert.
			continue
		}

		if field == "gps" {
			if sourceID == "fuse" {
				if err := h.fuseGPSTracks(ctx, tx, merged.ID, req.Sources, req.GPSFusionConfig); err != nil {
					httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
					return
				}
			} else {
				if err := fusionMergeGPSFromSource(ctx, tx, merged.ID, sourceID); err != nil {
					slog.Error("fusion-merge: GPS merge failed", "error", err)
					httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "GPS merge failed", r.URL.Path)
					return
				}
			}
			continue
		}

		col := validStreamFields[field]
		if err := fusionMergeStreamColumn(ctx, tx, merged.ID, sourceID, col); err != nil {
			slog.Error("fusion-merge: stream column merge failed", "field", field, "col", col, "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", fmt.Sprintf("merge field %s failed", field), r.URL.Path)
			return
		}
	}

	// 4. Titel aus Quell-Aktivität übernehmen.
	if req.TitleSource != nil && *req.TitleSource != targetID {
		if src, ok := activities[*req.TitleSource]; ok && src.Title != nil {
			if _, err := tx.Exec(ctx,
				`UPDATE activities SET title = $2, has_edits = true WHERE id = $1`,
				merged.ID, *src.Title,
			); err != nil {
				slog.Warn("fusion-merge: update title failed", "error", err)
			}
		}
	}

	// 5. Gear übernehmen.
	if req.GearSource != nil {
		if src, ok := activities[*req.GearSource]; ok && src.GearID != nil {
			tx.Exec(ctx, `UPDATE activities SET gear_id = $2, has_edits = true WHERE id = $1`, merged.ID, src.GearID) //nolint:errcheck
		}
	}

	// 6. Tags aus allen Quell-Aktivitäten zusammenführen.
	if req.TagsMerge {
		for _, srcID := range allIDs {
			if err := fusionMergeActivityTags(ctx, tx, merged.ID, srcID); err != nil {
				slog.Warn("fusion-merge: merge tags failed", "source", srcID, "error", err)
			}
		}
	}

	// 7. fusion_sources auf neue Aktivität setzen (für Info-Anzeige im Frontend).
	sourceMap := make(database.FusionSources, len(req.FieldMap))
	maps.Copy(sourceMap, req.FieldMap)
	sourcesJSON, err := json.Marshal(sourceMap)
	if err != nil {
		slog.Warn("fusion-merge: marshal fusion sources failed", "error", err)
	} else {
		if _, err := tx.Exec(ctx,
			`UPDATE activities SET fusion_sources = $2 WHERE id = $1 AND user_id = $3`,
			merged.ID, sourcesJSON, userID,
		); err != nil {
			slog.Warn("fusion-merge: set fusion sources failed", "error", err)
		}
	}

	// 8. Alle Quellen (inkl. Target) via parent_id verstecken.
	for _, id := range allIDs {
		if _, err := tx.Exec(ctx,
			`UPDATE activities SET parent_id = $2 WHERE id = $1 AND user_id = $3`,
			id, merged.ID, userID,
		); err != nil {
			slog.Warn("fusion-merge: hide source failed", "id", id, "error", err)
		}
	}

	// 9. activity_tracks für neue Aktivität berechnen — uses db.Pool internally,
	// so it runs within its own connection (outside tx). Execute before commit
	// so track data is ready, but note it won't be rolled back on tx failure.
	// These are best-effort rebuilds; their failure is non-fatal.
	if merged.HasGPS {
		if err := h.DB.RebuildActivityTrack(ctx, merged.ID); err != nil {
			slog.Warn("fusion-merge: rebuild track failed", "error", err)
		}
		if !merged.ExcludeFromHeatmap {
			if err := h.DB.RebuildHeatmapData(ctx, merged.ID); err != nil {
				slog.Warn("fusion-merge: rebuild heatmap failed", "error", err)
			}
		}
		h.DB.InvalidateTerraIncognitaCache(ctx, userID)
	}

	// 10. Audit-Log (inside transaction).
	auditData, _ := json.Marshal(map[string]any{
		"type":      "fusion_merge",
		"sources":   allIDs,
		"field_map": req.FieldMap,
	})
	if _, err := tx.Exec(ctx,
		`INSERT INTO activity_edits (activity_id, user_id, edit_type, edit_data)
		 VALUES ($1, $2, 'fusion_merge', $3)`,
		merged.ID, userID, auditData,
	); err != nil {
		slog.Warn("fusion-merge: failed to insert audit record", "id", merged.ID, "error", err)
	}

	// Commit transaction — all DB mutations are now durable.
	if err := tx.Commit(ctx); err != nil {
		slog.Error("fusion-merge: commit transaction failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to commit transaction", r.URL.Path)
		return
	}

	// 11. FIT-Level-Merge: Original-FIT-Dateien zusammenführen (non-DB, after commit).
	if req.FitLevel {
		if err := h.fitLevelMerge(ctx, userID, targetID, merged.ID, req.Sources, req.FieldMap, activities); err != nil {
			slog.Error("fusion-merge: FIT-level merge failed", "target", merged.ID, "error", err)
			// Non-fatal: DB-level merge was already successful.
		} else {
			slog.Info("fusion-merge: FIT-level merge completed", "target", merged.ID)
		}
	}

	// 12. Write .meta companion for merged activity (ShadowFS, after commit).
	mergedAct, _ := h.DB.GetActivity(ctx, merged.ID)
	if mergedAct != nil && mergedAct.FileID != nil && *mergedAct.FileID != "" {
		sidecar.WriteMetaCompanion(ctx, h.ShadowFS, h.DB, userID, merged.ID, *mergedAct.FileID, nil, mergedAct)
	}

	// 13. Remove cached entries for all source activities so they don't appear as stale candidates.
	for _, id := range allIDs {
		if err := h.DB.DeleteFusionCandidatesForActivity(ctx, id); err != nil {
			slog.Warn("fusion-merge: delete cached candidates failed", "id", id, "error", err)
		}
	}

	slog.Info("fusion-merge: completed", "new_id", merged.ID, "sources", allIDs)
	httputil.WriteJSON(w, http.StatusCreated, map[string]string{"id": merged.ID})
}

// fusionMergeStreamColumn kopiert eine einzelne Stream-Spalte von sourceID → targetID
// (nur für Zeitpunkte die in beiden Aktivitäten vorhanden sind).
func fusionMergeStreamColumn(ctx context.Context, q database.Querier, targetID, sourceID, column string) error {
	query := fmt.Sprintf(`
		UPDATE activity_streams t
		SET %s = s.%s
		FROM activity_streams s
		WHERE t.activity_id = $1
		  AND s.activity_id = $2
		  AND t.time = s.time
		  AND s.%s IS NOT NULL`,
		column, column, column)
	_, err := q.Exec(ctx, query, targetID, sourceID)
	return err
}

// fusionMergeGPSFromSource kopiert lat/lng von sourceID → targetID (timestamp-matched).
func fusionMergeGPSFromSource(ctx context.Context, q database.Querier, targetID, sourceID string) error {
	_, err := q.Exec(ctx, `
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

// fusionMergeActivityTags copies all tag associations from sourceID to targetID
// using the provided Querier (pool or transaction).
func fusionMergeActivityTags(ctx context.Context, q database.Querier, targetID, sourceID string) error {
	// Get all tag IDs from source.
	rows, err := q.Query(ctx,
		`SELECT tag_id FROM activity_tags WHERE activity_id = $1`, sourceID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	var tagIDs []string
	for rows.Next() {
		var tagID string
		if err := rows.Scan(&tagID); err != nil {
			return err
		}
		tagIDs = append(tagIDs, tagID)
	}
	rows.Close()

	if len(tagIDs) == 0 {
		return nil
	}

	// Link each tag to target (ON CONFLICT DO NOTHING to skip existing links).
	for _, tagID := range tagIDs {
		if _, err := q.Exec(ctx,
			`INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES ($1, $2, 'merge')
			 ON CONFLICT (activity_id, tag_id) DO NOTHING`,
			targetID, tagID,
		); err != nil {
			slog.Warn("fusion-merge: link tag failed", "tag_id", tagID, "error", err)
		}
		// Update denormalized activity_count.
		q.Exec(ctx, //nolint:errcheck
			`UPDATE tags
			 SET activity_count = (SELECT COUNT(DISTINCT activity_id) FROM activity_tags WHERE tag_id = $1)
			 WHERE id = $1`,
			tagID,
		)
	}

	// Sync the denormalized tags TEXT[] cache on the target.
	_, _ = q.Exec(ctx, `
		UPDATE activities a
		SET tags = COALESCE(
			ARRAY(
				SELECT DISTINCT t.name
				FROM activity_tags at_join
				JOIN tags t ON t.id = at_join.tag_id
				WHERE at_join.activity_id = a.id
			), '{}'::TEXT[]
		)
		WHERE a.id = $1`, targetID,
	)
	return nil
}

// fitLevelMerge lädt die Original-FIT-Dateien (via ShadowFS oder S3-Fallback),
// führt sie auf Protokoll-Ebene zusammen und speichert die neue FIT-Datei zurück.
// targetID ist die originale Ziel-Aktivität (für Map-Lookup + FIT-Lesen).
// mergedID ist die neue zusammengefasste Aktivität (wo das Ergebnis gespeichert wird).
// Unterstützt aktuell nur einen Source (den ersten in der Liste).
func (h *Handler) fitLevelMerge(
	ctx context.Context,
	userID, targetID, mergedID string,
	sourceIDs []string,
	fieldMap map[string]string,
	activities map[string]*model.Activity,
) error {
	if h.ShadowFS == nil {
		return fmt.Errorf("ShadowFS not configured")
	}
	if len(sourceIDs) == 0 {
		return fmt.Errorf("no sources for FIT-level merge")
	}

	target := activities[targetID]
	source := activities[sourceIDs[0]]

	// Beide Aktivitäten müssen FIT-Dateien mit gültigem Hash haben.
	if target.FileHash == nil || strings.HasSuffix(*target.FileHash, "-missing") {
		return fmt.Errorf("target activity %s has no original FIT file", targetID)
	}
	if source.FileHash == nil || strings.HasSuffix(*source.FileHash, "-missing") {
		return fmt.Errorf("source activity %s has no original FIT file", sourceIDs[0])
	}

	// Dateiformat prüfen — FIT-Level-Merge nur für FIT-Dateien möglich.
	targetExt := "fit"
	if target.OriginalFileFormat != nil {
		targetExt = *target.OriginalFileFormat
	}
	sourceExt := "fit"
	if source.OriginalFileFormat != nil {
		sourceExt = *source.OriginalFileFormat
	}
	if targetExt != "fit" || sourceExt != "fit" {
		return fmt.Errorf("FIT-level merge requires both files to be FIT format (target=%s, source=%s)", targetExt, sourceExt)
	}

	if len(sourceIDs) > 1 {
		slog.Warn("fusion-merge: FIT-level merge only supports 1 source, ignoring extra sources",
			"used", sourceIDs[0], "ignored", sourceIDs[1:])
	}

	const maxFITSize = 50 * 1024 * 1024 // 50 MB

	// readFITData liest eine FIT-Datei via ShadowFS.
	readFITData := func(activityID, fileHash string) ([]byte, error) {
		var fileID string
		_ = h.DB.Pool.QueryRow(ctx,
			`SELECT COALESCE(file_id::text, '') FROM activities WHERE id = $1 AND user_id = $2`,
			activityID, userID,
		).Scan(&fileID)
		if fileID == "" {
			return nil, fmt.Errorf("no file_id for activity %s", activityID)
		}
		rc, getErr := h.ShadowFS.Get(ctx, userID, fileID)
		if getErr != nil {
			return nil, fmt.Errorf("ShadowFS.Get for activity %s (file_id=%s): %w", activityID, fileID, getErr)
		}
		defer rc.Close()
		d, readErr := io.ReadAll(rc)
		if readErr != nil {
			return nil, fmt.Errorf("read ShadowFS entry %s: %w", fileID, readErr)
		}
		return d, nil
	}

	targetData, err := readFITData(targetID, *target.FileHash)
	if err != nil {
		return fmt.Errorf("read target FIT: %w", err)
	}
	if len(targetData) > maxFITSize {
		return fmt.Errorf("target FIT file too large: %d bytes (max %d)", len(targetData), maxFITSize)
	}

	sourceData, err := readFITData(sourceIDs[0], *source.FileHash)
	if err != nil {
		return fmt.Errorf("read source FIT: %w", err)
	}
	if len(sourceData) > maxFITSize {
		return fmt.Errorf("source FIT file too large: %d bytes (max %d)", len(sourceData), maxFITSize)
	}

	slog.Info("fusion-merge: FIT-level merge starting",
		"target", targetID, "target_bytes", len(targetData),
		"source", sourceIDs[0], "source_bytes", len(sourceData),
	)

	// FIT-Dateien auf Protokoll-Ebene zusammenführen.
	result, err := fusion.MergeFITFiles(targetData, sourceData, fieldMap, sourceIDs[0])
	if err != nil {
		return fmt.Errorf("FIT-level merge failed: %w", err)
	}

	// Neuen Hash für die zusammengeführte Datei berechnen.
	hashBytes := sha256.Sum256(result.Data)
	newHash := hex.EncodeToString(hashBytes[:])

	// Zusammengeführte FIT-Datei via ShadowFS speichern.
	rec, putErr := h.ShadowFS.Put(ctx, shadowfs.FileEntry{
		UserID:      userID,
		FileType:    "activity",
		ContentType: "application/octet-stream",
		FileName:    fmt.Sprintf("%s.fit", newHash),
		SizeBytes:   int64(len(result.Data)),
		FileHash:    newHash,
		Tags:        map[string]string{"format": "fit", "source": "fusion_merge"},
	}, bytes.NewReader(result.Data))
	if putErr != nil {
		return fmt.Errorf("ShadowFS.Put merged FIT: %w", putErr)
	}
	mergedFileID := rec.ID
	slog.Info("fusion-merge: FIT-level merge saved via ShadowFS",
		"target", targetID, "records", result.RecordCount, "file_id", mergedFileID)

	// file_hash + file_id der zusammengefassten Aktivität aktualisieren.
	if _, execErr := h.DB.Pool.Exec(ctx,
		`UPDATE activities SET file_hash = $2, file_id = $3, has_edits = true WHERE id = $1`,
		mergedID, newHash, mergedFileID,
	); execErr != nil {
		return fmt.Errorf("update file_hash/file_id: %w", execErr)
	}

	return nil
}

// createMergedActivity erstellt eine neue Aktivität als Kopie von src mit einer neuen UUID.
// Streams werden NICHT kopiert — das übernimmt der Aufrufer.
func createMergedActivity(ctx context.Context, q database.Querier, src *model.Activity) (*model.Activity, error) {
	newID := uuid.New().String()
	var createdAt, updatedAt time.Time
	err := q.QueryRow(ctx, `
		INSERT INTO activities (
			id, user_id, sport_type, sub_type,
			parsed_sport_type, parsed_sub_type,
			fit_sport_id, fit_sub_sport_id,
			title, title_auto, description,
			started_at, ended_at, elapsed_time_s, moving_time_s,
			distance_m, elevation_gain_m, elevation_loss_m,
			max_elevation_m, min_elevation_m,
			avg_speed_ms, max_speed_ms,
			avg_heart_rate, max_heart_rate,
			avg_power_w, max_power_w, normalized_power_w,
			avg_cadence, avg_temperature_c,
			tss, trimp, is_indoor, has_gps,
			start_location, end_location,
			source, source_device,
			visibility, enrichment_status, has_edits
		)
		SELECT
			$2, user_id, sport_type, sub_type,
			parsed_sport_type, parsed_sub_type,
			fit_sport_id, fit_sub_sport_id,
			title, title_auto, description,
			started_at, ended_at, elapsed_time_s, moving_time_s,
			distance_m, elevation_gain_m, elevation_loss_m,
			max_elevation_m, min_elevation_m,
			avg_speed_ms, max_speed_ms,
			avg_heart_rate, max_heart_rate,
			avg_power_w, max_power_w, normalized_power_w,
			avg_cadence, avg_temperature_c,
			tss, trimp, is_indoor, has_gps,
			start_location, end_location,
			source, source_device,
			visibility, 'pending', TRUE
		FROM activities WHERE id = $1
		RETURNING created_at, updated_at`,
		src.ID, newID,
	).Scan(&createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("create merged activity: %w", err)
	}
	result := *src
	result.ID = newID
	result.CreatedAt = createdAt
	result.UpdatedAt = updatedAt
	return &result, nil
}

// RevertFusion löscht eine Fusion-Ergebnis-Aktivität und stellt die Quellen wieder her.
// POST /api/v1/activities/{id}/revert-fusion
func (h *Handler) RevertFusion(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	activity, err := h.DB.GetActivity(ctx, id)
	if err != nil || activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}
	if activity.FusionSources == nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "activity is not a fusion result", r.URL.Path)
		return
	}

	// Alle versteckten Quellen (parent_id = id) wiederherstellen.
	if _, err := h.DB.Pool.Exec(ctx,
		`UPDATE activities SET parent_id = NULL WHERE parent_id = $1 AND user_id = $2`,
		id, userID,
	); err != nil {
		slog.Error("revert-fusion: restore sources failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to restore sources", r.URL.Path)
		return
	}

	// Clean up ShadowFS files before deleting the fusion result.
	h.cleanupActivityFiles(ctx, userID, id)

	// Fusion-Ergebnis löschen.
	if err := h.DB.DeleteActivity(ctx, userID, id); err != nil {
		slog.Error("revert-fusion: delete result failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete fusion result", r.URL.Path)
		return
	}

	// Remove cached fusion candidate entries for the deleted fusion result
	if err := h.DB.DeleteFusionCandidatesForActivity(ctx, id); err != nil {
		slog.Warn("revert-fusion: delete cached candidates failed", "id", id, "error", err)
	}
	h.DB.InvalidateTerraIncognitaCache(ctx, userID)
	w.WriteHeader(http.StatusNoContent)
}
