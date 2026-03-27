package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
)

// ListTags returns all tags for the authenticated user.
func (h *Handler) ListTags(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	tags, err := h.DB.ListTags(r.Context(), userID)
	if err != nil {
		slog.Error("list tags failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list tags", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, tags)
}

// CreateTag creates a new tag for the authenticated user.
func (h *Handler) CreateTag(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	var req struct {
		Name            string          `json:"name"`
		Color           string          `json:"color"`
		TextColor       string          `json:"text_color"`
		Conditions      json.RawMessage `json:"conditions,omitempty"`
		ConditionGroups json.RawMessage `json:"condition_groups,omitempty"`
		ConditionsLogic string          `json:"conditions_logic,omitempty"`
		Description     *string         `json:"description,omitempty"`
		Lat             *float64        `json:"lat"`
		Lng             *float64        `json:"lng"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.Name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}

	// Apply defaults
	if req.Color == "" {
		req.Color = "#6366F1"
	}
	if req.TextColor == "" {
		req.TextColor = "#FFFFFF"
	}

	// Derive is_auto from presence of conditions — never trust client-supplied flag
	isAuto := len(req.ConditionGroups) > 2 || len(req.Conditions) > 2

	tag, err := h.DB.CreateTag(r.Context(), userID, req.Name, req.Color, req.TextColor, isAuto, []byte(req.Conditions), []byte(req.ConditionGroups), req.ConditionsLogic, req.Description, req.Lat, req.Lng)
	if err != nil {
		slog.Error("create tag failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create tag", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, tag)
}

// UpdateTag updates an existing tag.
func (h *Handler) UpdateTag(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	tagID := chi.URLParam(r, "id")

	// Use json.RawMessage for lat/lng so we can distinguish "absent" from "null".
	var req struct {
		Name            string           `json:"name"`
		Color           string           `json:"color"`
		TextColor       string           `json:"text_color"`
		Conditions      *json.RawMessage `json:"conditions,omitempty"`
		ConditionGroups *json.RawMessage `json:"condition_groups,omitempty"`
		ConditionsLogic *string          `json:"conditions_logic,omitempty"`
		Description     *string          `json:"description,omitempty"`
		Lat             *json.RawMessage `json:"lat"`
		Lng             *json.RawMessage `json:"lng"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	var condBytes *[]byte
	if req.Conditions != nil {
		b := []byte(*req.Conditions)
		condBytes = &b
	}

	var condGroupBytes *[]byte
	if req.ConditionGroups != nil {
		b := []byte(*req.ConditionGroups)
		condGroupBytes = &b
	}

	// Derive is_auto from updated conditions — never trust client-supplied flag.
	// Only recompute when at least one conditions field is being updated.
	var isAuto *bool
	if req.Conditions != nil || req.ConditionGroups != nil {
		var condGroupLen, condLen int
		if req.ConditionGroups != nil {
			condGroupLen = len(*req.ConditionGroups)
		}
		if req.Conditions != nil {
			condLen = len(*req.Conditions)
		}
		v := condGroupLen > 2 || condLen > 2
		isAuto = &v
	}

	// Parse lat/lng from raw JSON — nil raw = field absent = don't update.
	// "null" raw = clear location. A number = set location.
	var lat, lng *float64
	locationPresent := req.Lat != nil || req.Lng != nil
	if req.Lat != nil && string(*req.Lat) != "null" {
		var f float64
		if err := json.Unmarshal(*req.Lat, &f); err == nil {
			lat = &f
		}
	}
	if req.Lng != nil && string(*req.Lng) != "null" {
		var f float64
		if err := json.Unmarshal(*req.Lng, &f); err == nil {
			lng = &f
		}
	}

	tag, err := h.DB.UpdateTag(r.Context(), tagID, userID, req.Name, req.Color, req.TextColor, isAuto, condBytes, condGroupBytes, req.ConditionsLogic, req.Description, lat, lng, locationPresent)
	if err != nil {
		slog.Error("update tag failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update tag", r.URL.Path)
		return
	}
	if tag == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "tag not found", r.URL.Path)
		return
	}

	if locationPresent && lat != nil && lng != nil {
		tagIDForBackfill := tagID
		userIDForBackfill := userID
		// recompute=true: coordinates changed, re-calculate for all linked activities.
		// Both backfills run in parallel with a 5-minute timeout.
		go func() {
			ctx2, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			g, gCtx := errgroup.WithContext(ctx2)
			g.Go(func() error {
				if err := h.backfillSunTimesForLocationTag(gCtx, userIDForBackfill, tagIDForBackfill, nil, true); err != nil {
					slog.Warn("update tag: sun times backfill failed", "tag_id", tagIDForBackfill, "error", err)
				}
				return nil
			})
			g.Go(func() error {
				if err := h.backfillWeatherForLocationTag(gCtx, userIDForBackfill, tagIDForBackfill, nil, true); err != nil {
					slog.Warn("update tag: weather backfill failed", "tag_id", tagIDForBackfill, "error", err)
				}
				return nil
			})
			_ = g.Wait()
		}()
	}

	httputil.WriteJSON(w, http.StatusOK, tag)
}

// DeleteTag deletes a tag.
func (h *Handler) DeleteTag(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	tagID := chi.URLParam(r, "id")

	// Resolve cover file references before deletion so we can clean up storage.
	var coverFileID string
	_ = h.DB.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(cover_file_id::text, '')
		 FROM tags WHERE id = $1 AND user_id = $2`,
		tagID, userID,
	).Scan(&coverFileID)

	if err := h.DB.DeleteTag(r.Context(), tagID, userID); err != nil {
		if err.Error() == "tag not found" {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "tag not found", r.URL.Path)
			return
		}
		slog.Error("delete tag failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete tag", r.URL.Path)
		return
	}

	// Best-effort ShadowFS cleanup (cover image).
	if coverFileID != "" && h.ShadowFS != nil {
		if delErr := h.ShadowFS.Delete(r.Context(), userID, coverFileID); delErr != nil {
			slog.Warn("delete tag: ShadowFS.Delete cover failed", "tag_id", tagID, "file_id", coverFileID, "error", delErr)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// UploadTagCoverImage uploads a cover image for a tag (location pin).
// Supports multipart file upload (field "file") or JSON body with { "activity_photo_url": "..." }.
// POST /api/v1/tags/{id}/cover-image
func (h *Handler) UploadTagCoverImage(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	tagID := chi.URLParam(r, "id")

	// Ownership check
	tag, err := h.DB.GetTagByID(r.Context(), tagID, userID)
	if err != nil {
		slog.Error("upload tag cover: get tag failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load tag", r.URL.Path)
		return
	}
	if tag == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "tag not found", r.URL.Path)
		return
	}

	ct := r.Header.Get("Content-Type")

	// JSON body path: legacy — now a no-op.
	// Now a no-op: just return the current tag state.
	if strings.HasPrefix(ct, "application/json") {
		updated, err := h.DB.UpdateTag(r.Context(), tagID, userID, "", "", "", nil, nil, nil, nil, nil, nil, nil, false)
		if err != nil {
			slog.Error("update tag cover failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update tag", r.URL.Path)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, updated)
		return
	}

	// Multipart file upload path
	const maxCoverSize = 10 << 20 // 10 MB
	r.Body = http.MaxBytesReader(w, r.Body, maxCoverSize)
	if err := r.ParseMultipartForm(maxCoverSize); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "file too large (max 10 MB)", r.URL.Path)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "no file in request (field: image)", r.URL.Path)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		slog.Error("read cover image data failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to read file", r.URL.Path)
		return
	}

	// Validate image type by inspecting the actual file bytes (magic bytes),
	// not the client-supplied Content-Type header which can be spoofed.
	detectedCT, errMsg := validateImageBytes(data)
	if errMsg != "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", errMsg, r.URL.Path)
		return
	}
	fileCT := detectedCT

	// Resolve old cover_file_id so we can delete it after successful DB update.
	var oldCoverFileID string
	_ = h.DB.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(cover_file_id::text, '') FROM tags WHERE id = $1 AND user_id = $2`,
		tagID, userID,
	).Scan(&oldCoverFileID)

	// Upload cover image to ShadowFS.
	var coverFileID string
	if h.ShadowFS != nil {
		rec, putErr := h.ShadowFS.Put(r.Context(), shadowfs.FileEntry{
			UserID:      userID,
			FileType:    "media",
			ContentType: fileCT,
			FileName:    header.Filename,
			SizeBytes:   int64(len(data)),
			ParentType:  func() *string { s := "tag"; return &s }(),
			ParentID:    &tagID,
			Tags:        map[string]string{"role": "cover"},
		}, bytes.NewReader(data))
		if putErr != nil {
			slog.Error("shadowfs upload tag cover failed", "error", putErr)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to store cover image", r.URL.Path)
			return
		}
		coverFileID = rec.ID
	} else {
		httputil.WriteError(w, http.StatusServiceUnavailable, "Service Unavailable", "no storage configured", r.URL.Path)
		return
	}

	updated, err := h.DB.UpdateTag(r.Context(), tagID, userID, "", "", "", nil, nil, nil, nil, nil, nil, nil, false)
	if err != nil {
		slog.Error("update tag cover_file_id failed", "error", err)
		// DB update failed — clean up the new file we already stored.
		if coverFileID != "" && h.ShadowFS != nil {
			if delErr := h.ShadowFS.Delete(r.Context(), userID, coverFileID); delErr != nil {
				slog.Warn("upload tag cover: ShadowFS.Delete new cover rollback failed", "tag_id", tagID, "file_id", coverFileID, "error", delErr)
			}
		}
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update tag", r.URL.Path)
		return
	}

	// Link ShadowFS record to domain row.
	if coverFileID != "" {
		if _, dbErr := h.DB.Pool.Exec(r.Context(),
			`UPDATE tags SET cover_file_id = $1 WHERE id = $2`,
			coverFileID, tagID,
		); dbErr != nil {
			slog.Warn("tags: set cover_file_id failed", "tag_id", tagID, "error", dbErr)
		}
	}

	// Delete old cover AFTER successful DB update to avoid orphaning new file on error.
	if oldCoverFileID != "" && h.ShadowFS != nil {
		if delErr := h.ShadowFS.Delete(r.Context(), userID, oldCoverFileID); delErr != nil {
			slog.Warn("upload tag cover: ShadowFS.Delete old cover failed", "tag_id", tagID, "file_id", oldCoverFileID, "error", delErr)
		}
	}

	httputil.WriteJSON(w, http.StatusOK, updated)
}

// ClearTagCoverImage löscht das Cover-Bild und den Thumbnail eines Tags.
// Das verknüpfte Activity-Photo bleibt unangetastet.
// DELETE /api/v1/tags/{id}/cover-image
func (h *Handler) ClearTagCoverImage(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	tagID := chi.URLParam(r, "id")

	tag, err := h.DB.GetTagByID(r.Context(), tagID, userID)
	if err != nil {
		slog.Error("clear tag cover: get tag failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load tag", r.URL.Path)
		return
	}
	if tag == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "tag not found", r.URL.Path)
		return
	}

	// Resolve ShadowFS file_id for deletion (best-effort).
	var coverFileID string
	_ = h.DB.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(cover_file_id::text, '') FROM tags WHERE id = $1 AND user_id = $2`,
		tagID, userID,
	).Scan(&coverFileID)

	// ShadowFS delete.
	if coverFileID != "" && h.ShadowFS != nil {
		if delErr := h.ShadowFS.Delete(r.Context(), userID, coverFileID); delErr != nil {
			slog.Warn("clear tag cover: ShadowFS.Delete failed", "tag_id", tagID, "file_id", coverFileID, "error", delErr)
		}
	}

	updated, err := h.DB.ClearTagCoverFile(r.Context(), tagID, userID)
	if err != nil {
		slog.Error("clear tag cover: DB update failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to clear cover image", r.URL.Path)
		return
	}
	if updated == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "tag not found", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, updated)
}

// AssignActivitiesToTag manually assigns a list of activities to a tag.
// POST /api/v1/tags/{id}/assign-activities
// Body: {"activity_ids": ["id1", "id2"]}
func (h *Handler) AssignActivitiesToTag(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	tagID := chi.URLParam(r, "id")

	var req struct {
		ActivityIDs []string `json:"activity_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if len(req.ActivityIDs) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "activity_ids must not be empty", r.URL.Path)
		return
	}

	// Clear any removal records — user is explicitly re-adding this tag.
	for _, actID := range req.ActivityIDs {
		if err := h.DB.ClearTagRemoval(r.Context(), actID, tagID); err != nil {
			slog.Warn("clear tag removal", "err", err, "activity", actID, "tag", tagID)
		}
	}

	if err := h.DB.AssignActivitiesToTag(r.Context(), tagID, userID, req.ActivityIDs); err != nil {
		slog.Error("assign activities to tag failed", "tag_id", tagID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to assign activities", r.URL.Path)
		return
	}

	// Location-Pin-Trigger: fetch weather/sun-times for indoor activities that now have a location via this tag.
	// recompute=false: only fill activities that have no data yet (newly assigned).
	activityIDs := req.ActivityIDs
	go func() {
		ctx2, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := h.backfillWeatherForLocationTag(ctx2, userID, tagID, activityIDs, false); err != nil {
			slog.Warn("location tag weather backfill failed", "tag_id", tagID, "error", err)
		}
		if err := h.backfillSunTimesForLocationTag(ctx2, userID, tagID, activityIDs, false); err != nil {
			slog.Warn("location tag sun times backfill failed", "tag_id", tagID, "error", err)
		}
	}()

	w.WriteHeader(http.StatusNoContent)
}

// backfillWeatherForLocationTag fetches weather for indoor activities using the tag's coordinates.
// When recompute=true, all activities linked to the tag are re-fetched (activityIDs is ignored
// and the NOT EXISTS filter is dropped). When recompute is true, activityIDs is ignored and all
// indoor activities linked to the tag are reprocessed.
// When recompute=false, only activities in activityIDs that have no weather data yet are processed.
func (h *Handler) backfillWeatherForLocationTag(
	ctx context.Context, userID, tagID string, activityIDs []string, recompute bool,
) error {
	if recompute && len(activityIDs) > 0 {
		slog.Warn("backfillWeatherForLocationTag: activityIDs passed with recompute=true, IDs will be ignored", "tag_id", tagID, "activity_count", len(activityIDs))
	}
	// Read tag coordinates
	var lat, lng *float64
	err := h.DB.Pool.QueryRow(ctx,
		`SELECT lat, lng FROM tags WHERE id = $1 AND user_id = $2`,
		tagID, userID,
	).Scan(&lat, &lng)
	if err != nil || lat == nil || lng == nil {
		return nil // not a location tag
	}

	var rows pgx.Rows
	if recompute {
		// Re-compute weather for ALL indoor activities linked to this tag
		rows, err = h.DB.Pool.Query(ctx, `
			SELECT a.id, a.started_at
			FROM activities a
			JOIN activity_tags at ON at.activity_id = a.id
			WHERE at.tag_id = $1
			  AND a.user_id = $2
			  AND a.has_gps = false
			  AND a.started_at IS NOT NULL`,
			tagID, userID,
		)
	} else {
		// Only fill missing weather for the given activity IDs
		rows, err = h.DB.Pool.Query(ctx, `
			SELECT a.id, a.started_at
			FROM activities a
			WHERE a.id = ANY($1)
			  AND a.user_id = $2
			  AND a.has_gps = false
			  AND NOT EXISTS (SELECT 1 FROM activity_weather aw WHERE aw.activity_id = a.id)`,
			activityIDs, userID,
		)
	}
	if err != nil {
		return err
	}
	defer rows.Close()

	type candidate struct {
		id        string
		startedAt time.Time
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.startedAt); err == nil {
			candidates = append(candidates, c)
		}
	}
	rows.Close()

	for _, c := range candidates {
		req := enrichment.WeatherRequest{
			ReadingType: "single",
			Lat:         *lat,
			Lng:         *lng,
			ElevationM:  0,
			At:          c.startedAt,
		}
		readings, err := enrichment.FetchWeatherReadings(ctx, []enrichment.WeatherRequest{req})
		if err != nil {
			slog.Warn("location pin weather fetch failed", "activity_id", c.id, "error", err)
			continue
		}
		if err := h.DB.InsertWeatherReadings(ctx, c.id, readings); err != nil {
			slog.Warn("location pin weather insert failed", "activity_id", c.id, "error", err)
		} else {
			// Sync weather tags to activity_tags junction table
			weatherPrefs, _ := h.DB.GetWeatherPrefs(ctx, userID)
			if weatherPrefs.AutoTags {
				var allWeatherTags []string
				for _, r := range readings {
					allWeatherTags = append(allWeatherTags, r.Tags...)
				}
				if err := h.DB.SyncWeatherTagsToActivityTags(ctx, c.id, userID, allWeatherTags); err != nil {
					slog.Warn("location pin weather: sync weather tags failed", "activity_id", c.id, "error", err)
				}
			}
		}
	}
	return nil
}

// backfillSunTimesForLocationTag fills sunrise_at, sunset_at and time_of_day for
// indoor activities linked to the given location tag.
// When recompute=true, all linked activities are re-processed (sunrise_at IS NULL filter dropped);
// activityIDs is ignored in that case.
// When recompute=false and activityIDs is non-empty, only those specific activities are processed
// (restricted to ones that still have no sun data). When recompute=false and activityIDs is empty,
// all linked activities without sun data are filled.
func (h *Handler) backfillSunTimesForLocationTag(ctx context.Context, userID, tagID string, activityIDs []string, recompute bool) error {
	// Read tag coordinates
	var lat, lng *float64
	err := h.DB.Pool.QueryRow(ctx,
		`SELECT lat, lng FROM tags WHERE id = $1 AND user_id = $2`,
		tagID, userID,
	).Scan(&lat, &lng)
	if err != nil || lat == nil || lng == nil {
		return nil // not a location tag
	}

	// Find indoor activities linked to this tag
	var rows pgx.Rows
	if recompute {
		rows, err = h.DB.Pool.Query(ctx, `
		SELECT a.id, a.started_at, a.ended_at
		FROM activities a
		JOIN activity_tags at ON at.activity_id = a.id
		WHERE at.tag_id = $1
		  AND a.user_id = $2
		  AND a.has_gps = false
		  AND a.started_at IS NOT NULL
		  AND a.ended_at IS NOT NULL`,
			tagID, userID)
	} else if len(activityIDs) > 0 {
		rows, err = h.DB.Pool.Query(ctx, `
		SELECT a.id, a.started_at, a.ended_at
		FROM activities a
		JOIN activity_tags at ON at.activity_id = a.id
		WHERE at.tag_id = $1
		  AND a.user_id = $2
		  AND a.has_gps = false
		  AND a.sunrise_at IS NULL
		  AND a.started_at IS NOT NULL
		  AND a.ended_at IS NOT NULL
		  AND a.id = ANY($3)`,
			tagID, userID, activityIDs)
	} else {
		rows, err = h.DB.Pool.Query(ctx, `
		SELECT a.id, a.started_at, a.ended_at
		FROM activities a
		JOIN activity_tags at ON at.activity_id = a.id
		WHERE at.tag_id = $1
		  AND a.user_id = $2
		  AND a.has_gps = false
		  AND a.sunrise_at IS NULL
		  AND a.started_at IS NOT NULL
		  AND a.ended_at IS NOT NULL`,
			tagID, userID)
	}
	if err != nil {
		return err
	}
	defer rows.Close()

	type candidate struct {
		id        string
		startedAt time.Time
		endedAt   time.Time
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.startedAt, &c.endedAt); err == nil {
			candidates = append(candidates, c)
		}
	}
	rows.Close()

	for _, c := range candidates {
		sun := enrichment.CalculateSunTimes(*lat, *lng, *lat, *lng, c.startedAt, c.endedAt, nil)
		periods := enrichment.TimeOfDayPeriods(*lat, *lng, c.startedAt, c.endedAt)

		fields := map[string]any{}
		if sun.SunriseAt != nil {
			fields["sunrise_at"] = *sun.SunriseAt
		}
		if sun.SunsetAt != nil {
			fields["sunset_at"] = *sun.SunsetAt
		}
		if len(periods) > 0 {
			fields["time_of_day"] = periods
		}
		if len(fields) > 0 {
			if err := h.DB.UpdateEnrichmentFields(ctx, c.id, fields); err != nil {
				slog.Warn("location pin sun times update failed", "activity_id", c.id, "error", err)
			}
		}
	}
	return nil
}

// UnassignActivitiesFromTag removes a list of activities from a tag.
// DELETE /api/v1/tags/{id}/activities
// Body: {"activity_ids": ["id1", "id2"]}
func (h *Handler) UnassignActivitiesFromTag(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	tagID := chi.URLParam(r, "id")

	var req struct {
		ActivityIDs []string `json:"activity_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid body", r.URL.Path)
		return
	}
	if len(req.ActivityIDs) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Load tag to check if it's a system tag (for recording removal decision).
	tag, tagErr := h.DB.GetTagByID(r.Context(), tagID, userID)
	if tagErr != nil {
		slog.Error("unassign activities: get tag failed", "tag_id", tagID, "error", tagErr)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load tag", r.URL.Path)
		return
	}
	if tag == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "tag not found", r.URL.Path)
		return
	}

	if err := h.DB.UnassignActivitiesFromTag(r.Context(), tagID, userID, req.ActivityIDs); err != nil {
		slog.Error("unassign activities from tag failed", "tag_id", tagID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to unassign activities", r.URL.Path)
		return
	}

	// If this is a system tag, record the removal so enrichment won't re-add it.
	if tag.IsSystem {
		for _, actID := range req.ActivityIDs {
			if err := h.DB.RecordTagRemoval(r.Context(), actID, tagID); err != nil {
				slog.Warn("record tag removal", "err", err, "activity", actID, "tag", tagID)
			}
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetTagActivities returns a list of activities that have the given tag.
func (h *Handler) GetTagActivities(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	tagID := chi.URLParam(r, "id")

	activities, err := h.DB.GetTagActivities(r.Context(), tagID, userID)
	if err != nil {
		slog.Error("get tag activities failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get tag activities", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, activities)
}

// MergeTag merges a source tag into a target tag.
// POST /api/v1/tags/{id}/merge   body: { "source_tag_id": "uuid" }
// The tag identified by {id} survives; source_tag_id is deleted.
func (h *Handler) MergeTag(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	targetTagID := chi.URLParam(r, "id")

	var req struct {
		SourceTagID string `json:"source_tag_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.SourceTagID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "source_tag_id is required", r.URL.Path)
		return
	}
	if req.SourceTagID == targetTagID {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "source and target tag must differ", r.URL.Path)
		return
	}

	tag, err := h.DB.MergeTag(r.Context(), targetTagID, req.SourceTagID, userID)
	if err != nil {
		errMsg := err.Error()
		if errMsg == "target tag not found" || errMsg == "source tag not found" {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", errMsg, r.URL.Path)
			return
		}
		if errMsg == "source_is_system" || errMsg == "source_has_rule" {
			httputil.WriteJSON(w, http.StatusOK, map[string]any{
				"status": "conflict",
				"reason": errMsg,
			})
			return
		}
		slog.Error("merge tag failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to merge tags", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, tag)
}

// SyncTagLocation sets location_lat/location_lng on all activities linked to this tag,
// using the tag's own lat/lng coordinates. Only acts when the tag has a location set.
// POST /api/v1/tags/{id}/sync-location
func (h *Handler) SyncTagLocation(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	tagID := chi.URLParam(r, "id")

	// Load the tag to get its coordinates.
	tag, err := h.DB.GetTagByID(r.Context(), tagID, userID)
	if err != nil {
		slog.Error("sync tag location: get tag failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load tag", r.URL.Path)
		return
	}
	if tag == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "tag not found", r.URL.Path)
		return
	}
	if tag.Lat == nil || tag.Lng == nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "tag has no location coordinates", r.URL.Path)
		return
	}

	// Fetch all activity IDs linked to this tag.
	activityIDs, err := h.DB.GetTagActivityIDs(r.Context(), tagID)
	if err != nil {
		slog.Error("sync tag location: get activity ids failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get tag activities", r.URL.Path)
		return
	}

	// Update location_lat/location_lng for each linked activity.
	lat := *tag.Lat
	lng := *tag.Lng
	updated := 0
	for _, actID := range activityIDs {
		_, err := h.DB.UpdateActivity(r.Context(), actID, userID, &model.ActivityUpdate{
			LocationLat: &lat,
			LocationLng: &lng,
		})
		if err != nil {
			slog.Warn("sync tag location: update activity failed", "activity_id", actID, "error", err)
			continue
		}
		updated++
	}

	tagIDForBackfill := tagID
	userIDForBackfill := userID
	activityIDsForMeta := activityIDs
	// recompute=true: location coordinates set/changed, re-calculate for all linked activities.
	// Both backfills and .meta refresh run in parallel with a 5-minute timeout.
	go func() {
		ctx2, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		g, gCtx := errgroup.WithContext(ctx2)
		g.Go(func() error {
			if err := h.backfillSunTimesForLocationTag(gCtx, userIDForBackfill, tagIDForBackfill, nil, true); err != nil {
				slog.Warn("sync tag location: sun times backfill failed", "tag_id", tagIDForBackfill, "error", err)
			}
			return nil
		})
		g.Go(func() error {
			if err := h.backfillWeatherForLocationTag(gCtx, userIDForBackfill, tagIDForBackfill, nil, true); err != nil {
				slog.Warn("sync tag location: weather backfill failed", "tag_id", tagIDForBackfill, "error", err)
			}
			return nil
		})
		g.Go(func() error {
			// Refresh .meta companions — location_lat/lng changed.
			for _, actID := range activityIDsForMeta {
				act, _ := h.DB.GetActivity(gCtx, actID)
				if act != nil && act.FileID != nil && *act.FileID != "" {
					sidecar.WriteMetaCompanion(gCtx, h.ShadowFS, h.DB, userIDForBackfill, actID, *act.FileID, nil, act)
				}
			}
			return nil
		})
		_ = g.Wait()
	}()

	httputil.WriteJSON(w, http.StatusOK, map[string]int{"updated_count": updated})
}

// GetTagZone returns the zone for a tag.
// GET /api/v1/tag-zones/{tagId}
func (h *Handler) GetTagZone(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	tagID := chi.URLParam(r, "tagId")

	zone, err := h.DB.GetTagZone(r.Context(), tagID, userID)
	if err != nil {
		slog.Error("get tag zone failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get tag zone", r.URL.Path)
		return
	}
	if zone == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "no zone for tag", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, zone)
}

// UpsertTagZone creates or replaces the zone for a tag.
// PUT /api/v1/tag-zones/{tagId}
func (h *Handler) UpsertTagZone(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	tagID := chi.URLParam(r, "tagId")

	req, ok := httputil.DecodeJSON[model.TagZone](w, r)
	if !ok {
		return
	}
	if req.ZoneType != "circle" && req.ZoneType != "polygon" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "zone_type must be 'circle' or 'polygon'", r.URL.Path)
		return
	}

	zone, err := h.DB.UpsertTagZone(r.Context(), tagID, userID, req)
	if err != nil {
		if err.Error() == "tag not found" {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "tag not found", r.URL.Path)
			return
		}
		if err.Error() == "circle zone requires center and radius_m" {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
			return
		}
		slog.Error("upsert tag zone failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to upsert tag zone", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, zone)
}

// DeleteTagZone removes the zone from a tag.
// DELETE /api/v1/tag-zones/{tagId}
func (h *Handler) DeleteTagZone(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	tagID := chi.URLParam(r, "tagId")

	if err := h.DB.DeleteTagZone(r.Context(), tagID, userID); err != nil {
		if err.Error() == "tag zone not found" {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "tag zone not found", r.URL.Path)
			return
		}
		slog.Error("delete tag zone failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete tag zone", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteOrphanTag removes an orphan tag name (not in tags table) from all activities.tags TEXT[].
// DELETE /api/v1/tags/orphan   body: {"name": "tagname"}
func (h *Handler) DeleteOrphanTag(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.Name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}

	affected, err := h.DB.DeleteOrphanTag(r.Context(), userID, req.Name)
	if err != nil {
		slog.Error("delete orphan tag failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete orphan tag", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]int64{"affected": affected})
}

// SeedDefaultTags creates the default autotag definitions if they don't exist.
func (h *Handler) SeedDefaultTags(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	if err := h.DB.SeedSystemDefaults(r.Context(), userID); err != nil {
		slog.Error("seed default tags failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to seed default tags", r.URL.Path)
		return
	}

	tags, err := h.DB.ListTags(r.Context(), userID)
	if err != nil {
		slog.Error("list tags after seed failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list tags", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, tags)
}
