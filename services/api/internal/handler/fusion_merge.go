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
// Sources enthält bis zu 5 zusätzliche Quell-Aktivitäts-IDs.
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
	// Kern
	"hr":          "heart_rate",
	"power":       "power_w",
	"cadence":     "cadence",
	"temperature": "temperature_c",
	"gps":         "gps",
	"altitude":    "altitude_m",
	"speed":       "speed_ms",
	// Running Dynamics
	"gct":                  "ground_contact_time_ms",
	"vertical_oscillation": "vertical_oscillation_mm",
	"stance_time":          "stance_time_percent",
	"step_length":          "step_length_mm",
	"left_right_balance":   "left_right_balance",
	// Pedal Analytics
	"torque_effectiveness_l": "left_torque_effectiveness_pct",
	"torque_effectiveness_r": "right_torque_effectiveness_pct",
	"pedal_smoothness_l":     "left_pedal_smoothness_pct",
	"pedal_smoothness_r":     "right_pedal_smoothness_pct",
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

	if len(req.Sources) > 5 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "max 6 activities total", r.URL.Path)
		return
	}

	allIDs := append([]string{targetID}, req.Sources...)

	// Alle Aktivitäten laden + Ownership prüfen.
	activities, ok := h.loadAndVerifyActivities(w, r, userID, allIDs)
	if !ok {
		return
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

	// 1b. FieldMap-aware metadata overrides: NULL external IDs, override aggregates
	// from designated sources, COALESCE session-only fields from other sources.
	if err := applyFusionMetadataOverrides(ctx, tx, merged.ID, targetID, req.FieldMap, activities); err != nil {
		slog.Error("fusion-merge: metadata overrides failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to apply metadata overrides", r.URL.Path)
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

	// 4-7 + rebuild + audit log: shared finalization within transaction.
	auditData, _ := json.Marshal(map[string]any{
		"type":      "fusion_merge",
		"sources":   allIDs,
		"field_map": req.FieldMap,
	})
	// V1: skip title override when the designated source is the target itself
	// (the merged activity already has the target's title from createMergedActivity).
	titleSource := req.TitleSource
	if titleSource != nil && *titleSource == targetID {
		titleSource = nil
	}
	fp := fusionFinalizeParams{
		userID:            userID,
		merged:            merged,
		allSourceIDs:      allIDs,
		activities:        activities,
		fieldMap:          req.FieldMap,
		titleSource:       titleSource,
		tagsMerge:         req.TagsMerge,
		gearSource:        req.GearSource,
		fitLevel:          req.FitLevel,
		fitLevelTargetID:  targetID,
		fitLevelSourceIDs: req.Sources,
		auditData:         auditData,
	}
	h.applyFusionTxExtras(ctx, tx, fp)

	// 8. Alle Quellen (inkl. Target) via parent_id verstecken + als Fusion-Quelle markieren.
	for _, id := range allIDs {
		if _, err := tx.Exec(ctx,
			`UPDATE activities SET parent_id = $2, is_fusion_source = TRUE WHERE id = $1 AND user_id = $3`,
			id, merged.ID, userID,
		); err != nil {
			slog.Warn("fusion-merge: hide source failed", "id", id, "error", err)
		}
	}

	// Commit transaction — all DB mutations are now durable.
	if err := tx.Commit(ctx); err != nil {
		slog.Error("fusion-merge: commit transaction failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to commit transaction", r.URL.Path)
		return
	}

	// Post-commit: FIT-level merge, sidecar write, candidate cache cleanup.
	h.postCommitFusionCleanup(ctx, fp)

	slog.Info("fusion-merge: completed", "new_id", merged.ID, "sources", allIDs)
	httputil.WriteJSON(w, http.StatusCreated, map[string]string{"id": merged.ID})
}

// loadAndVerifyActivities loads all activities in ids, checks ownership against userID,
// and returns a map of id → *model.Activity.
// On any error it writes the appropriate HTTP response and returns (nil, false).
func (h *Handler) loadAndVerifyActivities(w http.ResponseWriter, r *http.Request, userID string, ids []string) (map[string]*model.Activity, bool) {
	activities := make(map[string]*model.Activity, len(ids))
	for _, id := range ids {
		act, err := h.DB.GetActivity(r.Context(), id)
		if err != nil || act == nil {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", fmt.Sprintf("activity %s not found", id), r.URL.Path)
			return nil, false
		}
		if act.UserID != userID {
			httputil.WriteError(w, http.StatusForbidden, "Forbidden", "not your activity", r.URL.Path)
			return nil, false
		}
		activities[id] = act
	}
	return activities, true
}

// fusionFinalizeParams holds all inputs needed by finalizeFusionActivity.
type fusionFinalizeParams struct {
	// userID is the authenticated user.
	userID string
	// merged is the newly created merged activity (result of createMergedActivity).
	merged *model.Activity
	// allSourceIDs is the list of all source activity IDs (target + extras in V1; req.Sources in V2).
	allSourceIDs []string
	// activities is the full activity map for title/gear lookups.
	activities map[string]*model.Activity
	// fieldMap maps stream fields to source IDs.
	fieldMap map[string]string
	// titleSource, if non-nil, designates the source for the title.
	titleSource *string
	// tagsMerge, if true, merges tags from all sources.
	tagsMerge bool
	// gearSource, if non-nil, designates the source for gear.
	gearSource *string
	// fitLevel, if true, triggers a FIT-protocol-level merge post-commit.
	fitLevel bool
	// fitLevelTargetID is the "target" activity for fitLevelMerge (targetID in V1, Sources[0] in V2).
	fitLevelTargetID string
	// fitLevelSourceIDs are the extra sources for fitLevelMerge (req.Sources in V1, req.Sources[1:] in V2).
	fitLevelSourceIDs []string
	// auditData is marshalled JSON for the activity_edits record.
	auditData []byte
}

// applyFusionTxExtras applies the shared in-transaction finalization steps to the merged activity:
// title update (step 4), gear copy (step 5), tags merge (step 6), fusion_sources (step 7),
// RebuildActivityTrack + RebuildHeatmapData + InvalidateTerraIncognitaCache (pre-commit rebuilds),
// and audit log insert.
// Must be called before tx.Commit. Does not commit.
func (h *Handler) applyFusionTxExtras(ctx context.Context, tx database.Querier, p fusionFinalizeParams) {
	// Step 4: Title from designated source.
	if p.titleSource != nil {
		if src, ok := p.activities[*p.titleSource]; ok && src.Title != nil {
			if _, err := tx.Exec(ctx,
				`UPDATE activities SET title = $2, has_edits = true WHERE id = $1`,
				p.merged.ID, *src.Title,
			); err != nil {
				slog.Warn("fusion-merge: update title failed", "error", err)
			}
		}
	}

	// Step 5: Gear from designated source.
	if p.gearSource != nil {
		if src, ok := p.activities[*p.gearSource]; ok && src.GearID != nil {
			tx.Exec(ctx, `UPDATE activities SET gear_id = $2, has_edits = true WHERE id = $1`, p.merged.ID, src.GearID) //nolint:errcheck
		}
	}

	// Step 6: Tags merge from all source activities.
	if p.tagsMerge {
		for _, srcID := range p.allSourceIDs {
			if err := fusionMergeActivityTags(ctx, tx, p.merged.ID, srcID); err != nil {
				slog.Warn("fusion-merge: merge tags failed", "source", srcID, "error", err)
			}
		}
	}

	// Step 7: Set fusion_sources on new activity.
	sourceMap := make(database.FusionSources, len(p.fieldMap))
	maps.Copy(sourceMap, p.fieldMap)
	sourcesJSON, err := json.Marshal(sourceMap)
	if err != nil {
		slog.Warn("fusion-merge: marshal fusion sources failed", "error", err)
	} else {
		if _, err := tx.Exec(ctx,
			`UPDATE activities SET fusion_sources = $2 WHERE id = $1 AND user_id = $3`,
			p.merged.ID, sourcesJSON, p.userID,
		); err != nil {
			slog.Warn("fusion-merge: set fusion sources failed", "error", err)
		}
	}

	// Rebuild track + heatmap (best-effort, uses db.Pool internally — outside tx).
	if p.merged.HasGPS {
		if err := h.DB.RebuildActivityTrack(ctx, p.merged.ID); err != nil {
			slog.Warn("fusion-merge: rebuild track failed", "error", err)
		}
		if !p.merged.ExcludeFromHeatmap {
			if err := h.DB.RebuildHeatmapData(ctx, p.merged.ID); err != nil {
				slog.Warn("fusion-merge: rebuild heatmap failed", "error", err)
			}
		}
		h.DB.InvalidateTerraIncognitaCache(ctx, p.userID)
	}

	// Audit log (inside transaction).
	if _, err := tx.Exec(ctx,
		`INSERT INTO activity_edits (activity_id, user_id, edit_type, edit_data)
		 VALUES ($1, $2, 'fusion_merge', $3)`,
		p.merged.ID, p.userID, p.auditData,
	); err != nil {
		slog.Warn("fusion-merge: failed to insert audit record", "id", p.merged.ID, "error", err)
	}
}

// postCommitFusionCleanup runs the shared post-commit steps:
// FIT-level merge (if requested), sidecar .meta write, and fusion candidate cache invalidation.
func (h *Handler) postCommitFusionCleanup(ctx context.Context, p fusionFinalizeParams) {
	// FIT-level merge (non-DB, after commit, non-fatal).
	if p.fitLevel && p.fitLevelTargetID != "" {
		if err := h.fitLevelMerge(ctx, p.userID, p.fitLevelTargetID, p.merged.ID, p.fitLevelSourceIDs, p.fieldMap, p.activities); err != nil {
			slog.Error("fusion-merge: FIT-level merge failed", "target", p.merged.ID, "error", err)
		} else {
			slog.Info("fusion-merge: FIT-level merge completed", "target", p.merged.ID)
		}
	}

	// Write .meta companion for merged activity (ShadowFS).
	mergedAct, _ := h.DB.GetActivity(ctx, p.merged.ID)
	if mergedAct != nil && mergedAct.FileID != nil && *mergedAct.FileID != "" {
		sidecar.WriteMetaCompanion(ctx, h.ShadowFS, h.DB, p.userID, p.merged.ID, *mergedAct.FileID, nil, mergedAct)
	}

	// Remove cached fusion candidates for all source activities.
	for _, id := range p.allSourceIDs {
		if err := h.DB.DeleteFusionCandidatesForActivity(ctx, id); err != nil {
			slog.Warn("fusion-merge: delete cached candidates failed", "id", id, "error", err)
		}
	}
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
// Alle Spalten aus der activities-Tabelle werden kopiert, außer:
//   - id (neue UUID), created_at/updated_at (auto), enrichment_status ('pending'),
//   - has_edits (TRUE), parent_id (NULL), is_fusion_source (FALSE),
//   - is_group_secondary (FALSE, trigger), fusion_sources (separat gesetzt),
//   - file_hash/file_id (NULL oder später per FIT-Merge gesetzt).
func createMergedActivity(ctx context.Context, q database.Querier, src *model.Activity) (*model.Activity, error) {
	newID := uuid.New().String()
	var createdAt, updatedAt time.Time
	err := q.QueryRow(ctx, `
		INSERT INTO activities (
			id, user_id,
			sport_type, sub_type, parsed_sport_type, parsed_sub_type,
			fit_sport_id, fit_sub_sport_id,
			title, title_auto, title_is_manual, description,
			started_at, ended_at, elapsed_time_s, moving_time_s,
			distance_m, elevation_gain_m, elevation_loss_m,
			max_elevation_m, min_elevation_m,
			avg_speed_ms, max_speed_ms, min_speed_ms,
			avg_heart_rate, max_heart_rate, min_heart_rate,
			avg_power_w, max_power_w, min_power_w, normalized_power_w,
			avg_cadence, max_cadence,
			avg_temperature_c, min_temperature_c, max_temperature_c,
			tss, trimp,
			is_indoor, has_gps,
			start_location, end_location,
			source, source_device,
			visibility, enrichment_status, has_edits,
			original_file_format, original_filename,
			gear_id, tags,
			hide_map, data_visibility,
			start_city, end_city, start_state, start_country, start_country_code,
			is_round_trip, intensity, time_of_day,
			sport_type_is_manual,
			sunrise_at, sunset_at,
			total_calories,
			aerobic_training_effect, anaerobic_training_effect,
			training_load_peak,
			avg_spo2_pct, avg_respiration_rate, max_respiration_rate, min_respiration_rate,
			avg_stress_level,
			workout_rpe, workout_feel,
			session_rmssd_ms, sdrr_hrv_ms,
			avg_core_temperature_c, min_core_temperature_c, max_core_temperature_c,
			threshold_power_w,
			variability_index, efficiency_factor, aerobic_decoupling,
			lift_count, lift_distance_m, lift_ascent_m, lift_descent_m, lift_duration_s,
			net_distance_m, net_ascent_m, net_descent_m, exclude_lift_subtraction,
			location_lat, location_lng,
			weather_temp_min_c, weather_temp_max_c, weather_wind_speed_max_ms,
			garmin_activity_id, strava_activity_id, komoot_tour_id,
			edits_applied_at,
			is_commute,
			elevation_source, distance_source,
			is_electric,
			sport_conflict_pending,
			recording_device_id,
			total_cycles, total_work_j, total_fat_calories, metabolic_calories,
			total_moving_time_s,
			swim_stroke, pool_length_m, pool_length_unit,
			num_lengths, num_active_lengths,
			avg_stroke_count, avg_stroke_distance_m, stroke_count,
			zone_count,
			avg_fractional_cadence, max_fractional_cadence, total_fractional_cycles,
			avg_vertical_oscillation_mm,
			avg_stance_time_ms, avg_stance_time_pct, avg_stance_time_balance_pct,
			avg_step_length_mm, avg_vertical_ratio_pct,
			avg_grade_pct, avg_pos_grade_pct, avg_neg_grade_pct,
			max_pos_grade_pct, max_neg_grade_pct,
			avg_pos_vertical_speed_ms, avg_neg_vertical_speed_ms,
			max_pos_vertical_speed_ms, max_neg_vertical_speed_ms,
			avg_vam_ms, avg_altitude_m,
			total_fractional_ascent_m, total_fractional_descent_m,
			avg_left_torque_effectiveness_pct, avg_right_torque_effectiveness_pct,
			avg_left_pedal_smoothness_pct, avg_right_pedal_smoothness_pct,
			avg_combined_pedal_smoothness_pct,
			avg_left_pco_mm, avg_right_pco_mm,
			avg_left_power_phase_start_deg, avg_left_power_phase_end_deg,
			avg_left_power_phase_peak_start_deg, avg_left_power_phase_peak_end_deg,
			avg_right_power_phase_start_deg, avg_right_power_phase_end_deg,
			avg_right_power_phase_peak_start_deg, avg_right_power_phase_peak_end_deg,
			avg_power_position_w, max_power_position_w,
			avg_cadence_position, max_cadence_position,
			time_standing_s, stand_count,
			avg_lev_motor_power_w, max_lev_motor_power_w, lev_battery_consumption_pct,
			total_grit, total_flow, avg_grit, avg_flow,
			jump_count,
			max_ball_speed_ms, avg_ball_speed_ms,
			player_score, opponent_score, opponent_name,
			avg_total_hemoglobin_conc, min_total_hemoglobin_conc, max_total_hemoglobin_conc,
			avg_saturated_hemoglobin_pct, min_saturated_hemoglobin_pct, max_saturated_hemoglobin_pct,
			avg_depth_m, max_depth_m,
			surface_interval_s, dive_number,
			start_cns_pct, end_cns_pct, start_n2_pct, end_n2_pct, o2_toxicity_otu,
			avg_lap_time_s, best_lap_index, num_laps, first_lap_index,
			intensity_factor, left_right_balance,
			sport_profile_name, sport_index,
			nec_lat, nec_lng, swc_lat, swc_lng,
			workout_id,
			is_manual_entry, is_virtual,
			exclude_from_heatmap, exclude_from_stats,
			sidecar_file_id,
			computed_tss, computed_np_w, computed_ngp_mps, tss_method, computed_if,
			timezone,
			num_sessions,
			vo2max_ml_kg_min, vo2max_calibrated, vo2max_hr_source, vo2max_speed_source,
			gps_accuracy_m
		)
		SELECT
			$2, user_id,
			sport_type, sub_type, parsed_sport_type, parsed_sub_type,
			fit_sport_id, fit_sub_sport_id,
			title, title_auto, title_is_manual, description,
			started_at, ended_at, elapsed_time_s, moving_time_s,
			distance_m, elevation_gain_m, elevation_loss_m,
			max_elevation_m, min_elevation_m,
			avg_speed_ms, max_speed_ms, min_speed_ms,
			avg_heart_rate, max_heart_rate, min_heart_rate,
			avg_power_w, max_power_w, min_power_w, normalized_power_w,
			avg_cadence, max_cadence,
			avg_temperature_c, min_temperature_c, max_temperature_c,
			tss, trimp,
			is_indoor, has_gps,
			start_location, end_location,
			source, source_device,
			visibility, 'pending', TRUE,
			original_file_format, original_filename,
			gear_id, tags,
			hide_map, data_visibility,
			start_city, end_city, start_state, start_country, start_country_code,
			is_round_trip, intensity, time_of_day,
			sport_type_is_manual,
			sunrise_at, sunset_at,
			total_calories,
			aerobic_training_effect, anaerobic_training_effect,
			training_load_peak,
			avg_spo2_pct, avg_respiration_rate, max_respiration_rate, min_respiration_rate,
			avg_stress_level,
			workout_rpe, workout_feel,
			session_rmssd_ms, sdrr_hrv_ms,
			avg_core_temperature_c, min_core_temperature_c, max_core_temperature_c,
			threshold_power_w,
			variability_index, efficiency_factor, aerobic_decoupling,
			lift_count, lift_distance_m, lift_ascent_m, lift_descent_m, lift_duration_s,
			net_distance_m, net_ascent_m, net_descent_m, exclude_lift_subtraction,
			location_lat, location_lng,
			weather_temp_min_c, weather_temp_max_c, weather_wind_speed_max_ms,
			garmin_activity_id, strava_activity_id, komoot_tour_id,
			edits_applied_at,
			is_commute,
			elevation_source, distance_source,
			is_electric,
			sport_conflict_pending,
			recording_device_id,
			total_cycles, total_work_j, total_fat_calories, metabolic_calories,
			total_moving_time_s,
			swim_stroke, pool_length_m, pool_length_unit,
			num_lengths, num_active_lengths,
			avg_stroke_count, avg_stroke_distance_m, stroke_count,
			zone_count,
			avg_fractional_cadence, max_fractional_cadence, total_fractional_cycles,
			avg_vertical_oscillation_mm,
			avg_stance_time_ms, avg_stance_time_pct, avg_stance_time_balance_pct,
			avg_step_length_mm, avg_vertical_ratio_pct,
			avg_grade_pct, avg_pos_grade_pct, avg_neg_grade_pct,
			max_pos_grade_pct, max_neg_grade_pct,
			avg_pos_vertical_speed_ms, avg_neg_vertical_speed_ms,
			max_pos_vertical_speed_ms, max_neg_vertical_speed_ms,
			avg_vam_ms, avg_altitude_m,
			total_fractional_ascent_m, total_fractional_descent_m,
			avg_left_torque_effectiveness_pct, avg_right_torque_effectiveness_pct,
			avg_left_pedal_smoothness_pct, avg_right_pedal_smoothness_pct,
			avg_combined_pedal_smoothness_pct,
			avg_left_pco_mm, avg_right_pco_mm,
			avg_left_power_phase_start_deg, avg_left_power_phase_end_deg,
			avg_left_power_phase_peak_start_deg, avg_left_power_phase_peak_end_deg,
			avg_right_power_phase_start_deg, avg_right_power_phase_end_deg,
			avg_right_power_phase_peak_start_deg, avg_right_power_phase_peak_end_deg,
			avg_power_position_w, max_power_position_w,
			avg_cadence_position, max_cadence_position,
			time_standing_s, stand_count,
			avg_lev_motor_power_w, max_lev_motor_power_w, lev_battery_consumption_pct,
			total_grit, total_flow, avg_grit, avg_flow,
			jump_count,
			max_ball_speed_ms, avg_ball_speed_ms,
			player_score, opponent_score, opponent_name,
			avg_total_hemoglobin_conc, min_total_hemoglobin_conc, max_total_hemoglobin_conc,
			avg_saturated_hemoglobin_pct, min_saturated_hemoglobin_pct, max_saturated_hemoglobin_pct,
			avg_depth_m, max_depth_m,
			surface_interval_s, dive_number,
			start_cns_pct, end_cns_pct, start_n2_pct, end_n2_pct, o2_toxicity_otu,
			avg_lap_time_s, best_lap_index, num_laps, first_lap_index,
			intensity_factor, left_right_balance,
			sport_profile_name, sport_index,
			nec_lat, nec_lng, swc_lat, swc_lng,
			workout_id,
			is_manual_entry, is_virtual,
			exclude_from_heatmap, exclude_from_stats,
			sidecar_file_id,
			computed_tss, computed_np_w, computed_ngp_mps, tss_method, computed_if,
			timezone,
			num_sessions,
			vo2max_ml_kg_min, vo2max_calibrated, vo2max_hr_source, vo2max_speed_source,
			gps_accuracy_m
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

// fieldMapAggregates maps FieldMap keys to the correlated aggregate columns in the
// activities table. When the user chose a stream field from a non-richest source,
// the corresponding aggregates must be overwritten from that source.
var fieldMapAggregates = map[string][]string{
	"hr":          {"avg_heart_rate", "max_heart_rate", "min_heart_rate"},
	"power":       {"avg_power_w", "max_power_w", "min_power_w", "normalized_power_w", "threshold_power_w"},
	"cadence":     {"avg_cadence", "max_cadence"},
	"speed":       {"avg_speed_ms", "max_speed_ms", "min_speed_ms"},
	"altitude":    {"elevation_gain_m", "elevation_loss_m", "max_elevation_m", "min_elevation_m"},
	"temperature": {"avg_temperature_c", "min_temperature_c", "max_temperature_c"},
	"gps":         {"distance_m", "has_gps", "start_location", "end_location"},
}

// sessionOnlyCOALESCECols are nullable session-only columns that enrichment does NOT
// recompute. They are filled via COALESCE from other sources when the richest source
// has NULL. Excludes: enrichment-computed fields (time_of_day, timezone, sunrise_at,
// sunset_at, start_city, end_city, etc.), FieldMap-correlated fields (handled separately),
// and external IDs (NULLed out explicitly).
var sessionOnlyCOALESCECols = []string{
	// Training / health
	"training_load_peak", "aerobic_training_effect", "anaerobic_training_effect",
	"avg_spo2_pct", "avg_respiration_rate", "max_respiration_rate", "min_respiration_rate",
	"avg_stress_level", "workout_rpe", "workout_feel",
	"session_rmssd_ms", "sdrr_hrv_ms",
	"avg_core_temperature_c", "min_core_temperature_c", "max_core_temperature_c",
	"total_calories",
	// Computed metrics (from enrichment but session-sourced)
	"tss", "trimp",
	"variability_index", "efficiency_factor", "aerobic_decoupling",
	"computed_tss", "computed_np_w", "computed_ngp_mps", "tss_method", "computed_if",
	"intensity_factor", "left_right_balance",
	// Movement / cycles
	"total_cycles", "total_work_j", "total_fat_calories", "metabolic_calories",
	"total_moving_time_s",
	// Swimming
	"swim_stroke", "pool_length_m", "pool_length_unit",
	"num_lengths", "num_active_lengths",
	"avg_stroke_count", "avg_stroke_distance_m", "stroke_count",
	"zone_count",
	// Cadence extended
	"avg_fractional_cadence", "max_fractional_cadence", "total_fractional_cycles",
	// Running dynamics
	"avg_vertical_oscillation_mm",
	"avg_stance_time_ms", "avg_stance_time_pct", "avg_stance_time_balance_pct",
	"avg_step_length_mm", "avg_vertical_ratio_pct",
	// Grade
	"avg_grade_pct", "avg_pos_grade_pct", "avg_neg_grade_pct",
	"max_pos_grade_pct", "max_neg_grade_pct",
	// Vertical speed
	"avg_pos_vertical_speed_ms", "avg_neg_vertical_speed_ms",
	"max_pos_vertical_speed_ms", "max_neg_vertical_speed_ms",
	"avg_vam_ms",
	// Altitude extended
	"avg_altitude_m", "total_fractional_ascent_m", "total_fractional_descent_m",
	// Pedal analytics
	"avg_left_torque_effectiveness_pct", "avg_right_torque_effectiveness_pct",
	"avg_left_pedal_smoothness_pct", "avg_right_pedal_smoothness_pct",
	"avg_combined_pedal_smoothness_pct",
	"avg_left_pco_mm", "avg_right_pco_mm",
	"avg_left_power_phase_start_deg", "avg_left_power_phase_end_deg",
	"avg_left_power_phase_peak_start_deg", "avg_left_power_phase_peak_end_deg",
	"avg_right_power_phase_start_deg", "avg_right_power_phase_end_deg",
	"avg_right_power_phase_peak_start_deg", "avg_right_power_phase_peak_end_deg",
	// Power/cadence by position
	"avg_power_position_w", "max_power_position_w",
	"avg_cadence_position", "max_cadence_position",
	// Standing/sitting
	"time_standing_s", "stand_count",
	// E-Bike
	"avg_lev_motor_power_w", "max_lev_motor_power_w", "lev_battery_consumption_pct",
	// MTB Grit/Flow
	"total_grit", "total_flow", "avg_grit", "avg_flow",
	// Jumps
	"jump_count",
	// Ball sports
	"max_ball_speed_ms", "avg_ball_speed_ms",
	"player_score", "opponent_score", "opponent_name",
	// Hemoglobin
	"avg_total_hemoglobin_conc", "min_total_hemoglobin_conc", "max_total_hemoglobin_conc",
	"avg_saturated_hemoglobin_pct", "min_saturated_hemoglobin_pct", "max_saturated_hemoglobin_pct",
	// Diving
	"avg_depth_m", "max_depth_m",
	"surface_interval_s", "dive_number",
	"start_cns_pct", "end_cns_pct", "start_n2_pct", "end_n2_pct", "o2_toxicity_otu",
	// Laps
	"avg_lap_time_s", "best_lap_index", "num_laps", "first_lap_index",
	// Misc session
	"sport_profile_name", "sport_index",
	"nec_lat", "nec_lng", "swc_lat", "swc_lng",
	// VO2max
	"vo2max_ml_kg_min", "vo2max_calibrated", "vo2max_hr_source", "vo2max_speed_source",
	// GPS metadata
	"gps_accuracy_m",
	// Lift stats
	"lift_count", "lift_distance_m", "lift_ascent_m", "lift_descent_m", "lift_duration_s",
	"net_distance_m", "net_ascent_m", "net_descent_m",
	// Weather
	"weather_temp_min_c", "weather_temp_max_c", "weather_wind_speed_max_ms",
	// Misc
	"num_sessions",
}

// applyFusionMetadataOverrides performs 3 post-INSERT steps on the merged activity:
//  1. NULL out external IDs and set source = 'fusion'
//  2. Override FieldMap-correlated aggregates from designated sources
//  3. COALESCE session-only fields from non-richest sources
//
// allSourceIDs is derived from the keys of allActivities; callers no longer pass it separately.
func applyFusionMetadataOverrides(
	ctx context.Context,
	q database.Querier,
	mergedID string,
	richestSourceID string,
	fieldMap map[string]string,
	allActivities map[string]*model.Activity,
) error {
	// Derive allSourceIDs from the map so callers don't duplicate this.
	allSourceIDs := make([]string, 0, len(allActivities))
	for id := range allActivities {
		allSourceIDs = append(allSourceIDs, id)
	}
	// Step 1: NULL out external IDs + set source = 'fusion'.
	if _, err := q.Exec(ctx, `UPDATE activities SET
		garmin_activity_id = NULL,
		strava_activity_id = NULL,
		komoot_tour_id = NULL,
		source = 'fusion',
		source_device = NULL
	WHERE id = $1`, mergedID); err != nil {
		return fmt.Errorf("null external IDs: %w", err)
	}

	// Step 2: FieldMap-correlated aggregates.
	// For each FieldMap entry where the designated source differs from the richest
	// source (the one we INSERT-SELECT'd from), UPDATE the correlated aggregates.
	for field, sourceID := range fieldMap {
		if sourceID == richestSourceID || sourceID == "fuse" {
			continue // already correct from INSERT SELECT, or GPS fuse
		}
		cols, ok := fieldMapAggregates[field]
		if !ok {
			continue
		}
		// Build SET clause: col1 = s.col1, col2 = s.col2, ...
		var setClauses []string
		for _, c := range cols {
			setClauses = append(setClauses, fmt.Sprintf("%s = s.%s", c, c))
		}
		query := fmt.Sprintf(
			`UPDATE activities t SET %s FROM activities s WHERE t.id = $1 AND s.id = $2`,
			strings.Join(setClauses, ", "),
		)
		if _, err := q.Exec(ctx, query, mergedID, sourceID); err != nil {
			slog.Warn("fusion metadata: fieldmap aggregate override failed",
				"field", field, "source", sourceID, "error", err)
		}
	}

	// Step 3: COALESCE session-only fields from non-richest sources.
	// Build a list of other sources sorted by field-richness DESC (excluding richest).
	type scoredSource struct {
		id    string
		score int
	}
	var otherSources []scoredSource
	for _, id := range allSourceIDs {
		if id == richestSourceID {
			continue
		}
		if act, ok := allActivities[id]; ok {
			otherSources = append(otherSources, scoredSource{id: id, score: activityFieldRichness(act)})
		}
	}
	slices.SortFunc(otherSources, func(a, b scoredSource) int {
		return b.score - a.score // DESC
	})

	if len(otherSources) > 0 && len(sessionOnlyCOALESCECols) > 0 {
		// Build SET clause: col = COALESCE(t.col, s.col) for each session-only column.
		var setClauses []string
		for _, c := range sessionOnlyCOALESCECols {
			setClauses = append(setClauses, fmt.Sprintf("%s = COALESCE(t.%s, s.%s)", c, c, c))
		}
		coalQuery := fmt.Sprintf(
			`UPDATE activities t SET %s FROM activities s WHERE t.id = $1 AND s.id = $2`,
			strings.Join(setClauses, ", "),
		)
		for _, src := range otherSources {
			if _, err := q.Exec(ctx, coalQuery, mergedID, src.id); err != nil {
				slog.Warn("fusion metadata: COALESCE session fields failed",
					"source", src.id, "error", err)
			}
		}
	}

	return nil
}

// FusionMergeRequestV2 describes the body for POST /api/v1/fusion-merge (no target).
// All sources are equal — the unified timeline is built from ALL timestamps.
type FusionMergeRequestV2 struct {
	Sources         []string          `json:"sources"`            // all equal, min 2, max 6
	FieldMap        map[string]string `json:"field_map"`          // "hr" → "uuid-B", "gps" → "fuse"
	GPSFusionConfig *GPSFusionConfig  `json:"gps_fusion_config,omitempty"`
	TitleSource     *string           `json:"title_source,omitempty"`
	TagsMerge       bool              `json:"tags_merge"`
	GearSource      *string           `json:"gear_source,omitempty"`
	FitLevel        bool              `json:"fit_level"`
	GroupID         *string           `json:"group_id,omitempty"` // if fusing from a recording group
}

// FusionMergeV2 implements the unified-timeline merge without a target activity.
// POST /api/v1/fusion-merge
func (h *Handler) FusionMergeV2(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	req, ok := httputil.DecodeJSON[FusionMergeRequestV2](w, r)
	if !ok {
		return
	}

	if len(req.Sources) < 2 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "at least 2 sources required", r.URL.Path)
		return
	}
	if len(req.Sources) > 6 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "max 6 sources", r.URL.Path)
		return
	}

	// Load all activities + ownership check.
	activities, ok := h.loadAndVerifyActivities(w, r, userID, req.Sources)
	if !ok {
		return
	}

	// Validate FieldMap.
	for field, sourceID := range req.FieldMap {
		if _, ok := validStreamFields[field]; !ok {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", fmt.Sprintf("unknown field: %s", field), r.URL.Path)
			return
		}
		if sourceID != "fuse" && !slices.Contains(req.Sources, sourceID) {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", fmt.Sprintf("source %s not in sources list", sourceID), r.URL.Path)
			return
		}
		if sourceID == "fuse" && field != "gps" {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "fuse only allowed for gps field", r.URL.Path)
			return
		}
	}

	// Validate group_id if provided.
	if req.GroupID != nil && *req.GroupID != "" {
		group, err := h.DB.GetRecordingGroup(r.Context(), *req.GroupID)
		if err != nil || group == nil {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "recording group not found", r.URL.Path)
			return
		}
		if group.UserID != userID {
			httputil.WriteError(w, http.StatusForbidden, "Forbidden", "not your recording group", r.URL.Path)
			return
		}
	}

	ctx := r.Context()

	// Determine the richest source for metadata (most non-nil summary fields).
	metadataSource := pickRichestSource(activities, req.Sources)

	// Begin transaction.
	tx, err := h.DB.Pool.Begin(ctx)
	if err != nil {
		slog.Error("fusion-merge-v2: begin transaction failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to begin transaction", r.URL.Path)
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// 1. Create new activity from the richest source's metadata.
	merged, err := createMergedActivity(ctx, tx, metadataSource)
	if err != nil {
		slog.Error("fusion-merge-v2: create merged activity failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to create merged activity", r.URL.Path)
		return
	}

	// 1b. FieldMap-aware metadata overrides.
	if err := applyFusionMetadataOverrides(ctx, tx, merged.ID, metadataSource.ID, req.FieldMap, activities); err != nil {
		slog.Error("fusion-merge-v2: metadata overrides failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to apply metadata overrides", r.URL.Path)
		return
	}

	// 2. Build unified timeline: INSERT skeleton rows for all unique timestamps across
	//    all source activities referenced in the FieldMap.
	sourceSet := make(map[string]struct{})
	for _, srcID := range req.FieldMap {
		if srcID != "fuse" {
			sourceSet[srcID] = struct{}{}
		}
	}
	// For "fuse" GPS mode, all sources contribute timestamps.
	if req.FieldMap["gps"] == "fuse" {
		for _, id := range req.Sources {
			sourceSet[id] = struct{}{}
		}
	}

	uniqueSources := make([]string, 0, len(sourceSet))
	for id := range sourceSet {
		uniqueSources = append(uniqueSources, id)
	}

	if err := insertUnifiedTimeline(ctx, tx, merged.ID, uniqueSources); err != nil {
		slog.Error("fusion-merge-v2: build unified timeline failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to build unified timeline", r.URL.Path)
		return
	}

	// 3. For each field in FieldMap, overlay data from the designated source.
	for field, sourceID := range req.FieldMap {
		if field == "gps" {
			if sourceID == "fuse" {
				if err := h.fuseGPSTracks(ctx, tx, merged.ID, req.Sources, req.GPSFusionConfig); err != nil {
					httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
					return
				}
			} else {
				if err := fusionMergeGPSFromSource(ctx, tx, merged.ID, sourceID); err != nil {
					slog.Error("fusion-merge-v2: GPS merge failed", "error", err)
					httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "GPS merge failed", r.URL.Path)
					return
				}
			}
			continue
		}

		col := validStreamFields[field]
		if err := fusionMergeStreamColumn(ctx, tx, merged.ID, sourceID, col); err != nil {
			slog.Error("fusion-merge-v2: stream column merge failed", "field", field, "col", col, "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", fmt.Sprintf("merge field %s failed", field), r.URL.Path)
			return
		}
	}

	// 4-7 + rebuild + audit log: shared finalization within transaction.
	auditData, _ := json.Marshal(map[string]any{
		"type":      "fusion_merge_v2",
		"sources":   req.Sources,
		"field_map": req.FieldMap,
		"group_id":  req.GroupID,
	})
	var fitLevelTargetID string
	var fitLevelSourceIDs []string
	if req.FitLevel && len(req.Sources) >= 2 {
		fitLevelTargetID = req.Sources[0]
		fitLevelSourceIDs = req.Sources[1:]
	}
	fp := fusionFinalizeParams{
		userID:            userID,
		merged:            merged,
		allSourceIDs:      req.Sources,
		activities:        activities,
		fieldMap:          req.FieldMap,
		titleSource:       req.TitleSource,
		tagsMerge:         req.TagsMerge,
		gearSource:        req.GearSource,
		fitLevel:          req.FitLevel,
		fitLevelTargetID:  fitLevelTargetID,
		fitLevelSourceIDs: fitLevelSourceIDs,
		auditData:         auditData,
	}
	h.applyFusionTxExtras(ctx, tx, fp)

	// 8. Source visibility: group-aware vs legacy.
	if req.GroupID == nil || *req.GroupID == "" {
		// Legacy ad-hoc mode: hide sources via parent_id.
		// Group mode: sources stay visible; fused activity joins the group post-commit.
		for _, id := range req.Sources {
			if _, err := tx.Exec(ctx,
				`UPDATE activities SET parent_id = $2 WHERE id = $1 AND user_id = $3`,
				id, merged.ID, userID,
			); err != nil {
				slog.Warn("fusion-merge-v2: hide source failed", "id", id, "error", err)
			}
		}
	}

	// 9. Update started_at/ended_at from the unified timeline.
	if _, err := tx.Exec(ctx, `
		UPDATE activities SET
			started_at = COALESCE((SELECT MIN(time) FROM activity_streams WHERE activity_id = $1), started_at),
			ended_at   = COALESCE((SELECT MAX(time) FROM activity_streams WHERE activity_id = $1), ended_at)
		WHERE id = $1`, merged.ID); err != nil {
		slog.Warn("fusion-merge-v2: update time bounds failed", "error", err)
	}

	// Commit transaction.
	if err := tx.Commit(ctx); err != nil {
		slog.Error("fusion-merge-v2: commit transaction failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to commit transaction", r.URL.Path)
		return
	}

	// Post-commit: group integration.
	if req.GroupID != nil && *req.GroupID != "" {
		if err := h.DB.AddRecordingGroupMemberFused(ctx, *req.GroupID, merged.ID); err != nil {
			slog.Error("fusion-merge-v2: add fused member to group failed", "group", *req.GroupID, "error", err)
			// Non-fatal: the merge itself succeeded.
		}
	}

	// Post-commit: FIT-level merge, sidecar write, candidate cache cleanup.
	h.postCommitFusionCleanup(ctx, fp)

	slog.Info("fusion-merge-v2: completed", "new_id", merged.ID, "sources", req.Sources)
	httputil.WriteJSON(w, http.StatusCreated, map[string]string{"id": merged.ID})
}

// insertUnifiedTimeline creates skeleton rows in activity_streams for the merged activity
// using the UNION of all timestamps from the given source activities.
// Each row has only (activity_id, time) — field data is overlaid afterwards.
func insertUnifiedTimeline(ctx context.Context, q database.Querier, mergedID string, sourceIDs []string) error {
	if len(sourceIDs) == 0 {
		return nil
	}

	// Build a UNION ALL query for all source timestamps, then INSERT DISTINCT into merged.
	// Using a parameterized approach: $1 = mergedID, $2..$N+1 = sourceIDs.
	args := make([]any, 0, len(sourceIDs)+1)
	args = append(args, mergedID)

	var unions []string
	for i, id := range sourceIDs {
		paramIdx := i + 2
		unions = append(unions, fmt.Sprintf(
			`SELECT DISTINCT time FROM activity_streams WHERE activity_id = $%d`, paramIdx))
		args = append(args, id)
	}

	query := fmt.Sprintf(`
		INSERT INTO activity_streams (activity_id, time)
		SELECT $1, t.time FROM (
			%s
		) t
		GROUP BY t.time
		ORDER BY t.time ASC`,
		strings.Join(unions, " UNION ALL "))

	_, err := q.Exec(ctx, query, args...)
	return err
}

// pickRichestSource returns the activity with the most non-nil summary fields.
// This determines which source provides metadata (sport, times, etc.) for the merged activity.
func pickRichestSource(activities map[string]*model.Activity, sourceIDs []string) *model.Activity {
	var best *model.Activity
	bestScore := -1

	for _, id := range sourceIDs {
		act := activities[id]
		if act == nil {
			continue
		}
		score := activityFieldRichness(act)
		if score > bestScore {
			bestScore = score
			best = act
		}
	}
	return best
}

// activityFieldRichness counts how many optional summary fields are non-nil on an activity.
func activityFieldRichness(a *model.Activity) int {
	score := 0
	if a.Title != nil {
		score++
	}
	if a.Description != nil {
		score++
	}
	if a.EndedAt != nil {
		score++
	}
	if a.ElapsedTimeS != nil {
		score++
	}
	if a.MovingTimeS != nil {
		score++
	}
	if a.DistanceM != nil {
		score++
	}
	if a.ElevationGainM != nil {
		score++
	}
	if a.ElevationLossM != nil {
		score++
	}
	if a.AvgSpeedMS != nil {
		score++
	}
	if a.MaxSpeedMS != nil {
		score++
	}
	if a.AvgHeartRate != nil {
		score++
	}
	if a.MaxHeartRate != nil {
		score++
	}
	if a.AvgPowerW != nil {
		score++
	}
	if a.MaxPowerW != nil {
		score++
	}
	if a.NormalizedPowerW != nil {
		score++
	}
	if a.AvgCadence != nil {
		score++
	}
	if a.AvgTemperatureC != nil {
		score++
	}
	if a.TSS != nil {
		score++
	}
	if a.GearID != nil {
		score++
	}
	if a.HasGPS {
		score++
	}
	return score
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

	// Alle versteckten Quellen (parent_id = id) wiederherstellen + is_fusion_source zurücksetzen.
	// Für Group-Fusions (V2): parent_id war nie gesetzt, daher ist das ein No-Op für parent_id,
	// aber is_fusion_source = FALSE ist in jedem Fall korrekt (Legacy-Bugfix).
	// CASCADE entfernt die Fusion-Aktivität aus recording_group_members; der DEFERRED-Trigger
	// berechnet is_group_secondary + primary_activity_id neu.
	if _, err := h.DB.Pool.Exec(ctx,
		`UPDATE activities SET parent_id = NULL, is_fusion_source = FALSE WHERE parent_id = $1 AND user_id = $2`,
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
