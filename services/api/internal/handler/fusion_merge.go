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
	"github.com/skadia-app/skadia/services/api/internal/dbutil"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/fusion"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
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
	TagsSource      *string           `json:"tags_source,omitempty"`
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

// deriveFusionMemberships builds the membership list from the merge request.
// "fuse"-literal (GPS stitching) expands to one row per source in allSources.
// Concrete source-ids produce exactly one row each.
// userID is the owner — same for merged + all sources (fusion across users is rejected upstream).
func deriveFusionMemberships(mergedID, userID string, fieldMap map[string]string, allSources []string) []database.FusionMembership {
	out := make([]database.FusionMembership, 0, len(fieldMap)+len(allSources))
	for fieldKey, srcID := range fieldMap {
		if srcID == "fuse" {
			for _, s := range allSources {
				out = append(out, database.FusionMembership{
					MergedID: mergedID,
					SourceID: s,
					FieldKey: fieldKey,
					UserID:   userID,
				})
			}
			continue
		}
		out = append(out, database.FusionMembership{
			MergedID: mergedID,
			SourceID: srcID,
			FieldKey: fieldKey,
			UserID:   userID,
		})
	}
	return out
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
	if len(req.FieldMap) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "field_map must not be empty", r.URL.Path)
		return
	}
	if req.TagsMerge && req.TagsSource != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "tags_merge and tags_source are mutually exclusive", r.URL.Path)
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

	// tagsSource / titleSource / gearSource must reference one of the authorized sources.
	// Without this, an attacker could pass another user's activity UUID and leak data.
	if req.TagsSource != nil && !slices.Contains(allIDs, *req.TagsSource) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "tags_source not in sources list", r.URL.Path)
		return
	}
	if req.TitleSource != nil && !slices.Contains(allIDs, *req.TitleSource) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "title_source not in sources list", r.URL.Path)
		return
	}
	if req.GearSource != nil && !slices.Contains(allIDs, *req.GearSource) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "gear_source not in sources list", r.URL.Path)
		return
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

	// Serialise concurrent Merge/Revert for the same user via an advisory lock
	// that is scoped to the transaction (released automatically on Commit/Rollback).
	lockKey := dbutil.AdvisoryLockKey("fusion:" + userID)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockKey); err != nil {
		slog.Error("fusion-merge: advisory lock failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "fusion advisory lock: "+err.Error(), r.URL.Path)
		return
	}

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

	// 2. TEMP TABLE erstellen + Ziel-Activity hineinkopieren.
	// Alle weiteren Overlays werden auf der TEMP TABLE gemacht (kein Hypertable →
	// kein Compression-Problem). finalizeStreamsMerge schreibt danach in activity_streams.
	tempName, err := createTempMergedStreams(ctx, tx)
	if err != nil {
		slog.Error("fusion-merge: create temp table failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to create temp table", r.URL.Path)
		return
	}

	if err := copyTargetStreamsInto(ctx, tx, tempName, merged.ID, targetID); err != nil {
		slog.Error("fusion-merge: copy streams to temp failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to copy streams", r.URL.Path)
		return
	}

	// 3. Stream-Felder zusammenführen (auf TEMP TABLE angewendet).
	for field, sourceID := range req.FieldMap {
		if sourceID == merged.ID || sourceID == targetID {
			// Ziel = neue Aktivität oder original Target → Streams bereits in TEMP kopiert.
			continue
		}

		if field == "gps" {
			if sourceID == "fuse" {
				// allIDs = target + extras: GPS-Stitcher muss alle Quellen kennen
				// (inkl. Target-GPS das bereits in TEMP liegt), damit Target-Timestamps
				// nicht aus dem Stitching herausfallen.
				if err := h.fuseGPSTracks(ctx, tx, tempName, merged.ID, allIDs, req.GPSFusionConfig); err != nil {
					httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
					return
				}
			} else {
				if err := fusionMergeGPSFromSource(ctx, tx, tempName, sourceID); err != nil {
					slog.Error("fusion-merge: GPS merge failed", "error", err)
					httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "GPS merge failed", r.URL.Path)
					return
				}
			}
			continue
		}

		col := validStreamFields[field]
		if err := fusionMergeStreamColumn(ctx, tx, tempName, sourceID, col); err != nil {
			slog.Error("fusion-merge: stream column merge failed", "field", field, "col", col, "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", fmt.Sprintf("merge field %s failed", field), r.URL.Path)
			return
		}
	}

	// 3b. TEMP TABLE → activity_streams finalisieren.
	if err := finalizeStreamsMerge(ctx, tx, tempName); err != nil {
		slog.Error("fusion-merge: finalize streams merge failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to finalize streams", r.URL.Path)
		return
	}

	// 4-7 + rebuild + audit log: shared finalization within transaction.
	auditData, _ := json.Marshal(map[string]any{
		"type":      "fusion_merge",
		"sources":   allIDs,
		"field_map": req.FieldMap,
	})
	// V1: skip title/gear override when the designated source is the target itself
	// (the merged activity already has the target's title/gear from createMergedActivity).
	titleSource := req.TitleSource
	if titleSource != nil && *titleSource == targetID {
		titleSource = nil
	}
	gearSourceV1 := req.GearSource
	if gearSourceV1 != nil && *gearSourceV1 == targetID {
		gearSourceV1 = nil
	}
	fp := fusionFinalizeParams{
		userID:            userID,
		merged:            merged,
		allSourceIDs:      allIDs,
		activities:        activities,
		fieldMap:          req.FieldMap,
		titleSource:       titleSource,
		tagsMerge:         req.TagsMerge,
		tagsSource:        req.TagsSource,
		gearSource:        gearSourceV1,
		metadataSourceID:  targetID,
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

	// 8b. Dual-write: insert into activity_fusion_memberships (new normalised table).
	// allIDs = [targetID] + req.Sources — req.Sources contains the extra sources only,
	// so we pass req.Sources as allSources and include the full allIDs as the "all sources"
	// slice so that "fuse" expansions cover every participant.
	memberships := deriveFusionMemberships(merged.ID, userID, req.FieldMap, allIDs)
	if err := database.InsertFusionMemberships(ctx, tx, memberships); err != nil {
		slog.Error("fusion-merge: insert memberships failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "insert memberships (v1): "+err.Error(), r.URL.Path)
		return
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
	// tagsSource, if non-nil, designates a single source activity whose tags
	// overwrite the merged activity's tags. Ignored when tagsMerge is true
	// (fuse wins over single-source pick).
	tagsSource *string
	// gearSource, if non-nil, designates the source for gear.
	gearSource *string
	// metadataSourceID is the source whose metadata createMergedActivity copied
	// (targetID in V1, richest source ID in V2). Used as default tag source.
	metadataSourceID string
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
		if src, ok := p.activities[*p.gearSource]; ok {
			if src.GearID != nil {
				tx.Exec(ctx, `UPDATE activities SET gear_id = $2, has_edits = true WHERE id = $1`, p.merged.ID, src.GearID) //nolint:errcheck
			} else {
				slog.Info("fusion-merge: gearSource has no gear_id set, merged activity retains metadata-source gear",
					"gear_source", *p.gearSource)
			}
		}
	}

	// Step 6: Tags — three mutually exclusive modes:
	//   1. tagsMerge=true                     → union of all sources' tags (fuse)
	//   2. tagsSource!=nil && !tagsMerge      → overwrite with tags from the designated source
	//   3. default                            → Junction-Rows aus metadata-Source kopieren
	if p.tagsMerge {
		for _, srcID := range p.allSourceIDs {
			if err := fusionMergeActivityTags(ctx, tx, p.merged.ID, srcID); err != nil {
				slog.Warn("fusion-merge: merge tags failed", "source", srcID, "error", err)
			}
		}
		// Recompute tags.activity_count for all tags now linked to the merged activity.
		// fusionMergeActivityTags updates per-tag counts individually but does not handle
		// the denormalized TEXT[] cache sync on the activity itself (done via SQL inside
		// fusionMergeActivityTags already). We must still update activity_count here to
		// account for the merged activity being newly linked via the tagsMerge path.
		if _, err := tx.Exec(ctx,
			`UPDATE tags SET activity_count = (
			    SELECT COUNT(DISTINCT activity_id) FROM activity_tags WHERE tag_id = tags.id
			 ) WHERE id IN (SELECT tag_id FROM activity_tags WHERE activity_id = $1)`,
			p.merged.ID,
		); err != nil {
			slog.Warn("fusion-merge: recompute tags.activity_count after tagsMerge failed", "error", err)
		}
	} else if p.tagsSource != nil {
		// Keep activities.tags TEXT[] AND activity_tags junction in sync, otherwise
		// tag-filter queries (which go through the junction) will not find this activity.
		if _, err := tx.Exec(ctx,
			`UPDATE activities t SET tags = s.tags, has_edits = true
			 FROM activities s WHERE t.id = $1 AND s.id = $2`,
			p.merged.ID, *p.tagsSource,
		); err != nil {
			slog.Warn("fusion-merge: set tags TEXT[] from source failed", "source", *p.tagsSource, "error", err)
		} else {
			// Populate junction: copy all activity_tags rows from source to merged.
			// ON CONFLICT DO NOTHING because createMergedActivity's INSERT-SELECT
			// doesn't create junction rows (activity_tags is a separate table), so
			// the junction is empty at this point — the conflict guard is defensive.
			if _, err := tx.Exec(ctx,
				`INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
				 SELECT $1, tag_id, 'merge' FROM activity_tags WHERE activity_id = $2
				 ON CONFLICT (activity_id, tag_id) DO NOTHING`,
				p.merged.ID, *p.tagsSource,
			); err != nil {
				slog.Warn("fusion-merge: copy activity_tags junction from source failed", "source", *p.tagsSource, "error", err)
			}
			// Increment tags.activity_count for each tag now linked to the merged activity.
			if _, err := tx.Exec(ctx,
				`UPDATE tags SET activity_count = (
				    SELECT COUNT(DISTINCT activity_id) FROM activity_tags WHERE tag_id = tags.id
				 ) WHERE id IN (SELECT tag_id FROM activity_tags WHERE activity_id = $1)`,
				p.merged.ID,
			); err != nil {
				slog.Warn("fusion-merge: recompute tags.activity_count failed", "error", err)
			}
		}
	} else if p.metadataSourceID != "" {
		// Default: keine explizite Wahl → Junction-Rows aus metadata-Source kopieren.
		// createMergedActivity kopiert activities.tags TEXT[] per INSERT-SELECT, aber
		// activity_tags (Junction) ist eine separate Tabelle — sie muss manuell befüllt werden.
		if _, err := tx.Exec(ctx,
			`INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
			 SELECT $1, tag_id, 'merge' FROM activity_tags WHERE activity_id = $2
			 ON CONFLICT (activity_id, tag_id) DO NOTHING`,
			p.merged.ID, p.metadataSourceID,
		); err != nil {
			slog.Warn("fusion-merge: copy default tags junction failed", "error", err)
		} else {
			// activity_count neu berechnen: Tag war bisher an metadata-Source → count +1 (merged).
			if _, err := tx.Exec(ctx,
				`UPDATE tags SET activity_count = (
				    SELECT COUNT(DISTINCT activity_id) FROM activity_tags WHERE tag_id = tags.id
				 ) WHERE id IN (SELECT tag_id FROM activity_tags WHERE activity_id = $1)`,
				p.merged.ID,
			); err != nil {
				slog.Warn("fusion-merge: recompute default tags.activity_count failed", "error", err)
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

	// NOTE: Track + heatmap rebuild was moved to postCommitFusionCleanup.
	// These calls run on db.Pool (not tx) and therefore cannot see rows inserted
	// inside this still-open transaction. They must execute AFTER tx.Commit().

	// Step 7a: has_gps aus tatsächlich vorhandenen Streams ableiten.
	// Muss NACH dem Stream-Merge (Steps 2/3) und VOR dem Aggregate-Recompute (Step 7b)
	// laufen, damit der Wert auf den fusionierten Streams basiert — nicht auf der
	// Metadata-Source oder dem fieldMap (der User könnte "gps" auf eine GPS-lose
	// Source gesetzt haben oder "fuse" ohne GPS in irgendeiner Source).
	if _, err := tx.Exec(ctx, `
		UPDATE activities SET has_gps = EXISTS(
			SELECT 1 FROM activity_streams
			WHERE activity_id = $1 AND lat IS NOT NULL AND lng IS NOT NULL
		) WHERE id = $1`, p.merged.ID,
	); err != nil {
		slog.Warn("fusion-merge: has_gps recompute from streams failed", "error", err)
	}

	// Step 7b: Aggregate-Recompute aus den gemergten Streams.
	// COALESCE(sub.X, a.X): wenn der Stream-Wert NULL ist (z.B. weil das Feld nicht im
	// FieldMap enthalten war und die Streams daher NULL haben), bleibt der aus der
	// metadata-Source kopierte Wert erhalten statt auf NULL überschrieben zu werden.
	// elapsed_time_s wird NICHT neu berechnet: MAX(time)-MIN(time) ergibt die
	// Gesamtzeitspanne, nicht die moving_time. Der Wert wurde korrekt aus der
	// metadata-Source kopiert; Step 9 (V2) korrigiert started_at/ended_at separat.
	// elevation_gain_m/elevation_loss_m werden hier NICHT gesetzt — sie erfordern
	// kumulative Differenzberechnung (separater Algorithmus, TODO).
	if _, err := tx.Exec(ctx, `
		UPDATE activities a SET
			distance_m            = COALESCE(sub.max_dist,  a.distance_m),
			avg_speed_ms          = COALESCE(sub.avg_speed, a.avg_speed_ms),
			max_speed_ms          = COALESCE(sub.max_speed, a.max_speed_ms),
			min_speed_ms          = COALESCE(sub.min_speed, a.min_speed_ms),
			avg_heart_rate        = COALESCE(sub.avg_hr,    a.avg_heart_rate),
			max_heart_rate        = COALESCE(sub.max_hr,    a.max_heart_rate),
			min_heart_rate        = COALESCE(sub.min_hr,    a.min_heart_rate),
			avg_power_w           = COALESCE(sub.avg_pw,    a.avg_power_w),
			max_power_w           = COALESCE(sub.max_pw,    a.max_power_w),
			min_power_w           = COALESCE(sub.min_pw,    a.min_power_w),
			avg_cadence           = COALESCE(sub.avg_cad,   a.avg_cadence),
			max_cadence           = COALESCE(sub.max_cad,   a.max_cadence),
			avg_temperature_c     = COALESCE(sub.avg_temp,  a.avg_temperature_c),
			min_temperature_c     = COALESCE(sub.min_temp,  a.min_temperature_c),
			max_temperature_c     = COALESCE(sub.max_temp,  a.max_temperature_c),
			avg_altitude_m        = COALESCE(sub.avg_alt,   a.avg_altitude_m),
			max_elevation_m       = COALESCE(sub.max_alt,   a.max_elevation_m),
			min_elevation_m       = COALESCE(sub.min_alt,   a.min_elevation_m)
		FROM (
			SELECT
				MAX(distance_m)                                              AS max_dist,
				AVG(speed_ms) FILTER (WHERE speed_ms IS NOT NULL)::real      AS avg_speed,
				MAX(speed_ms)                                                AS max_speed,
				MIN(speed_ms) FILTER (WHERE speed_ms > 0)                    AS min_speed,
				AVG(heart_rate) FILTER (WHERE heart_rate IS NOT NULL)::int   AS avg_hr,
				MAX(heart_rate)                                              AS max_hr,
				MIN(heart_rate) FILTER (WHERE heart_rate > 0)                AS min_hr,
				AVG(power_w) FILTER (WHERE power_w IS NOT NULL)::int         AS avg_pw,
				MAX(power_w)                                                 AS max_pw,
				MIN(power_w) FILTER (WHERE power_w > 0)                      AS min_pw,
				AVG(cadence) FILTER (WHERE cadence IS NOT NULL)::int         AS avg_cad,
				MAX(cadence)                                                 AS max_cad,
				AVG(temperature_c) FILTER (WHERE temperature_c IS NOT NULL)::real AS avg_temp,
				MIN(temperature_c) FILTER (WHERE temperature_c IS NOT NULL)   AS min_temp,
				MAX(temperature_c) FILTER (WHERE temperature_c IS NOT NULL)   AS max_temp,
				AVG(altitude_m) FILTER (WHERE altitude_m IS NOT NULL)::real   AS avg_alt,
				MAX(altitude_m)    FILTER (WHERE altitude_m IS NOT NULL)      AS max_alt,
				MIN(altitude_m)    FILTER (WHERE altitude_m IS NOT NULL)      AS min_alt
			FROM activity_streams WHERE activity_id = $1
		) sub
		WHERE a.id = $1`,
		p.merged.ID,
	); err != nil {
		slog.Warn("fusion-merge: aggregate recompute from streams failed", "error", err)
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
// FIT-level merge (if requested), sidecar .meta write, fusion candidate cache invalidation,
// and best-effort computation for the newly created merged activity.
func (h *Handler) postCommitFusionCleanup(ctx context.Context, p fusionFinalizeParams) {
	// Rebuild track + heatmap AFTER commit — only now can db.Pool see the new
	// merged activity and its streams. EnsureActivityTrack is used because
	// RebuildActivityTrack only UPDATEs an existing row (which doesn't exist yet).
	// Lade merged activity frisch, damit has_gps/exclude_from_heatmap den
	// nach-Commit-Zustand (inkl. Fix 2 has_gps-Update) widerspiegeln.
	freshMerged, freshErr := h.DB.GetActivity(ctx, p.merged.ID)
	if freshErr != nil || freshMerged == nil {
		slog.Warn("fusion-merge: reload merged activity failed, skipping track rebuild",
			"id", p.merged.ID, "error", freshErr)
	} else if freshMerged.HasGPS {
		if err := h.DB.EnsureActivityTrack(ctx, p.merged.ID); err != nil {
			slog.Warn("fusion-merge: ensure track failed", "error", err)
		}
		if !freshMerged.ExcludeFromHeatmap {
			if err := h.DB.RebuildHeatmapData(ctx, p.merged.ID); err != nil {
				slog.Warn("fusion-merge: rebuild heatmap failed", "error", err)
			}
		}
		h.DB.InvalidateTerraIncognitaCache(ctx, p.userID)
	}

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

	// Compute best efforts for the newly merged activity (background, non-fatal).
	if h.BestEfforts != nil {
		mergedID := p.merged.ID
		svc := h.BestEfforts
		shutdown.Go("best-efforts-fusion-merge:"+mergedID, func(ctx context.Context) {
			info, err := h.DB.GetBestEffortActivityInfo(ctx, mergedID)
			if err != nil || info == nil {
				slog.Warn("fusion-merge: get best effort info failed", "id", mergedID, "error", err)
				return
			}
			stream, err := h.DB.GetStreamPoints(ctx, mergedID)
			if err != nil {
				slog.Warn("fusion-merge: get stream points for best efforts failed", "id", mergedID, "error", err)
				return
			}
			if _, err := svc.ComputeForActivity(ctx, *info, stream); err != nil {
				slog.Warn("fusion-merge: compute best efforts failed", "id", mergedID, "error", err)
			}
		})
	}

	// Re-Enrichment der merged activity (async, non-fatal).
	// RunStage1 berechnet time_of_day, timezone, sunrise/sunset, intensity etc.
	// RunGeoAndPublishStage2 enrichiert Gipfel, Orte, Oberflächen, löst Stage-2-Event aus.
	// GraphReader: h.GraphManager.Reader() wenn verfügbar, sonst nil (surface detection
	// wird dann übersprungen; alle PostGIS-basierten Geo-Module laufen normal).
	{
		mergedID := p.merged.ID
		userID := p.userID
		db := h.DB
		events := h.Events
		var gr routing.GraphReader
		if h.GraphManager != nil {
			gr = h.GraphManager.Reader()
		}
		shutdown.Go("re-enrich-fusion-merge:"+mergedID, func(ctx context.Context) {
			// Lade die merged Activity frisch innerhalb des goroutine-Kontexts, damit
			// alle Felder (has_gps, location_lat/lng, started_at, ended_at, etc.) den
			// endgültigen Post-Commit-Zustand widerspiegeln.
			act, err := db.GetActivity(ctx, mergedID)
			if err != nil || act == nil {
				slog.Warn("fusion-merge: re-enrich: reload merged activity failed", "id", mergedID, "error", err)
				return
			}
			// enrichment_status race guard: atomisch auf 'running' setzen.
			// Wenn ein paralleler Scheduler die Activity bereits gegriffen hat,
			// überspringen wir — Doppel-Enrichment würde Inkonsistenzen erzeugen.
			tag, claimErr := db.Pool.Exec(ctx,
				`UPDATE activities SET enrichment_status = 'running'
				 WHERE id = $1 AND enrichment_status = 'pending'`, mergedID)
			if claimErr != nil {
				slog.Warn("fusion-merge: re-enrich: claim enrichment_status failed", "id", mergedID, "error", claimErr)
				return
			}
			if tag.RowsAffected() == 0 {
				// Scheduler hat die Activity bereits gegriffen — kein Doppel-Enrichment.
				return
			}
			// Build ActivityData aus den geladenen Activity-Feldern.
			// Diese Felder werden von RunStage1 für Intensität, TSS, TRIMP, Timezone,
			// Weather, Sunrise/Sunset, AutoTitle etc. benötigt.
			data := &model.ActivityData{
				SportType: act.SportType,
				StartedAt: act.StartedAt,
				IsVirtual: act.IsVirtual,
				IsIndoor:  act.IsIndoor,
				HasGPS:    act.HasGPS,
			}
			if act.EndedAt != nil {
				data.EndedAt = *act.EndedAt
			}
			if act.ElapsedTimeS != nil {
				data.ElapsedTimeS = *act.ElapsedTimeS
			}
			if act.MovingTimeS != nil {
				data.MovingTimeS = *act.MovingTimeS
			}
			if act.DistanceM != nil {
				data.DistanceM = *act.DistanceM
			}
			if act.ElevationGainM != nil {
				data.ElevationGainM = *act.ElevationGainM
			}
			if act.ElevationLossM != nil {
				data.ElevationLossM = *act.ElevationLossM
			}
			if act.AvgSpeedMS != nil {
				data.AvgSpeedMS = *act.AvgSpeedMS
			}
			if act.MaxSpeedMS != nil {
				data.MaxSpeedMS = *act.MaxSpeedMS
			}
			if act.AvgHeartRate != nil {
				data.AvgHeartRate = *act.AvgHeartRate
			}
			if act.MaxHeartRate != nil {
				data.MaxHeartRate = *act.MaxHeartRate
			}
			if act.AvgPowerW != nil {
				data.AvgPowerW = *act.AvgPowerW
			}
			if act.MaxPowerW != nil {
				data.MaxPowerW = *act.MaxPowerW
			}
			if act.NormalizedPowerW != nil {
				data.NormalizedPowerW = *act.NormalizedPowerW
			}
			// GPS start/end coordinates — StartLat/Lng from track, fallback to location_lat/lng.
			if act.StartLat != nil {
				data.StartLat = *act.StartLat
			} else if act.LocationLat != nil {
				data.StartLat = *act.LocationLat
			}
			if act.StartLon != nil {
				data.StartLng = *act.StartLon
			} else if act.LocationLng != nil {
				data.StartLng = *act.LocationLng
			}
			if act.EndLat != nil {
				data.EndLat = *act.EndLat
			}
			if act.EndLon != nil {
				data.EndLng = *act.EndLon
			}
			if act.SubType != nil {
				data.SubType = *act.SubType
			}
			enrichment.RunStage1(ctx, db, userID, mergedID, data, events,
				true /* asyncWeather */, false /* skipStreamEnrichment */, gr, false /* skipGeo */)
			enrichment.RunGeoAndPublishStage2(ctx, db, userID, mergedID,
				act.HasGPS, act.SportType, act.IsVirtual, events, gr)
		})
	}
}

// fusionMergeStreamColumn kopiert eine einzelne Stream-Spalte von sourceID → tempName.
// tempName ist die TEMP TABLE (kein Hypertable), UPDATE funktioniert zuverlässig.
// s liest aus activity_streams (kann compressed sein) → nur JOIN/SELECT, kein UPDATE dort.
func fusionMergeStreamColumn(ctx context.Context, q database.Querier, tempName, sourceID, column string) error {
	query := fmt.Sprintf(`
		UPDATE %s t
		SET %s = s.%s
		FROM activity_streams s
		WHERE t.time = s.time
		  AND s.activity_id = $1
		  AND s.%s IS NOT NULL`,
		tempName, column, column, column)
	_, err := q.Exec(ctx, query, sourceID)
	return err
}

// fusionMergeGPSFromSource kopiert lat/lng von sourceID → tempName (timestamp-matched).
// tempName ist die TEMP TABLE (kein Hypertable), UPDATE funktioniert zuverlässig.
// s liest aus activity_streams (kann compressed sein) → nur JOIN/SELECT, kein UPDATE dort.
func fusionMergeGPSFromSource(ctx context.Context, q database.Querier, tempName, sourceID string) error {
	// Copy the whole GPS package (position + derived metrics) in one UPDATE.
	// Without distance_m/speed_ms/altitude_m the merged activity's stream chart has
	// no x-axis (all points collapse at distance=0) and no speed/pace curve.
	// altitude_m is included here even though it has its own FieldMap key — the
	// separate "altitude" overlay runs afterwards and overwrites this value when
	// the user picked a different source for altitude.
	_, err := q.Exec(ctx, fmt.Sprintf(`
		UPDATE %s t
		SET lat            = s.lat,
		    lng            = s.lng,
		    altitude_m     = COALESCE(s.altitude_m,     t.altitude_m),
		    distance_m     = COALESCE(s.distance_m,     t.distance_m),
		    speed_ms       = COALESCE(s.speed_ms,       t.speed_ms),
		    gps_accuracy_m = COALESCE(s.gps_accuracy_m, t.gps_accuracy_m),
		    heading        = COALESCE(s.heading,        t.heading)
		FROM activity_streams s
		WHERE t.time = s.time
		  AND s.activity_id = $1
		  AND s.lat IS NOT NULL
		  AND s.lng IS NOT NULL`, tempName),
		sourceID)
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
			NULL::bigint, NULL::bigint, NULL::bigint,
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
	// recording_device_id and original_filename are also cleared: the merged
	// activity wasn't recorded by a single device, so frontends that derive a
	// device label from these fields would otherwise mis-label it as the
	// metadata source's device (e.g. "Garmin Instinct 2X") instead of "Fusion".
	if _, err := q.Exec(ctx, `UPDATE activities SET
		garmin_activity_id = NULL,
		strava_activity_id = NULL,
		komoot_tour_id = NULL,
		source = 'fusion',
		source_device = NULL,
		recording_device_id = NULL,
		original_filename = NULL,
		original_file_format = NULL
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
	TagsSource      *string           `json:"tags_source,omitempty"`
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
	if len(req.FieldMap) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "field_map must not be empty", r.URL.Path)
		return
	}
	if req.TagsMerge && req.TagsSource != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "tags_merge and tags_source are mutually exclusive", r.URL.Path)
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

	// tagsSource / titleSource / gearSource must reference one of the authorized sources.
	// Without this, an attacker could pass another user's activity UUID and leak data.
	if req.TagsSource != nil && !slices.Contains(req.Sources, *req.TagsSource) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "tags_source not in sources list", r.URL.Path)
		return
	}
	if req.TitleSource != nil && !slices.Contains(req.Sources, *req.TitleSource) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "title_source not in sources list", r.URL.Path)
		return
	}
	if req.GearSource != nil && !slices.Contains(req.Sources, *req.GearSource) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "gear_source not in sources list", r.URL.Path)
		return
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

	// Serialise concurrent Merge/Revert for the same user via an advisory lock
	// that is scoped to the transaction (released automatically on Commit/Rollback).
	lockKeyV2 := dbutil.AdvisoryLockKey("fusion:" + userID)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockKeyV2); err != nil {
		slog.Error("fusion-merge-v2: advisory lock failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "fusion advisory lock: "+err.Error(), r.URL.Path)
		return
	}

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
	//    all source activities. We use req.Sources (alle vom User deklarierten Quellen)
	//    statt nur fieldMap-Werte — sonst fehlen Timestamps von Sources die keinem Feld
	//    explizit zugewiesen sind (z.B. bei 3-Quellen-Fusion mit nur 2 field_map-Einträgen).
	sourceSet := make(map[string]struct{}, len(req.Sources))
	for _, id := range req.Sources {
		sourceSet[id] = struct{}{}
	}

	uniqueSources := make([]string, 0, len(sourceSet))
	for id := range sourceSet {
		uniqueSources = append(uniqueSources, id)
	}

	// 2b. TEMP TABLE erstellen + Unified Timeline hineinfügen.
	// Alle Overlays werden auf der TEMP TABLE gemacht (kein Hypertable → kein Compression-Problem).
	// finalizeStreamsMerge schreibt danach in activity_streams.
	tempName, err := createTempMergedStreams(ctx, tx)
	if err != nil {
		slog.Error("fusion-merge-v2: create temp table failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to create temp table", r.URL.Path)
		return
	}

	if err := insertUnifiedTimelineInto(ctx, tx, tempName, merged.ID, uniqueSources); err != nil {
		slog.Error("fusion-merge-v2: build unified timeline failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to build unified timeline", r.URL.Path)
		return
	}

	// 3. For each field in FieldMap, overlay data from the designated source (on TEMP TABLE).
	for field, sourceID := range req.FieldMap {
		if field == "gps" {
			if sourceID == "fuse" {
				if err := h.fuseGPSTracks(ctx, tx, tempName, merged.ID, req.Sources, req.GPSFusionConfig); err != nil {
					httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", err.Error(), r.URL.Path)
					return
				}
			} else {
				if err := fusionMergeGPSFromSource(ctx, tx, tempName, sourceID); err != nil {
					slog.Error("fusion-merge-v2: GPS merge failed", "error", err)
					httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "GPS merge failed", r.URL.Path)
					return
				}
			}
			continue
		}

		col := validStreamFields[field]
		if err := fusionMergeStreamColumn(ctx, tx, tempName, sourceID, col); err != nil {
			slog.Error("fusion-merge-v2: stream column merge failed", "field", field, "col", col, "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", fmt.Sprintf("merge field %s failed", field), r.URL.Path)
			return
		}
	}

	// 3b. TEMP TABLE → activity_streams finalisieren.
	if err := finalizeStreamsMerge(ctx, tx, tempName); err != nil {
		slog.Error("fusion-merge-v2: finalize streams merge failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to finalize streams", r.URL.Path)
		return
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
	// Skip redundant title/tags/gear UPDATEs when the user-designated source equals the
	// metadata source — the INSERT-SELECT in createMergedActivity already copied
	// those fields from there. Same pattern as V1's targetID guard.
	titleSourceV2 := req.TitleSource
	if titleSourceV2 != nil && *titleSourceV2 == metadataSource.ID {
		titleSourceV2 = nil
	}
	tagsSourceV2 := req.TagsSource
	if tagsSourceV2 != nil && *tagsSourceV2 == metadataSource.ID {
		tagsSourceV2 = nil
	}
	gearSourceV2 := req.GearSource
	if gearSourceV2 != nil && *gearSourceV2 == metadataSource.ID {
		gearSourceV2 = nil
	}
	fp := fusionFinalizeParams{
		userID:            userID,
		merged:            merged,
		allSourceIDs:      req.Sources,
		activities:        activities,
		fieldMap:          req.FieldMap,
		titleSource:       titleSourceV2,
		tagsMerge:         req.TagsMerge,
		tagsSource:        tagsSourceV2,
		gearSource:        gearSourceV2,
		metadataSourceID:  metadataSource.ID,
		fitLevel:          req.FitLevel,
		fitLevelTargetID:  fitLevelTargetID,
		fitLevelSourceIDs: fitLevelSourceIDs,
		auditData:         auditData,
	}
	h.applyFusionTxExtras(ctx, tx, fp)

	// 8. Mark all sources as fusion sources (so they are excluded from the
	// fusion-candidate list for future merges) and, in legacy mode, hide them
	// via parent_id. In group mode sources stay visible but still must be flagged.
	legacyMode := req.GroupID == nil || *req.GroupID == ""
	for _, id := range req.Sources {
		if legacyMode {
			if _, err := tx.Exec(ctx,
				`UPDATE activities SET parent_id = $2, is_fusion_source = TRUE WHERE id = $1 AND user_id = $3`,
				id, merged.ID, userID,
			); err != nil {
				slog.Warn("fusion-merge-v2: hide source + flag fusion source failed", "id", id, "error", err)
			}
		} else {
			if _, err := tx.Exec(ctx,
				`UPDATE activities SET is_fusion_source = TRUE WHERE id = $1 AND user_id = $2`,
				id, userID,
			); err != nil {
				slog.Warn("fusion-merge-v2: flag fusion source failed", "id", id, "error", err)
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

	// 9b. Dual-write: insert into activity_fusion_memberships (new normalised table).
	membershipsV2 := deriveFusionMemberships(merged.ID, userID, req.FieldMap, req.Sources)
	if err := database.InsertFusionMemberships(ctx, tx, membershipsV2); err != nil {
		slog.Error("fusion-merge-v2: insert memberships failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "insert memberships (v2): "+err.Error(), r.URL.Path)
		return
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
		} else {
			// Merged activity als Primary der Gruppe setzen; der Trigger
			// trg_group_primary_sync setzt is_group_secondary für die anderen Members.
			if err := h.DB.SetRecordingGroupPrimary(ctx, *req.GroupID, merged.ID); err != nil {
				slog.Warn("fusion-merge-v2: set group primary failed", "group", *req.GroupID, "error", err)
			}
		}
	}

	// Post-commit: FIT-level merge, sidecar write, candidate cache cleanup.
	h.postCommitFusionCleanup(ctx, fp)

	slog.Info("fusion-merge-v2: completed", "new_id", merged.ID, "sources", req.Sources)
	httputil.WriteJSON(w, http.StatusCreated, map[string]string{"id": merged.ID})
}

// createTempMergedStreams erstellt eine TEMP TABLE mit identischer Struktur zu
// activity_streams plus einem UNIQUE INDEX auf (time). Da TEMP-Tables keine
// Hypertable-Compression kennen, funktionieren UPDATE und ON CONFLICT darauf
// zuverlässig — auch wenn die Quell-Daten in compressed TimescaleDB-Chunks liegen.
// ON COMMIT DROP: Die Tabelle wird automatisch am Transaktions-Ende gelöscht.
func createTempMergedStreams(ctx context.Context, q database.Querier) (string, error) {
	tempName := fmt.Sprintf("merged_streams_tmp_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	// INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING GENERATED: erbt DEFAULT-Werte,
	// CHECK-Constraints und GENERATED-Spalten. FK-Constraints werden via LIKE nie vererbt
	// (PostgreSQL-Semantik), daher ist das für TEMP-Tables safe. INCLUDING ALL würde
	// zusätzlich Storage-Parameter und Statistiken vererben — nicht sinnvoll für TEMP.
	if _, err := q.Exec(ctx, fmt.Sprintf(
		`CREATE TEMP TABLE %s (LIKE activity_streams INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING GENERATED) ON COMMIT DROP`, tempName,
	)); err != nil {
		return "", fmt.Errorf("create temp table: %w", err)
	}
	// (activity_id, time) statt nur (time): semantisch korrekt, da activity_id
	// in der TEMP TABLE immer mergedID ist, aber verhindert stille Fehler wenn
	// zwei Quellen denselben Timestamp haben.
	if _, err := q.Exec(ctx, fmt.Sprintf(
		`CREATE UNIQUE INDEX ON %s (activity_id, time)`, tempName,
	)); err != nil {
		return "", fmt.Errorf("create unique index on temp table: %w", err)
	}
	return tempName, nil
}

// insertUnifiedTimelineInto fügt UNION-Timestamps aller sourceIDs in die TEMP TABLE ein.
// activity_id wird auf mergedID gesetzt, alle anderen Felder bleiben NULL.
func insertUnifiedTimelineInto(ctx context.Context, q database.Querier, tempName, mergedID string, sourceIDs []string) error {
	if len(sourceIDs) == 0 {
		return nil
	}

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
		INSERT INTO %s (activity_id, time)
		SELECT $1, t.time FROM (
			%s
		) t
		GROUP BY t.time
		ORDER BY t.time ASC`,
		tempName, strings.Join(unions, " UNION ALL "))

	_, err := q.Exec(ctx, query, args...)
	return err
}

// copyTargetStreamsInto kopiert alle Felder der Ziel-Aktivität in die TEMP TABLE.
// Wird für V1 (FusionMerge) als initialer Copy verwendet, bevor Overlays angewendet werden.
//
// Implementiert als zwei Statements ohne Spaltenliste, damit zukünftige
// activity_streams-Erweiterungen (ADD COLUMN) automatisch mitgehen — TEMP wurde
// via `LIKE activity_streams` strukturidentisch erstellt, daher ist `SELECT *`
// safe. Das anschließende UPDATE setzt activity_id auf mergedID um.
func copyTargetStreamsInto(ctx context.Context, q database.Querier, tempName, mergedID, targetID string) error {
	if _, err := q.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s SELECT * FROM activity_streams WHERE activity_id = $1 ORDER BY time ASC`,
		tempName,
	), targetID); err != nil {
		return fmt.Errorf("copy target streams: %w", err)
	}
	if _, err := q.Exec(ctx, fmt.Sprintf(
		`UPDATE %s SET activity_id = $1`, tempName,
	), mergedID); err != nil {
		return fmt.Errorf("rewrite activity_id in temp: %w", err)
	}
	return nil
}

// finalizeStreamsMerge kopiert alle Rows aus der TEMP TABLE in activity_streams.
// activity_id ist bereits korrekt gesetzt (mergedID), kein Override nötig.
func finalizeStreamsMerge(ctx context.Context, q database.Querier, tempName string) error {
	_, err := q.Exec(ctx, fmt.Sprintf(
		`INSERT INTO activity_streams SELECT * FROM %s ORDER BY time ASC`, tempName,
	))
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

	// Collect source IDs via zwei Wege:
	// 1. parent_id-Query (V1-Sources haben parent_id = id)
	// 2. FusionSources-JSON auf der Activity (V2-Sources haben parent_id=NULL)
	seenIDs := make(map[string]struct{})
	var sourceIDs []string

	// Weg 1: parent_id (V1)
	if rows, qErr := h.DB.Pool.Query(ctx,
		`SELECT id FROM activities WHERE parent_id = $1 AND user_id = $2`, id, userID,
	); qErr == nil {
		for rows.Next() {
			var sid string
			if err := rows.Scan(&sid); err == nil {
				if _, dup := seenIDs[sid]; !dup {
					seenIDs[sid] = struct{}{}
					sourceIDs = append(sourceIDs, sid)
				}
			}
		}
		rows.Close()
	}

	// Weg 2: FusionSources-JSON (V2 — sources haben keine parent_id)
	if activity.FusionSources != nil {
		var fusionSrcMap database.FusionSources
		if jsonErr := json.Unmarshal(activity.FusionSources, &fusionSrcMap); jsonErr == nil {
			for _, srcID := range fusionSrcMap {
				if srcID == "fuse" {
					continue
				}
				if _, dup := seenIDs[srcID]; !dup {
					seenIDs[srcID] = struct{}{}
					sourceIDs = append(sourceIDs, srcID)
				}
			}
		}
	}

	// ── Atomic state-flip: legacy flags + memberships ────────────────────────
	// Begin a transaction that resets both the legacy world (is_fusion_source /
	// parent_id) and the memberships world (activity_fusion_memberships) in one
	// atomic commit.  Advisory lock serialises concurrent Merge/Revert per user.
	flipTx, err := h.DB.Pool.Begin(ctx)
	if err != nil {
		slog.Error("revert-fusion: begin tx failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to begin revert tx", r.URL.Path)
		return
	}
	defer flipTx.Rollback(ctx) //nolint:errcheck — no-op if committed

	lockKey := dbutil.AdvisoryLockKey("fusion:" + userID)
	if _, err := flipTx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockKey); err != nil {
		slog.Error("revert-fusion: advisory lock failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "lock failed", r.URL.Path)
		return
	}

	// (a) V2-Sources zurücksetzen: is_fusion_source = FALSE; parent_id war nie gesetzt.
	// NOT EXISTS: wenn eine Source auch in einer ANDEREN Fusion (M2 != id) referenziert ist,
	// darf is_fusion_source nicht auf FALSE gesetzt werden — sie bleibt gebackt in M2.
	// NOTE: cross-fusion guard still uses jsonb_each_text(fusion_sources) — intentional
	// for Phase 1 (Dual-Write); rewritten to memberships in Phase 2a (Task 14).
	if len(sourceIDs) > 0 {
		if _, err := flipTx.Exec(ctx,
			`UPDATE activities SET is_fusion_source = FALSE, parent_id = NULL
			 WHERE id = ANY($1) AND user_id = $2
			   AND NOT EXISTS (
			       SELECT 1 FROM activities m
			       WHERE m.user_id = $2
			         AND m.id != $3
			         AND EXISTS (
			             SELECT 1 FROM jsonb_each_text(m.fusion_sources) kv
			             WHERE kv.value = activities.id::text
			         )
			   )`,
			sourceIDs, userID, id,
		); err != nil {
			slog.Error("revert-fusion: reset V2 flags failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "reset V2 failed", r.URL.Path)
			return
		}
	}

	// (b) Alle versteckten Quellen (parent_id = id) wiederherstellen + is_fusion_source zurücksetzen.
	// Für Group-Fusions (V2): parent_id war nie gesetzt, daher ist das ein No-Op für parent_id,
	// aber is_fusion_source = FALSE ist in jedem Fall korrekt (Legacy-Bugfix).
	// CASCADE entfernt die Fusion-Aktivität aus recording_group_members; der DEFERRED-Trigger
	// berechnet is_group_secondary + primary_activity_id neu.
	if _, err := flipTx.Exec(ctx,
		`UPDATE activities SET parent_id = NULL, is_fusion_source = FALSE WHERE parent_id = $1 AND user_id = $2`,
		id, userID,
	); err != nil {
		slog.Error("revert-fusion: restore V1 sources failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "restore V1 failed", r.URL.Path)
		return
	}

	// (c) Memberships cleanup — must happen before DeleteActivity so that tests
	// and Exit-Gate SQL reading memberships as source-of-truth see a consistent
	// state during Phase 1 (Dual-Write).
	if err := database.DeleteFusionMembershipsForMerged(ctx, flipTx, id); err != nil {
		slog.Error("revert-fusion: delete memberships failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "delete memberships failed", r.URL.Path)
		return
	}

	// Commit — legacy world + memberships world flip atomically.
	// Post-commit work (cleanupActivityFiles, DELETE streams, DeleteActivity,
	// candidate + cache invalidation, best-efforts recompute) runs outside the tx.
	// If the process crashes after this Commit but before DeleteActivity the merged
	// activity is orphaned — the Exit-Gate SQL (Task 8) detects this; recovery is manual.
	if err := flipTx.Commit(ctx); err != nil {
		slog.Error("revert-fusion: commit flip tx failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "commit failed", r.URL.Path)
		return
	}

	// Clean up ShadowFS files before deleting the fusion result.
	h.cleanupActivityFiles(ctx, userID, id)

	// Streams explizit löschen bevor die Aktivität gelöscht wird.
	// TimescaleDB compressed chunks (nach >7 Tagen) unterstützen DELETE ab v2.11 —
	// explizites DELETE ist zuverlässiger als auf CASCADE vom Hypertable zu warten.
	// Fehler sind nicht-fatal: die Activity-Löschung wird trotzdem versucht.
	if _, err := h.DB.Pool.Exec(ctx, `DELETE FROM activity_streams WHERE activity_id = $1`, id); err != nil {
		slog.Warn("revert-fusion: delete streams failed (continuing with activity delete)", "error", err)
	}

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

	// Recompute best efforts for each restored source activity (background, non-fatal).
	if h.BestEfforts != nil {
		for _, sid := range sourceIDs {
			actID := sid
			svc := h.BestEfforts
			shutdown.Go("best-efforts-fusion-restore:"+actID, func(ctx context.Context) {
				info, err := h.DB.GetBestEffortActivityInfo(ctx, actID)
				if err != nil || info == nil {
					slog.Warn("revert-fusion: get best effort info failed", "id", actID, "error", err)
					return
				}
				stream, err := h.DB.GetStreamPoints(ctx, actID)
				if err != nil {
					slog.Warn("revert-fusion: get stream points failed", "id", actID, "error", err)
					return
				}
				if _, err := svc.ComputeForActivity(ctx, *info, stream); err != nil {
					slog.Warn("revert-fusion: compute best efforts failed", "id", actID, "error", err)
				}
			})
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
