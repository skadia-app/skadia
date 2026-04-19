package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
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

// ErrAlreadyFused is returned when a FusionMerge attempt includes a source
// activity that is already a member of another merged activity. The preflight
// check inside each merge tx (after advisory-lock acquisition) rejects this
// case, which protects against concurrent merges of the same [A, B] set.
var ErrAlreadyFused = errors.New("one or more sources already belong to a fusion")

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

	// Preflight: after holding the lock, re-check that no source is already fused.
	// This closes the race window between two parallel FusionMerge calls for the same
	// user: the first commits its memberships, the second (after lock-release wait)
	// sees them and returns 409 Conflict.
	var alreadyFusedV1 bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM activity_fusion_memberships WHERE source_id = ANY($1))`,
		allIDs,
	).Scan(&alreadyFusedV1); err != nil {
		slog.Error("fusion-merge: preflight query failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "preflight failed", r.URL.Path)
		return
	}
	if alreadyFusedV1 {
		httputil.WriteError(w, http.StatusConflict, "Conflict", ErrAlreadyFused.Error(), r.URL.Path)
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
	if err := h.applyFusionTxExtras(ctx, tx, fp); err != nil {
		slog.Error("fusion-merge: applyFusionTxExtras failed, rolling back", "merged_id", merged.ID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "merge finalization failed: "+err.Error(), r.URL.Path)
		return
	}

	// 8. Alle Quellen (inkl. Target) via parent_id verstecken — einzelnes atomares UPDATE.
	sourcesToHide := make([]string, 0, len(allIDs))
	for _, id := range allIDs {
		if id != merged.ID {
			sourcesToHide = append(sourcesToHide, id)
		}
	}
	if len(sourcesToHide) > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE activities SET parent_id = $1 WHERE id = ANY($2) AND user_id = $3`,
			merged.ID, sourcesToHide, userID,
		); err != nil {
			slog.Error("fusion-merge: hide sources failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "hide sources failed: "+err.Error(), r.URL.Path)
			return
		}
	}

	// 8b. Insert into activity_fusion_memberships — the sole source-of-truth for fusion state
	// since Phase 3. parent_id (step 8) is a UI-visibility hint only; memberships own the data.
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
// title update (step 4), gear copy (step 5), tags merge (step 6),
// RebuildActivityTrack + RebuildHeatmapData + InvalidateTerraIncognitaCache (pre-commit rebuilds),
// and audit log insert.
// Must be called before tx.Commit. Does not commit.
// Returns an error if any fatal step fails (e.g. related-tables union-copy). The caller
// must roll back the transaction on non-nil return.
func (h *Handler) applyFusionTxExtras(ctx context.Context, tx database.Querier, p fusionFinalizeParams) error {
	// Step 4: Title from designated source.
	// Non-fatal: fusion field_map populates the title AFTER merge in many flows,
	// so a failed title-copy is recoverable (the merged activity gets a fallback title).
	// TODO: reconsider if/when the title-source path becomes the primary write path.
	if p.titleSource != nil {
		if src, ok := p.activities[*p.titleSource]; ok && src.Title != nil {
			if _, err := tx.Exec(ctx,
				`UPDATE activities SET title = $2, has_edits = true WHERE id = $1`,
				p.merged.ID, *src.Title,
			); err != nil {
				slog.Warn("fusion-merge: update title failed",
					"action", "fusion_merge_title_copy_nonfatal",
					"merged_id", p.merged.ID,
					"error", err)
			}
		}
	}

	// Step 5: Gear from designated source.
	// Non-fatal: gear is user-visible but manually reassignable after merge.
	if p.gearSource != nil {
		if src, ok := p.activities[*p.gearSource]; ok {
			if src.GearID != nil {
				h.copyGearIfPresent(ctx, tx, p.merged.ID, src.ID, src.GearID)
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
				slog.Warn("fusion-merge: merge tags failed",
					"action", "fusion_merge_tags_union_nonfatal",
					"source", srcID, "error", err)
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
			slog.Warn("fusion-merge: recompute tags.activity_count after tagsMerge failed",
				"action", "fusion_merge_tag_count_recompute_nonfatal",
				"merged_id", p.merged.ID, "error", err)
		}
	} else if p.tagsSource != nil {
		// Keep activities.tags TEXT[] AND activity_tags junction in sync, otherwise
		// tag-filter queries (which go through the junction) will not find this activity.
		if _, err := tx.Exec(ctx,
			`UPDATE activities t SET tags = s.tags, has_edits = true
			 FROM activities s WHERE t.id = $1 AND s.id = $2`,
			p.merged.ID, *p.tagsSource,
		); err != nil {
			slog.Warn("fusion-merge: set tags TEXT[] from source failed",
				"action", "fusion_merge_tags_text_array_nonfatal",
				"source", *p.tagsSource, "error", err)
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
				slog.Warn("fusion-merge: copy activity_tags junction from source failed",
					"action", "fusion_merge_tags_junction_nonfatal",
					"source", *p.tagsSource, "error", err)
			}
			// Increment tags.activity_count for each tag now linked to the merged activity.
			if _, err := tx.Exec(ctx,
				`UPDATE tags SET activity_count = (
				    SELECT COUNT(DISTINCT activity_id) FROM activity_tags WHERE tag_id = tags.id
				 ) WHERE id IN (SELECT tag_id FROM activity_tags WHERE activity_id = $1)`,
				p.merged.ID,
			); err != nil {
				slog.Warn("fusion-merge: recompute tags.activity_count failed",
					"action", "fusion_merge_tag_count_recompute_nonfatal",
					"merged_id", p.merged.ID, "error", err)
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
			slog.Warn("fusion-merge: copy default tags junction failed",
				"action", "fusion_merge_tags_junction_nonfatal",
				"merged_id", p.merged.ID, "error", err)
		} else {
			// activity_count neu berechnen: Tag war bisher an metadata-Source → count +1 (merged).
			if _, err := tx.Exec(ctx,
				`UPDATE tags SET activity_count = (
				    SELECT COUNT(DISTINCT activity_id) FROM activity_tags WHERE tag_id = tags.id
				 ) WHERE id IN (SELECT tag_id FROM activity_tags WHERE activity_id = $1)`,
				p.merged.ID,
			); err != nil {
				slog.Warn("fusion-merge: recompute default tags.activity_count failed",
					"action", "fusion_merge_tag_count_recompute_nonfatal",
					"merged_id", p.merged.ID, "error", err)
			}
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
		slog.Warn("fusion-merge: has_gps recompute from streams failed",
			"action", "fusion_merge_has_gps_recompute_nonfatal",
			"merged_id", p.merged.ID, "error", err)
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
		slog.Warn("fusion-merge: aggregate recompute from streams failed",
			"action", "fusion_merge_aggregate_recompute_nonfatal",
			"merged_id", p.merged.ID, "error", err)
	}

	// Step 7c: Phase 4b — union-copy related tables (photos, geo_tags, privacy_sections,
	// tag_removals, developer_apps, best_effort_exclusions) from all sources into merged.
	// Must run AFTER the merged activity row exists and streams are written so FK constraints hold.
	// Runs inside the same TX to keep atomicity.
	// FATAL: if any sub-operation fails (e.g. photos reassigned but geo_tags failed), the
	// TX must be rolled back — partial related-table state is worse than none at all.
	if err := copyRelatedTablesUnion(ctx, tx, p.merged.ID, p.allSourceIDs, p.metadataSourceID, p.userID); err != nil {
		return fmt.Errorf("related-tables union-copy: %w", err)
	}

	// TODO(phase-5): extract a per-activity segment_efforts recomputer and call it
	// here. See docs/internal/2026-04-16-fusion-segment-efforts-recompute.md.
	// Currently segment-matches land on next user-triggered segment recompute.

	// Audit log (inside transaction).
	// FATAL: the audit record is required for GDPR accountability. A failed INSERT
	// here would commit the merge without any audit trail, breaking the export/import
	// round-trip (postCommitFusionCleanup reads the latest audit record to populate
	// the sidecar). Roll back the entire merge rather than silently lose the record.
	if _, err := tx.Exec(ctx,
		`INSERT INTO activity_edits (activity_id, user_id, edit_type, edit_data)
		 VALUES ($1, $2, 'fusion_merge', $3)`,
		p.merged.ID, p.userID, p.auditData,
	); err != nil {
		return fmt.Errorf("audit record insert: %w", err)
	}

	return nil
}

// postCommitFusionCleanup runs the shared post-commit steps:
// FIT-level merge (if requested), sidecar .meta write, fusion candidate cache invalidation,
// and best-effort computation for the newly created merged activity.
func (h *Handler) postCommitFusionCleanup(ctx context.Context, p fusionFinalizeParams) {
	// Rebuild track + heatmap AFTER commit — only now can db.Pool see the new
	// merged activity and its streams. EnsureActivityTrack is used because
	// RebuildActivityTrack only UPDATEs an existing row (which doesn't exist yet).
	// Load merged activity fresh so has_gps/exclude_from_heatmap reflect the post-commit
	// state. freshMerged is reused by the sidecar block below.
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
			// fitLevelMerge writes a new file_hash + file_id to the activities row.
			// Reload freshMerged so the sidecar block below uses the updated file_id —
			// without this, sidecar.WriteMetaCompanionFor would receive the stale/nil
			// file_id that was set before the FIT-level merge ran.
			if reloaded, reloadErr := h.DB.GetActivity(ctx, p.merged.ID); reloadErr == nil && reloaded != nil {
				freshMerged = reloaded
			} else {
				slog.Warn("fusion-merge: reload merged activity after fitLevelMerge failed, sidecar may use stale file_id",
					"id", p.merged.ID, "error", reloadErr)
			}
		}
	}

	// Write .meta companion for merged activity (ShadowFS).
	// Uses GenerateMetaFusion to embed fusion lineage into the sidecar so that
	// GDPR export/import can reconstruct memberships from the .meta file alone.
	// freshMerged is reloaded above when fitLevelMerge ran, so file_id is current.
	{
		if mergedAct := freshMerged; mergedAct != nil {
			memberships, _ := h.DB.GetFusionMembershipsForMerged(ctx, p.merged.ID)
			// Batch-load titles for all source IDs in one query.
			// Guard: skip the DB call if there are no memberships (avoids a no-op roundtrip).
			sourceIDs := make([]string, len(memberships))
			for i, m := range memberships {
				sourceIDs[i] = m.SourceID
			}
			var titles map[string]string
			if len(sourceIDs) > 0 {
				var titlesErr error
				titles, titlesErr = h.DB.GetActivityTitles(ctx, sourceIDs)
				if titlesErr != nil {
					// GetActivityTitles returns nil map on error (no partial results).
					// Fall back to empty titles so the sidecar is still written.
					slog.Warn("post-commit: batch titles failed — sidecar will omit titles",
						"action", "sidecar_titles_partial",
						"merged_id", p.merged.ID,
						"error", titlesErr)
					titles = map[string]string{}
				}
			} else {
				titles = map[string]string{}
			}
			snapshots := make([]sidecar.FusionMemberSnapshot, len(memberships))
			for i, m := range memberships {
				snapshots[i] = sidecar.FusionMemberSnapshot{
					SourceID:    m.SourceID,
					SourceTitle: titles[m.SourceID], // "" if missing — not fatal
					FieldKey:    m.FieldKey,
				}
			}
			editData, _ := h.DB.GetLatestEditData(ctx, p.merged.ID, "fusion_merge")
			meta := sidecar.GenerateMetaFusion(p.merged.ID, mergedAct, snapshots, editData)
			sidecar.WriteMetaCompanionFor(ctx, h.ShadowFS, h.DB, p.userID, p.merged.ID, mergedAct.FileID, meta)
		}
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

// copyGearIfPresent copies the gear from a source activity to the merged activity.
// Non-fatal: gear is user-visible but can be reassigned manually after merge.
// The guard on gearID prevents an unnecessary UPDATE when the source has no gear set.
func (h *Handler) copyGearIfPresent(ctx context.Context, tx database.Querier, mergedID, srcID string, gearID *string) {
	if gearID == nil || *gearID == "" {
		return
	}
	if _, err := tx.Exec(ctx,
		`UPDATE activities SET gear_id = $2, has_edits = true WHERE id = $1`,
		mergedID, *gearID,
	); err != nil {
		slog.Warn("fusion-merge: gear copy failed",
			"action", "fusion_merge_gear_copy_nonfatal",
			"merged_id", mergedID,
			"src", srcID,
			"error", err)
	}
}

// ─── Phase 4b: Related-Tables Union-Copy ──────────────────────────────────────

// allowedRelatedTables is the exhaustive set of table names that may be used as SQL
// identifiers in copyRelatedTableSingle and copyRelatedTableUnion. Both functions
// format tableName into a TEMP TABLE DDL via fmt.Sprintf; a closed allow-list provides
// defense-in-depth against accidental SQL injection if the call path ever changes to
// accept user-supplied table names.
var allowedRelatedTables = map[string]struct{}{
	"activity_photos":           {},
	"activity_geo_tags":         {},
	"activity_privacy_sections": {},
	"activity_tag_removals":     {},
	"activity_developer_apps":   {},
	"best_effort_exclusions":    {},
}

// copyRelatedTablesUnion union-copies all Phase 4b related tables from source activities
// into the merged activity inside the same transaction. Must be called while the merged
// activity row already exists (FK constraint on activity_id).
//
// Table strategies:
//   - activity_photos            → reassign (UPDATE activity_id to mergedID); see note below
//   - activity_geo_tags          → union, dedup by (activity_id, osm_id) WHERE osm_id IS NOT NULL
//   - activity_privacy_sections  → union, no dedup (duplicates harmless for privacy masking)
//   - activity_tag_removals      → union, dedup by PK (activity_id, tag_id)
//   - activity_developer_apps    → metadata-source-wins, then union fallback
//   - best_effort_exclusions     → metadata-source-wins only (user preference, not union)
//
// Photos strategy note: activity_photos has a user-level unique constraint on
// (user_id, file_hash) WHERE file_hash IS NOT NULL. This prevents INSERT-based
// copying because the source rows (with the same user_id + file_hash) already
// occupy the unique slot. We therefore reassign the existing photo rows to the merged
// activity via UPDATE instead of copying. Sources are hidden (parent_id) after merge,
// so the photos remain accessible through the merged activity.
func copyRelatedTablesUnion(ctx context.Context, tx database.Querier, mergedID string, sources []string, metadataSourceID string, userID string) error {
	// Defensive: mergedID is used in SQL identifier construction (TEMP TABLE names).
	// Validate it is a well-formed UUID before any formatting to prevent SQL injection
	// if the call path ever changes to accept external input.
	if _, err := uuid.Parse(mergedID); err != nil {
		return fmt.Errorf("copyRelatedTablesUnion: mergedID not a valid UUID: %w", err)
	}

	// Photos: reassign (UPDATE activity_id) rather than copy — see strategy note above.
	// COALESCE preserves the original source_activity_id if a photo was already reassigned
	// during a previous fusion (transitive). RevertFusion uses source_activity_id to restore
	// activity_id before DeleteActivity triggers CASCADE — preventing data loss.
	// user_id filter: defense-in-depth — only reassign photos owned by the same user,
	// preventing cross-user data bleed if source IDs are ever supplied from untrusted input.
	if _, err := tx.Exec(ctx,
		`UPDATE activity_photos
		 SET activity_id = $1,
		     source_activity_id = COALESCE(source_activity_id, activity_id)
		 WHERE activity_id = ANY($2) AND user_id = $3`,
		mergedID, sources, userID,
	); err != nil {
		return fmt.Errorf("activity_photos reassign: %w", err)
	}
	if err := copyRelatedTableUnion(ctx, tx, mergedID, sources, "activity_geo_tags",
		`ON CONFLICT (activity_id, osm_id) WHERE osm_id IS NOT NULL DO NOTHING`); err != nil {
		return fmt.Errorf("activity_geo_tags: %w", err)
	}
	if err := copyRelatedTableUnion(ctx, tx, mergedID, sources, "activity_privacy_sections",
		``); err != nil {
		return fmt.Errorf("activity_privacy_sections: %w", err)
	}
	if err := copyRelatedTableUnion(ctx, tx, mergedID, sources, "activity_tag_removals",
		`ON CONFLICT (activity_id, tag_id) DO NOTHING`); err != nil {
		return fmt.Errorf("activity_tag_removals: %w", err)
	}

	// Metadata-source-wins tables: seed from metadataSource first, then union remainder.
	// The first INSERT claims all rows that might conflict; subsequent union INSERT DO NOTHINGs.
	if err := copyRelatedTableSingle(ctx, tx, mergedID, metadataSourceID, "activity_developer_apps",
		`ON CONFLICT (activity_id, application_id) DO NOTHING`); err != nil {
		return fmt.Errorf("activity_developer_apps (metadata-source seed): %w", err)
	}
	if err := copyRelatedTableUnion(ctx, tx, mergedID, sources, "activity_developer_apps",
		`ON CONFLICT (activity_id, application_id) DO NOTHING`); err != nil {
		return fmt.Errorf("activity_developer_apps (union fallback): %w", err)
	}

	// best_effort_exclusions: metadata-source-wins only — user explicitly excluded an effort
	// from that source; don't blindly import exclusions from other sources.
	if err := copyRelatedTableSingle(ctx, tx, mergedID, metadataSourceID, "best_effort_exclusions",
		`ON CONFLICT DO NOTHING`); err != nil {
		return fmt.Errorf("best_effort_exclusions: %w", err)
	}

	return nil
}

// copyRelatedTableUnion copies rows from all source activities into mergedID using a
// TEMP TABLE as staging area. The temp table is session-scoped and dropped ON COMMIT.
// The UUID suffix derived from mergedID ensures no collision across parallel merge TXs.
//
// Steps:
//  1. CREATE TEMP TABLE (LIKE source INCLUDING ...) — schema-drift safe.
//  2. INSERT INTO temp from all sources (WHERE activity_id = ANY($sources)).
//  3. UPDATE temp SET activity_id = mergedID, id = gen_random_uuid() (when table has own UUID PK).
//  4. INSERT INTO target from temp with the provided ON CONFLICT clause.
func copyRelatedTableUnion(ctx context.Context, tx database.Querier, mergedID string, sources []string, tableName string, onConflictClause string) error {
	if _, ok := allowedRelatedTables[tableName]; !ok {
		return fmt.Errorf("copyRelatedTableUnion: table %q not in allow-list", tableName)
	}
	tempName := fmt.Sprintf("tmp_merge_%s_%s", tableName, strings.ReplaceAll(mergedID, "-", ""))

	if _, err := tx.Exec(ctx, fmt.Sprintf(
		`CREATE TEMP TABLE %s (LIKE %s INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING GENERATED) ON COMMIT DROP`,
		tempName, tableName,
	)); err != nil {
		return fmt.Errorf("create temp %s: %w", tempName, err)
	}

	if _, err := tx.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s SELECT * FROM %s WHERE activity_id = ANY($1)`,
		tempName, tableName,
	), sources); err != nil {
		return fmt.Errorf("copy sources to %s: %w", tempName, err)
	}

	if relatedTableHasOwnUUIDPK(tableName) {
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			`UPDATE %s SET activity_id = $1, id = gen_random_uuid()`, tempName),
			mergedID); err != nil {
			return fmt.Errorf("rewrite activity_id+id on %s: %w", tempName, err)
		}
	} else {
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			`UPDATE %s SET activity_id = $1`, tempName),
			mergedID); err != nil {
			return fmt.Errorf("rewrite activity_id on %s: %w", tempName, err)
		}
	}

	if _, err := tx.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s SELECT * FROM %s %s`, tableName, tempName, onConflictClause,
	)); err != nil {
		return fmt.Errorf("insert %s from temp: %w", tableName, err)
	}
	return nil
}

// copyRelatedTableSingle is the single-source variant of copyRelatedTableUnion.
// Used for metadata-source-wins tables where only one source's rows should seed the merged activity.
func copyRelatedTableSingle(ctx context.Context, tx database.Querier, mergedID string, sourceID string, tableName string, onConflictClause string) error {
	// Defensive: both IDs are interpolated into SQL identifier strings via fmt.Sprintf.
	// Validate they are well-formed UUIDs to prevent SQL injection if the call path
	// ever changes to accept external input.
	if _, err := uuid.Parse(mergedID); err != nil {
		return fmt.Errorf("copyRelatedTableSingle: mergedID not a valid UUID: %w", err)
	}
	if _, err := uuid.Parse(sourceID); err != nil {
		return fmt.Errorf("copyRelatedTableSingle: sourceID not a valid UUID: %w", err)
	}
	if _, ok := allowedRelatedTables[tableName]; !ok {
		return fmt.Errorf("copyRelatedTableSingle: table %q not in allow-list", tableName)
	}
	tempName := fmt.Sprintf("tmp_single_%s_%s", tableName, strings.ReplaceAll(mergedID, "-", ""))

	if _, err := tx.Exec(ctx, fmt.Sprintf(
		`CREATE TEMP TABLE %s (LIKE %s INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING GENERATED) ON COMMIT DROP`,
		tempName, tableName,
	)); err != nil {
		return fmt.Errorf("create temp %s: %w", tempName, err)
	}

	if _, err := tx.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s SELECT * FROM %s WHERE activity_id = $1`,
		tempName, tableName,
	), sourceID); err != nil {
		return fmt.Errorf("copy source to %s: %w", tempName, err)
	}

	if relatedTableHasOwnUUIDPK(tableName) {
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			`UPDATE %s SET activity_id = $1, id = gen_random_uuid()`, tempName),
			mergedID); err != nil {
			return fmt.Errorf("rewrite activity_id+id on %s: %w", tempName, err)
		}
	} else {
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			`UPDATE %s SET activity_id = $1`, tempName),
			mergedID); err != nil {
			return fmt.Errorf("rewrite activity_id on %s: %w", tempName, err)
		}
	}

	if _, err := tx.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s SELECT * FROM %s %s`, tableName, tempName, onConflictClause,
	)); err != nil {
		return fmt.Errorf("insert %s from temp (single): %w", tableName, err)
	}
	return nil
}

// relatedTableHasOwnUUIDPK returns true when a Phase 4b related table has a standalone
// UUID `id` column that must be re-rolled during the TEMP-pattern INSERT to prevent
// PK or unique-constraint collisions on the target table.
//
// Tables with compound PKs (activity_id, other_col) and NO separate id column
// return false — activity_id rewrite alone suffices.
func relatedTableHasOwnUUIDPK(table string) bool {
	switch table {
	case "activity_photos":
		return true // PK: id UUID
	case "activity_geo_tags":
		return true // PK: id UUID
	case "activity_privacy_sections":
		return true // PK: id UUID
	case "activity_developer_apps":
		return true // id UUID (unique idx), PK (activity_id, application_id)
	default:
		return false // compound PK, no standalone id
	}
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
//   - has_edits (TRUE), parent_id (NULL),
//   - is_group_secondary (FALSE, trigger),
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
	Sources         []string          `json:"sources"`   // all equal, min 2, max 6
	FieldMap        map[string]string `json:"field_map"` // "hr" → "uuid-B", "gps" → "fuse"
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

	// Preflight: after holding the lock, re-check that no source is already fused.
	// This closes the race window between two parallel FusionMerge calls for the same
	// user: the first commits its memberships, the second (after lock-release wait)
	// sees them and returns 409 Conflict.
	var alreadyFusedV2 bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM activity_fusion_memberships WHERE source_id = ANY($1))`,
		req.Sources,
	).Scan(&alreadyFusedV2); err != nil {
		slog.Error("fusion-merge-v2: preflight query failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "preflight failed", r.URL.Path)
		return
	}
	if alreadyFusedV2 {
		httputil.WriteError(w, http.StatusConflict, "Conflict", ErrAlreadyFused.Error(), r.URL.Path)
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
	if err := h.applyFusionTxExtras(ctx, tx, fp); err != nil {
		slog.Error("fusion-merge-v2: applyFusionTxExtras failed, rolling back", "merged_id", merged.ID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "merge finalization failed: "+err.Error(), r.URL.Path)
		return
	}

	// 8. In legacy mode, hide sources via parent_id — single atomic UPDATE.
	// Membership rows (step 9b) are the sole source-of-truth for fusion state.
	legacyMode := req.GroupID == nil || *req.GroupID == ""
	if legacyMode {
		sourcesToHideV2 := make([]string, 0, len(req.Sources))
		for _, id := range req.Sources {
			if id != merged.ID {
				sourcesToHideV2 = append(sourcesToHideV2, id)
			}
		}
		if len(sourcesToHideV2) > 0 {
			if _, err := tx.Exec(ctx,
				`UPDATE activities SET parent_id = $1 WHERE id = ANY($2) AND user_id = $3`,
				merged.ID, sourcesToHideV2, userID,
			); err != nil {
				slog.Error("fusion-merge-v2: hide sources failed", "error", err)
				httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "hide sources (v2) failed: "+err.Error(), r.URL.Path)
				return
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

	// 9b. Insert into activity_fusion_memberships — the sole source-of-truth for fusion state
	// since Phase 3. parent_id (step 8) is a UI-visibility hint only; memberships own the data.
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

	// Pre-check OUTSIDE the advisory lock for fast-path rejection of non-fusion
	// activities. Running on db.Pool (READ COMMITTED) means this is *best-effort*:
	// a concurrent RevertFusion that just committed will be seen as either empty
	// (→ 400 "not a fusion") or non-empty (→ acquire lock → re-read empty → 409
	// Conflict). Both responses are correct; clients should treat 400/409 as
	// "this fusion is already being reverted or doesn't exist anymore".
	// The authoritative check is inside the advisory lock on the GetFusionMembershipsForMergedTx
	// call below (after pg_advisory_xact_lock).
	// user_id filter: defense-in-depth — a second call-site that omits the prior GetActivity
	// ownership check must not be able to observe another user's memberships.
	var preCheckCount int
	if err := h.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_fusion_memberships
		 WHERE merged_id = $1 AND user_id = $2`,
		id, userID,
	).Scan(&preCheckCount); err != nil {
		slog.Error("revert-fusion: pre-check get memberships failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "get memberships failed", r.URL.Path)
		return
	}
	if preCheckCount == 0 {
		// No memberships outside the lock → definitively not a fusion result.
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "activity is not a fusion result", r.URL.Path)
		return
	}

	// ── Atomic state-flip: parent_id + memberships ────────────────────────────
	// Advisory lock MUST be acquired BEFORE re-reading memberships to eliminate the
	// race between concurrent RevertFusion calls for the same user:
	//   Without the lock-before-reread, two concurrent requests can both read non-empty
	//   memberships, both enter the flip, and the second hits a 500 on DeleteActivity
	//   (already deleted by the first). With the lock-before-reread, the second request
	//   reads empty memberships under the lock and returns 409 Conflict (idempotent).
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

	// Re-read memberships INSIDE the advisory lock.
	// If empty now but non-empty in the pre-check, a concurrent revert completed
	// while we were waiting for the lock — return 409 Conflict (idempotent).
	memberships, err := h.DB.GetFusionMembershipsForMergedTx(ctx, flipTx, id)
	if err != nil {
		slog.Error("revert-fusion: get memberships failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "get memberships failed", r.URL.Path)
		return
	}
	if len(memberships) == 0 {
		// Pre-check had memberships; under-lock they are gone — concurrent revert won.
		httputil.WriteError(w, http.StatusConflict, "Conflict", "activity already reverted by concurrent request", r.URL.Path)
		return
	}

	// Collect source IDs via zwei Wege:
	// 1. parent_id-Query (V1-Sources haben parent_id = id) — must run inside lock TX
	// 2. Memberships already read above (V2-Sources haben parent_id=NULL)
	seenIDs := make(map[string]struct{})
	var sourceIDs []string

	// Weg 1: parent_id (V1) — read inside the TX so we see consistent state
	v1Rows, qErr := flipTx.Query(ctx,
		`SELECT id FROM activities WHERE parent_id = $1 AND user_id = $2`, id, userID,
	)
	if qErr != nil {
		slog.Error("revert-fusion: query parent_id children failed", "merged_id", id, "error", qErr)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "query V1 sources failed", r.URL.Path)
		return
	}
	for v1Rows.Next() {
		var sid string
		if err := v1Rows.Scan(&sid); err == nil {
			if _, dup := seenIDs[sid]; !dup {
				seenIDs[sid] = struct{}{}
				sourceIDs = append(sourceIDs, sid)
			}
		}
	}
	v1Rows.Close()

	// Weg 2: Memberships (V2 — sources haben keine parent_id)
	for _, m := range memberships {
		if _, dup := seenIDs[m.SourceID]; !dup {
			seenIDs[m.SourceID] = struct{}{}
			sourceIDs = append(sourceIDs, m.SourceID)
		}
	}

	// (a) Reset parent_id = NULL for all sources.
	// NOT EXISTS guard: if a source is also referenced by another fusion (merged_id != id),
	// its parent_id is left unchanged — it remains hidden under that other merged activity.
	// The cross-fusion guard reads activity_fusion_memberships directly.
	if len(sourceIDs) > 0 {
		if _, err := flipTx.Exec(ctx,
			`UPDATE activities SET parent_id = NULL
			 WHERE id = ANY($1) AND user_id = $2
			   AND NOT EXISTS (
			       SELECT 1 FROM activity_fusion_memberships
			       WHERE source_id = activities.id AND merged_id != $3::uuid
			   )`,
			sourceIDs, userID, id,
		); err != nil {
			slog.Error("revert-fusion: reset V2 sources failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "reset V2 failed", r.URL.Path)
			return
		}
	}

	// (b) Reset parent_id on any source that still points at the merged activity.
	// Applies to both V1 (always set parent_id=merged on sources) and V2-legacy-mode
	// (the default V2 path without group context, which also sets parent_id).
	// For V2-group-mode (group_id in the request), parent_id was never set and this UPDATE is a no-op.
	// CASCADE on DELETE activities removes the fusion from recording_group_members; the DEFERRED
	// trigger recomputes is_group_secondary + primary_activity_id.
	if _, err := flipTx.Exec(ctx,
		`UPDATE activities SET parent_id = NULL WHERE parent_id = $1 AND user_id = $2`,
		id, userID,
	); err != nil {
		slog.Error("revert-fusion: restore V1 sources failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "restore V1 failed", r.URL.Path)
		return
	}

	// (c) Restore photos to their original source activities so the subsequent
	// DeleteActivity(mergedID) does NOT CASCADE-delete them. Runs inside flipTx to
	// preserve atomicity — if the restore fails the entire revert rolls back, memberships
	// remain intact, and the user gets a 500 they can retry. Photos without
	// source_activity_id (uploaded directly to the merged activity, not reassigned during
	// fusion) have no origin to restore to and will be CASCADE-deleted when the merged
	// activity row is deleted post-commit.
	// user_id filter: defense-in-depth, mirrors the copyRelatedTablesUnion reassign.
	if _, err := flipTx.Exec(ctx,
		`UPDATE activity_photos
		 SET activity_id = source_activity_id,
		     source_activity_id = NULL
		 WHERE activity_id = $1
		   AND user_id = $2
		   AND source_activity_id IS NOT NULL`,
		id, userID,
	); err != nil {
		slog.Error("revert-fusion: restore photos failed", "merged_id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error",
			"restore photos failed", r.URL.Path)
		return
	}

	// (d) Memberships DELETE runs before DeleteActivity so RevertFusion leaves the DB in a
	// consistent state for any observer (tests, operators) between the two steps.
	if err := database.DeleteFusionMembershipsForMerged(ctx, flipTx, id); err != nil {
		slog.Error("revert-fusion: delete memberships failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "delete memberships failed", r.URL.Path)
		return
	}

	// Commit — parent_id flip + photo-restore + memberships-delete happen atomically.
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
