package handler

// activity_edit_ops.go — non-destructive Trim, Split, and Merge operations.
//
// Design principles:
//   - Original stream data is never deleted (edits are logical).
//   - Every operation is recorded in the activity_edits table (audit log).
//   - For trim: update activity metadata to reflect the trimmed window.
//   - For split: create a second activity record; streams are shared via
//     the edit metadata (actual stream re-assignment is deferred to enrichment).
//   - For merge: copy selected stream columns or metadata from source→target.
//   - Every operation also appends a record to the .edits.json sidecar in S3.

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/editor"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ── Trim ──────────────────────────────────────────────────────────────────────

// TrimActivity logically trims the start and/or end of an activity.
// The original streams are not deleted; the activity metadata is updated to
// reflect the new time window and recalculated metrics.
//
// POST /api/v1/activities/{id}/trim
// Body: { "trim_start": "RFC3339", "trim_end": "RFC3339" }
func (h *Handler) TrimActivity(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")

	// Ownership check.
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("trim: get activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	var req struct {
		TrimStart *string `json:"trim_start"` // RFC3339 timestamp (optional)
		TrimEnd   *string `json:"trim_end"`   // RFC3339 timestamp (optional)
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.TrimStart == nil && req.TrimEnd == nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "at least one of trim_start or trim_end is required", r.URL.Path)
		return
	}

	// Parse timestamps.
	var trimStart, trimEnd time.Time
	if req.TrimStart != nil {
		trimStart, err = time.Parse(time.RFC3339, *req.TrimStart)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Bad Request", "trim_start is not a valid RFC3339 timestamp", r.URL.Path)
			return
		}
	} else {
		trimStart = activity.StartedAt
	}
	if req.TrimEnd != nil {
		trimEnd, err = time.Parse(time.RFC3339, *req.TrimEnd)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Bad Request", "trim_end is not a valid RFC3339 timestamp", r.URL.Path)
			return
		}
	} else if activity.EndedAt != nil {
		trimEnd = *activity.EndedAt
	} else {
		trimEnd = time.Now()
	}

	if !trimStart.Before(trimEnd) {
		writeError(w, http.StatusBadRequest, "Bad Request", "trim_start must be before trim_end", r.URL.Path)
		return
	}

	// Load stream data to recalculate metrics.
	streams, err := h.DB.GetStreamPoints(r.Context(), activityID)
	if err != nil {
		slog.Error("trim: get streams failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load streams", r.URL.Path)
		return
	}

	// Filter to the trimmed window and recalculate metrics.
	trimmed := editor.FilterStreamsByWindow(streams, trimStart, trimEnd)
	metrics := editor.MetricsFromStreams(trimmed)

	// Persist updated metrics to the activities table.
	if err := h.applyTrimMetrics(r.Context(), activityID, metrics); err != nil {
		slog.Error("trim: update metrics failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update activity metrics", r.URL.Path)
		return
	}

	// Record in activity_edits audit log.
	editData, _ := json.Marshal(map[string]any{
		"trim_start": trimStart.Format(time.RFC3339),
		"trim_end":   trimEnd.Format(time.RFC3339),
	})
	if _, execErr := h.DB.Pool.Exec(r.Context(),
		`INSERT INTO activity_edits (activity_id, user_id, edit_type, edit_data)
		 VALUES ($1, $2, 'trim', $3)`,
		activityID, userID, editData,
	); execErr != nil {
		slog.Warn("trim: failed to insert audit record", "id", activityID, "error", execErr)
	}

	// Append trim edits to the S3 sidecar.
	h.appendSidecarEdits(r.Context(), userID, activityID, []editor.Edit{
		{
			Type:      "trim_start",
			Timestamp: req.TrimStart,
		},
		{
			Type:      "trim_end",
			Timestamp: req.TrimEnd,
		},
	})

	// Return updated activity.
	updated, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to reload activity", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// applyTrimMetrics updates activities table with recalculated values from trim.
func (h *Handler) applyTrimMetrics(ctx context.Context, activityID string, m editor.StreamMetrics) error {
	endedAt := m.EndedAt
	elapsed := m.ElapsedS
	moving := m.MovingS
	dist := m.DistanceM
	avgSpd := m.AvgSpeedMS
	maxSpd := m.MaxSpeedMS

	_, err := h.DB.Pool.Exec(ctx,
		`UPDATE activities SET
			started_at      = $2,
			ended_at        = $3,
			elapsed_time_s  = $4,
			moving_time_s   = $5,
			distance_m      = CASE WHEN $6::float8 > 0 THEN $6 ELSE distance_m END,
			avg_speed_ms    = CASE WHEN $7::float8 > 0 THEN $7 ELSE avg_speed_ms END,
			max_speed_ms    = CASE WHEN $8::float8 > 0 THEN $8 ELSE max_speed_ms END,
			avg_heart_rate  = COALESCE($9, avg_heart_rate),
			max_heart_rate  = COALESCE($10, max_heart_rate),
			avg_power_w     = COALESCE($11, avg_power_w),
			max_power_w     = COALESCE($12, max_power_w),
			avg_cadence     = COALESCE($13, avg_cadence),
			avg_temperature_c = COALESCE($14, avg_temperature_c),
			has_edits       = true,
			updated_at      = NOW()
		WHERE id = $1`,
		activityID,
		m.StartedAt,
		endedAt,
		elapsed,
		moving,
		dist,
		avgSpd,
		maxSpd,
		m.AvgHR,
		m.MaxHR,
		m.AvgPW,
		m.MaxPW,
		m.AvgCad,
		m.AvgTemp,
	)
	return err
}

// ── Split ─────────────────────────────────────────────────────────────────────

// SplitActivity splits an activity at a given timestamp.
// A new activity record is created for the second part; both activities share
// streams (the split boundary is stored in edit metadata).
//
// POST /api/v1/activities/{id}/split
// Body: { "split_at": "RFC3339" }
func (h *Handler) SplitActivity(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")

	// Ownership check.
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("split: get activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	var req struct {
		SplitAt string `json:"split_at"` // RFC3339
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	splitAt, err := time.Parse(time.RFC3339, req.SplitAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "split_at is not a valid RFC3339 timestamp", r.URL.Path)
		return
	}

	// Validate that the split point falls within the activity window.
	if !splitAt.After(activity.StartedAt) {
		writeError(w, http.StatusBadRequest, "Bad Request", "split_at must be after activity start", r.URL.Path)
		return
	}
	if activity.EndedAt != nil && !splitAt.Before(*activity.EndedAt) {
		writeError(w, http.StatusBadRequest, "Bad Request", "split_at must be before activity end", r.URL.Path)
		return
	}

	// Load streams and split into two windows.
	streams, err := h.DB.GetStreamPoints(r.Context(), activityID)
	if err != nil {
		slog.Error("split: get streams failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load streams", r.URL.Path)
		return
	}

	part1Streams := editor.FilterStreamsBefore(streams, splitAt)
	part2Streams := editor.FilterStreamsAfter(streams, splitAt)

	if len(part1Streams) < 2 {
		writeError(w, http.StatusUnprocessableEntity, "Unprocessable", "split point too close to activity start (fewer than 2 stream points in part 1)", r.URL.Path)
		return
	}
	if len(part2Streams) < 2 {
		writeError(w, http.StatusUnprocessableEntity, "Unprocessable", "split point too close to activity end (fewer than 2 stream points in part 2)", r.URL.Path)
		return
	}

	metrics1 := editor.MetricsFromStreams(part1Streams)
	metrics2 := editor.MetricsFromStreams(part2Streams)

	// Update the original activity to reflect part 1.
	if err := h.applyTrimMetrics(r.Context(), activityID, metrics1); err != nil {
		slog.Error("split: update part1 metrics failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update activity metrics", r.URL.Path)
		return
	}

	// Determine suffix for the new activity title.
	origTitle := "(Teil 2)"
	if activity.Title != nil && *activity.Title != "" {
		origTitle = *activity.Title + " (Teil 2)"
	}

	// Create the new activity for part 2 by copying the original's properties.
	newID := uuid.New().String()
	_, err = h.DB.Pool.Exec(r.Context(), `
		INSERT INTO activities (
			id, user_id,
			sport_type, sub_type, parsed_sport_type, parsed_sub_type,
			fit_sport_id, fit_sub_sport_id,
			title, description,
			is_indoor, is_ebike,
			gear_id,
			visibility,
			enrichment_status,
			source, source_device,
			original_filename, original_file_format,
			started_at, ended_at, elapsed_time_s, moving_time_s,
			distance_m,
			avg_speed_ms, max_speed_ms,
			avg_heart_rate, max_heart_rate,
			avg_power_w, max_power_w,
			avg_cadence, avg_temperature_c,
			has_edits
		)
		SELECT
			$2, user_id,
			sport_type, sub_type, parsed_sport_type, parsed_sub_type,
			fit_sport_id, fit_sub_sport_id,
			$3, description,
			is_indoor, is_ebike,
			gear_id,
			visibility,
			'pending',
			source, source_device,
			original_filename, original_file_format,
			$4, $5, $6, $7,
			$8,
			$9, $10,
			$11, $12,
			$13, $14,
			$15, $16,
			true
		FROM activities WHERE id = $1`,
		activityID, newID, origTitle,
		metrics2.StartedAt,
		func() any {
			if metrics2.EndedAt.IsZero() {
				return nil
			}
			return metrics2.EndedAt
		}(),
		metrics2.ElapsedS,
		metrics2.MovingS,
		func() any {
			if metrics2.DistanceM == 0 {
				return nil
			}
			return metrics2.DistanceM
		}(),
		func() any {
			if metrics2.AvgSpeedMS == 0 {
				return nil
			}
			return metrics2.AvgSpeedMS
		}(),
		func() any {
			if metrics2.MaxSpeedMS == 0 {
				return nil
			}
			return metrics2.MaxSpeedMS
		}(),
		metrics2.AvgHR, metrics2.MaxHR,
		metrics2.AvgPW, metrics2.MaxPW,
		metrics2.AvgCad, metrics2.AvgTemp,
	)
	if err != nil {
		slog.Error("split: create new activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create split activity", r.URL.Path)
		return
	}

	// Copy part2 streams to the new activity.
	if err := h.DB.InsertStreams(r.Context(), newID, part2Streams); err != nil {
		slog.Error("split: insert streams for new activity failed", "new_id", newID, "error", err)
		// Best-effort: don't fail the entire operation but log it.
	}

	// Build track for the new activity from part2 GPS points.
	var trackPts []model.TrackPoint
	for _, p := range part2Streams {
		if p.Lat != nil && p.Lng != nil {
			trackPts = append(trackPts, model.TrackPoint{Lat: *p.Lat, Lng: *p.Lng})
		}
	}
	if len(trackPts) >= 2 {
		if err := h.DB.InsertTrack(r.Context(), newID, trackPts); err != nil {
			slog.Warn("split: insert track for new activity failed", "new_id", newID, "error", err)
		}
	}

	// Record split edit for original activity.
	editData, _ := json.Marshal(map[string]any{
		"split_at":        req.SplitAt,
		"new_activity_id": newID,
		"part":            1,
	})
	if _, execErr := h.DB.Pool.Exec(r.Context(),
		`INSERT INTO activity_edits (activity_id, user_id, edit_type, edit_data)
		 VALUES ($1, $2, 'split', $3)`,
		activityID, userID, editData,
	); execErr != nil {
		slog.Warn("split: failed to insert audit record for part1", "id", activityID, "error", execErr)
	}

	// Record split edit for new activity.
	editData2, _ := json.Marshal(map[string]any{
		"split_at":             req.SplitAt,
		"original_activity_id": activityID,
		"part":                 2,
	})
	if _, execErr := h.DB.Pool.Exec(r.Context(),
		`INSERT INTO activity_edits (activity_id, user_id, edit_type, edit_data)
		 VALUES ($1, $2, 'split', $3)`,
		newID, userID, editData2,
	); execErr != nil {
		slog.Warn("split: failed to insert audit record for part2", "new_id", newID, "error", execErr)
	}

	splitAtStr := req.SplitAt
	h.appendSidecarEdits(r.Context(), userID, activityID, []editor.Edit{
		{Type: "trim_end", Timestamp: &splitAtStr},
	})

	writeJSON(w, http.StatusCreated, map[string]string{
		"activity_id":     activityID,
		"new_activity_id": newID,
	})
}

// ── Merge ─────────────────────────────────────────────────────────────────────

// MergeActivities copies selected fields from a source activity into the target.
// Valid fields: "hr", "power", "cadence", "temperature", "title", "tags", "gear".
//
// POST /api/v1/activities/{id}/merge
// Body: { "source_id": "uuid", "fields": ["hr", "power", ...] }
func (h *Handler) MergeActivities(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	targetID := chi.URLParam(r, "id")

	var req struct {
		SourceID string   `json:"source_id"`
		Fields   []string `json:"fields"` // "hr", "power", "cadence", "temperature", "title", "tags", "gear"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.SourceID == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "source_id is required", r.URL.Path)
		return
	}
	if len(req.Fields) == 0 {
		writeError(w, http.StatusBadRequest, "Bad Request", "at least one field is required", r.URL.Path)
		return
	}

	// Ownership check for target.
	target, err := h.DB.GetActivity(r.Context(), targetID)
	if err != nil {
		slog.Error("merge: get target activity failed", "id", targetID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load target activity", r.URL.Path)
		return
	}
	if target == nil || target.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "target activity not found", r.URL.Path)
		return
	}

	// Ownership check for source.
	source, err := h.DB.GetActivity(r.Context(), req.SourceID)
	if err != nil {
		slog.Error("merge: get source activity failed", "id", req.SourceID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load source activity", r.URL.Path)
		return
	}
	if source == nil || source.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "source activity not found", r.URL.Path)
		return
	}

	// Build a field set for O(1) lookup.
	fieldSet := make(map[string]bool, len(req.Fields))
	for _, f := range req.Fields {
		fieldSet[f] = true
	}

	// Merge stream columns (hr, power, cadence, temperature, gps, altitude, speed).
	streamFields := []string{}
	if fieldSet["hr"] {
		streamFields = append(streamFields, "hr")
	}
	if fieldSet["power"] {
		streamFields = append(streamFields, "power")
	}
	if fieldSet["cadence"] {
		streamFields = append(streamFields, "cadence")
	}
	if fieldSet["temperature"] {
		streamFields = append(streamFields, "temperature")
	}
	if fieldSet["gps"] {
		streamFields = append(streamFields, "lat", "lng")
	}
	if fieldSet["altitude"] {
		streamFields = append(streamFields, "altitude")
	}
	if fieldSet["speed"] {
		streamFields = append(streamFields, "speed")
	}

	if len(streamFields) > 0 {
		if err := h.mergeStreamFields(r.Context(), targetID, req.SourceID, streamFields); err != nil {
			slog.Error("merge: stream fields failed", "target", targetID, "source", req.SourceID, "error", err)
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to merge stream fields", r.URL.Path)
			return
		}
	}

	// Rebuild track and heatmap after GPS merge.
	if fieldSet["gps"] {
		ctx := r.Context()
		if err := h.DB.RebuildActivityTrack(ctx, targetID); err != nil {
			slog.Warn("merge: track rebuild failed", "id", targetID, "err", err)
		}
		if err := h.DB.RebuildHeatmapData(ctx, targetID); err != nil {
			slog.Warn("merge: heatmap rebuild failed", "id", targetID, "err", err)
		}
		h.DB.InvalidateTerraIncognitaCache(ctx, userID)
	}

	// Merge title.
	if fieldSet["title"] && source.Title != nil {
		_, err = h.DB.Pool.Exec(r.Context(),
			`UPDATE activities SET title = $2, has_edits = true, updated_at = NOW() WHERE id = $1`,
			targetID, *source.Title,
		)
		if err != nil {
			slog.Warn("merge: update title failed", "error", err)
		}
	}

	// Merge gear.
	if fieldSet["gear"] && source.GearID != nil {
		_, err = h.DB.Pool.Exec(r.Context(),
			`UPDATE activities SET gear_id = $2, has_edits = true, updated_at = NOW() WHERE id = $1`,
			targetID, *source.GearID,
		)
		if err != nil {
			slog.Warn("merge: update gear failed", "error", err)
		}
	}

	// Merge tags: copy all tag associations from source to target.
	if fieldSet["tags"] {
		if err := h.mergeActivityTags(r.Context(), targetID, req.SourceID, userID); err != nil {
			slog.Warn("merge: copy tags failed", "error", err)
		}
	}

	// Mark source as fusion source.
	_, _ = h.DB.Pool.Exec(r.Context(),
		`UPDATE activities SET is_fusion_source = true, updated_at = NOW() WHERE id = $1`,
		req.SourceID,
	)

	// Record in activity_edits audit log.
	editData, _ := json.Marshal(map[string]any{
		"source_id": req.SourceID,
		"fields":    req.Fields,
	})
	if _, execErr := h.DB.Pool.Exec(r.Context(),
		`INSERT INTO activity_edits (activity_id, user_id, edit_type, edit_data)
		 VALUES ($1, $2, 'merge', $3)`,
		targetID, userID, editData,
	); execErr != nil {
		slog.Warn("merge: failed to insert audit record", "id", targetID, "error", execErr)
	}

	// Append to sidecar.
	h.appendSidecarEdits(r.Context(), userID, targetID, []editor.Edit{
		{
			Type:   "merge",
			Method: req.SourceID,
		},
	})

	// Return updated target activity.
	updated, err := h.DB.GetActivity(r.Context(), targetID)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to reload activity", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// mergeStreamFields copies the specified stream columns from the source activity's
// streams into the matching time slots of the target activity's streams.
// Only rows where both activities have a matching timestamp are updated.
func (h *Handler) mergeStreamFields(ctx context.Context, targetID, sourceID string, fields []string) error {
	for _, field := range fields {
		var col string
		switch field {
		case "hr":
			col = "heart_rate"
		case "power":
			col = "power_w"
		case "cadence":
			col = "cadence"
		case "temperature":
			col = "temperature_c"
		case "lat":
			col = "lat"
		case "lng":
			col = "lng"
		case "altitude":
			col = "altitude_m"
		case "speed":
			col = "speed_ms"
		default:
			continue
		}
		_, err := h.DB.Pool.Exec(ctx, `
			UPDATE activity_streams t
			SET `+col+` = s.`+col+`
			FROM activity_streams s
			WHERE t.activity_id = $1
			  AND s.activity_id = $2
			  AND t.time = s.time
			  AND s.`+col+` IS NOT NULL`,
			targetID, sourceID,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// mergeActivityTags copies all tag associations from sourceID to targetID.
func (h *Handler) mergeActivityTags(ctx context.Context, targetID, sourceID, userID string) error {
	// Get all tag IDs from source.
	rows, err := h.DB.Pool.Query(ctx,
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
		if err := h.DB.LinkActivityTag(ctx, targetID, tagID, "merge"); err != nil {
			slog.Warn("merge: link tag failed", "tag_id", tagID, "error", err)
		}
	}

	// Sync the denormalized tags TEXT[] cache on the target.
	_, _ = h.DB.Pool.Exec(ctx, `
		UPDATE activities a
		SET tags = COALESCE(
			ARRAY(
				SELECT DISTINCT t.name
				FROM activity_tags at_join
				JOIN tags t ON t.id = at_join.tag_id
				WHERE at_join.activity_id = a.id
			), '{}'::TEXT[]
		),
		updated_at = NOW()
		WHERE a.id = $1`, targetID,
	)
	return nil
}

// ── Sidecar helpers ───────────────────────────────────────────────────────────

// appendSidecarEdits loads the existing S3 sidecar, appends the given edits, and saves it back.
// Errors are logged as warnings (best-effort); the primary operation is not rolled back.
func (h *Handler) appendSidecarEdits(ctx context.Context, userID, activityID string, edits []editor.Edit) {
	bucket := editorBucket()
	key := editorKey(userID, activityID)

	raw, _, err := h.Storage.DownloadFromBucket(ctx, bucket, key)
	var sidecar editor.EditSidecar
	if err == nil {
		if jsonErr := json.Unmarshal(raw, &sidecar); jsonErr != nil {
			sidecar = editor.EditSidecar{Version: 1, ActivityID: activityID}
		}
	} else {
		sidecar = editor.EditSidecar{Version: 1, ActivityID: activityID}
	}
	if sidecar.Edits == nil {
		sidecar.Edits = []editor.Edit{}
	}
	sidecar.Edits = append(sidecar.Edits, edits...)
	sidecar.UpdatedAt = time.Now()
	if sidecar.CreatedAt.IsZero() {
		sidecar.CreatedAt = time.Now()
	}
	if sidecar.Version == 0 {
		sidecar.Version = 1
	}

	data, _ := json.Marshal(&sidecar)
	if uploadErr := h.Storage.UploadToBucket(ctx, bucket, key, data, "application/json"); uploadErr != nil {
		slog.Warn("edit-ops: failed to update sidecar", "activity_id", activityID, "error", uploadErr)
		return
	}
	// Ensure has_edits flag is set.
	_, _ = h.DB.Pool.Exec(ctx, `UPDATE activities SET has_edits = true WHERE id = $1`, activityID)
}
