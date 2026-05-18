package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/paulmach/orb"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

var validSegmentTypes = map[string]struct{}{
	"standard":    {},
	"trail":       {},
	"lap":         {},
	"wintersport": {},
	"wassersport": {},
	"lift":        {},
}

var enrichableSegmentTypes = map[string]struct{}{
	"standard":    {},
	"trail":       {},
	"lap":         {},
	"wintersport": {},
	"wassersport": {},
}

func (h *Handler) ListSegments(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if l == "-1" {
			limit = -1
		} else if parsed, err := strconv.Atoi(l); err == nil && parsed >= 0 {
			limit = parsed
		}
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		name = r.URL.Query().Get("search")
	}

	params := model.ListSegmentsParams{
		UserID:           userID,
		IsAdmin:          isAdmin,
		SportGroup:       r.URL.Query().Get("sport_group"),
		SegmentType:      r.URL.Query().Get("segment_type"),
		SegmentTypes:     r.URL.Query().Get("segment_types"),
		Name:             name,
		BBox:             r.URL.Query().Get("bbox"),
		LiftsOnlyRidden:  r.URL.Query().Get("lifts_only_ridden") == "true",
		CompletedOnly:    r.URL.Query().Get("completed_only") == "true",
		IncludeGeoJSON:   r.URL.Query().Get("include_geojson") == "true",
		Cursor:           r.URL.Query().Get("cursor"),
		Limit:            limit,
		SacScale:         r.URL.Query().Get("sac_scale"),
		MtbScale:         r.URL.Query().Get("mtb_scale"),
		Surface:          r.URL.Query().Get("surface"),
		ElevationKind:    r.URL.Query().Get("elevation_kind"),
		VisibilityFilter: r.URL.Query().Get("visibility"),
		DifficultyMatch:  r.URL.Query().Get("difficulty_match"),
	}

	// Input-Validation (§5.3 Spec N2-6) — verhindert DoS via malformed Query
	if params.SegmentTypes != "" {
		for _, t := range strings.Split(params.SegmentTypes, ",") {
			if !isValidSegmentType(strings.TrimSpace(t)) {
				httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid segment_types parameter", r.URL.Path)
				return
			}
		}
	}

	if params.SacScale != "" && !database.IsValidSacList(params.SacScale) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid sac_scale parameter (expect T1-T6)", r.URL.Path)
		return
	}
	if params.MtbScale != "" && !database.IsValidMtbList(params.MtbScale) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid mtb_scale parameter (expect S0-S5)", r.URL.Path)
		return
	}
	if params.Surface != "" && !database.IsValidSurfaceList(params.Surface) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid surface parameter", r.URL.Path)
		return
	}
	if params.ElevationKind != "" {
		if err := validateRequestedSegmentElevationKind(&params.ElevationKind); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid elevation_kind parameter", r.URL.Path)
			return
		}
	}
	if params.VisibilityFilter != "" {
		// Dedup + clamp: build a set of whitelisted tokens, drop empties/whitespace,
		// write deterministically sorted result back to params so the DB layer
		// always sees at most len=2 and never a stale toggle fail-open (#15, A2).
		seen := make(map[string]struct{}, 2)
		for _, v := range strings.Split(params.VisibilityFilter, ",") {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			if v != "public" && v != "private" {
				httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid visibility parameter (expect public, private, or public,private)", r.URL.Path)
				return
			}
			seen[v] = struct{}{}
		}
		// Reconstruct deterministic order: public before private.
		deduped := make([]string, 0, 2)
		for _, k := range []string{"public", "private"} {
			if _, ok := seen[k]; ok {
				deduped = append(deduped, k)
			}
		}
		params.VisibilityFilter = strings.Join(deduped, ",")
	}
	if params.DifficultyMatch != "" && params.DifficultyMatch != "any" && params.DifficultyMatch != "all" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid difficulty_match parameter (expect any or all)", r.URL.Path)
		return
	}

	result, err := h.DB.ListSegments(r.Context(), params)

	if err != nil {
		slog.Error("list segments failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list segments", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) CreateSegment(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	create, ok := httputil.DecodeJSON[model.SegmentCreate](w, r)
	if !ok {
		return
	}
	if create.Name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if create.GeoJSONTrack == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "geojson_track is required", r.URL.Path)
		return
	}
	if create.SportGroup == "" {
		create.SportGroup = "generic"
	}
	if create.SegmentType == "" {
		create.SegmentType = "standard"
	}
	if !isValidSegmentType(create.SegmentType) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid segment_type", r.URL.Path)
		return
	}
	if create.TerrainType == "" {
		create.TerrainType = "standard"
	}
	if create.ClimbType == "" {
		create.ClimbType = "standard"
	}
	// Lift segments always get bergfahrt as default climb_type
	if create.TerrainType == "lift" && create.ClimbType == "standard" {
		create.ClimbType = "bergfahrt"
	}
	elevationKind, err := normalizeSegmentElevationKindForCreate(create.SegmentType, create.ClimbType, create.ElevationKind)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid elevation_kind", r.URL.Path)
		return
	}
	create.ElevationKind = &elevationKind
	if create.SegmentType == "lap" {
		create.ClimbType = "standard"
	}

	if create.Visibility == "" {
		create.Visibility = "public"
	} else if create.Visibility != "public" && create.Visibility != "private" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "visibility must be 'public' or 'private'", r.URL.Path)
		return
	}

	var sourceActivity *model.Activity
	if create.SourceActivityID != nil {
		var err error
		sourceActivity, err = h.DB.GetActivity(r.Context(), *create.SourceActivityID)
		if err != nil {
			slog.Error("create segment: load source activity failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load source activity", r.URL.Path)
			return
		}
		if sourceActivity == nil || sourceActivity.UserID != userID {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "source activity not found", r.URL.Path)
			return
		}
	}

	// Normalize historic German labels in segment_type_meta before DB write.
	if len(create.SegmentTypeMeta) > 0 {
		var m map[string]any
		if err := json.Unmarshal(create.SegmentTypeMeta, &m); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "segment_type_meta: invalid JSON", r.URL.Path)
			return
		}
		if sv, ok := m["surface"].(string); ok {
			m["surface"] = normalizeSurfaceLabel(sv)
		}
		if dv, ok := m["difficulty_color"].(string); ok {
			m["difficulty_color"] = normalizeDifficultyColor(dv)
		}
		stripIncompatibleMeta(m, create.SegmentType)
		buf, err := json.Marshal(m)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "re-marshal meta", r.URL.Path)
			return
		}
		create.SegmentTypeMeta = buf
	}

	segment, err := h.DB.CreateSegment(r.Context(), userID, &create)
	if err != nil {
		if errors.Is(err, database.ErrSegmentValidation) {
			httputil.WriteError(w, http.StatusUnprocessableEntity, "Validation Error", err.Error(), r.URL.Path)
			return
		}
		if strings.Contains(err.Error(), "lap segment must be a closed loop") {
			httputil.WriteError(w, http.StatusUnprocessableEntity, "Validation Error", err.Error(), r.URL.Path)
			return
		}
		slog.Error("create segment failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create segment", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, segment)

	// Backfill: match new segment against all existing activities asynchronously.
	// For private segments, backfill only runs against creator's own activities.
	// GetSpatialCandidateSegments enforces: s.visibility = 'public' OR s.created_by = userID
	segID := segment.ID
	shutdown.Go("segment-backfill-"+segID, func(ctx context.Context) {
		h.backfillSegmentEfforts(ctx, userID, segID, sourceActivity)
	})
}

func (h *Handler) matchSourceActivityForSegment(ctx context.Context, userID, segmentID string, act *model.Activity) {
	matchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	subType := ""
	if act.SubType != nil {
		subType = *act.SubType
	}
	before := time.Now()
	if err := h.recomputeSegmentEffortsForActivity(matchCtx, segmentID, act.ID, userID); err != nil {
		slog.Warn("create segment: source activity match failed",
			"segment_id", segmentID,
			"activity_id", act.ID,
			"sport_type", act.SportType,
			"sub_type", subType,
			"error", err)
	}
	slog.Info("create segment: source activity match attempted",
		"segment_id", segmentID,
		"activity_id", act.ID,
		"duration_ms", time.Since(before).Milliseconds())
}

func (h *Handler) backfillSegmentEfforts(ctx context.Context, userID, segmentID string, sourceActivity *model.Activity) {
	if sourceActivity != nil {
		h.matchSourceActivityForSegment(ctx, userID, segmentID, sourceActivity)
	}

	ids, err := h.DB.ListActivityIDsForSegmentBackfill(ctx, userID, segmentID)
	if err != nil {
		slog.Error("segment backfill: list activities failed",
			"segment_id", segmentID, "error", err)
		return
	}
	slog.Info("segment backfill: starting",
		"segment_id", segmentID, "activity_count", len(ids))
	for _, actID := range ids {
		if sourceActivity != nil && actID == sourceActivity.ID {
			continue
		}
		if err := h.recomputeSegmentEffortsForActivity(ctx, segmentID, actID, userID); err != nil {
			slog.Warn("segment backfill: activity match failed",
				"segment_id", segmentID,
				"activity_id", actID,
				"error", err)
		}
	}
	if err := h.DB.UpdateSegmentEffortRanks(ctx, segmentID); err != nil {
		slog.Warn("segment backfill: final rank update failed", "segment_id", segmentID, "error", err)
	}
	slog.Info("segment backfill: done", "segment_id", segmentID)
}

func (h *Handler) GetSegment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)

	includeGeoJSON := r.URL.Query().Get("include_geojson") == "true"

	segment, err := h.DB.GetSegment(r.Context(), id, userID, isAdmin, includeGeoJSON)

	if err != nil {
		slog.Error("get segment failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment", r.URL.Path)
		return
	}
	if segment == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, segment)
}

// stripIncompatibleMeta removes segment_type_meta keys that don't apply to the
// given segType. Wassersport/Wintersport/Lift have implicit surface
// (water/snow/infrastructure) and no ground-based difficulty scales — frontend
// already hides these fields, this is backend defense-in-depth so hand-crafted
// API payloads (or legacy data still in the map) don't persist orphaned values
// that the filter queries would still match.
func stripIncompatibleMeta(m map[string]any, segType string) {
	switch segType {
	case "wassersport", "wintersport", "lift":
		delete(m, "surface")
		delete(m, "sac_scale")
		delete(m, "mtb_scale")
	}
}

func isValidSegmentType(segType string) bool {
	_, ok := validSegmentTypes[segType]
	return ok
}

func isEnrichableSegmentType(segType string) bool {
	_, ok := enrichableSegmentTypes[segType]
	return ok
}

func validateRequestedSegmentElevationKind(requested *string) error {
	if requested == nil {
		return nil
	}
	switch *requested {
	case "flat", "hilly", "uphill", "downhill":
		return nil
	default:
		return fmt.Errorf("invalid elevation_kind")
	}
}

func legacySegmentElevationKind(climbType string) string {
	switch climbType {
	case "bergauf", "bergfahrt":
		return "uphill"
	case "bergab", "talfahrt":
		return "downhill"
	default:
		return "flat"
	}
}

func normalizeSegmentElevationKindForCreate(segmentType, climbType string, requested *string) (string, error) {
	if err := validateRequestedSegmentElevationKind(requested); err != nil {
		return "", err
	}
	if segmentType == "lap" {
		return "flat", nil
	}
	if requested != nil {
		return *requested, nil
	}
	return legacySegmentElevationKind(climbType), nil
}

func normalizeSegmentElevationKindForUpdate(
	stored string,
	segmentType string,
	climbType string,
	requested *string,
	climbTypeChanged bool,
	segmentTypeChanged bool,
) (string, error) {
	if err := validateRequestedSegmentElevationKind(requested); err != nil {
		return "", err
	}
	if segmentType == "lap" {
		return "flat", nil
	}
	if requested != nil {
		return *requested, nil
	}
	if climbTypeChanged || segmentTypeChanged {
		return legacySegmentElevationKind(climbType), nil
	}
	return stored, nil
}

func (h *Handler) UpdateSegment(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)

	// Load existing segment: pass userID so own private segments are visible,
	// and isAdmin so admins can edit any segment they can read.
	existing, err := h.DB.GetSegment(r.Context(), id, userID, isAdmin, false)

	if err != nil {
		slog.Error("update segment: get segment failed", "id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load segment", r.URL.Path)
		return
	}
	if existing == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}
	if existing.CreatedBy != userID && !isAdmin {
		httputil.WriteError(w, http.StatusForbidden, "Forbidden", "only the owner can edit this segment", r.URL.Path)
		return
	}

	update, ok := httputil.DecodeJSON[model.SegmentUpdate](w, r)
	if !ok {
		return
	}

	// Validate visibility value (clean 400 instead of DB CHECK constraint 500).
	if update.Visibility != nil && *update.Visibility != "public" && *update.Visibility != "private" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "visibility must be 'public' or 'private'", r.URL.Path)
		return
	}

	// Block private→public via UpdateSegment — must use POST /segments/{id}/publish endpoint
	// which triggers a full recompute for all users' activities.
	if update.Visibility != nil && *update.Visibility == "public" && existing.Visibility == "private" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "use POST /segments/{id}/publish to publish a private segment", r.URL.Path)
		return
	}

	// Validate segment_type if set — restrict to canonical enum values.
	// (The frontend segment-type picker sends this; without validation the DB
	// CHECK constraint would yield a 500 instead of a clean 400.)
	if update.SegmentType != nil {
		if !isValidSegmentType(*update.SegmentType) {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid segment_type", r.URL.Path)
			return
		}
	}

	// Effective segment_type after this update — used to strip incompatible meta.
	effectiveSegType := existing.SegmentType
	if update.SegmentType != nil {
		effectiveSegType = *update.SegmentType
	}
	effectiveClimbType := "standard"
	if existing.ClimbType != nil && *existing.ClimbType != "" {
		effectiveClimbType = *existing.ClimbType
	}
	if update.ClimbType != nil {
		effectiveClimbType = *update.ClimbType
	}
	elevationKind, err := normalizeSegmentElevationKindForUpdate(
		existing.ElevationKind,
		effectiveSegType,
		effectiveClimbType,
		update.ElevationKind,
		update.ClimbType != nil,
		update.SegmentType != nil,
	)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid elevation_kind", r.URL.Path)
		return
	}
	if update.ElevationKind != nil || update.ClimbType != nil || update.SegmentType != nil || effectiveSegType == "lap" {
		update.ElevationKind = &elevationKind
	}
	if effectiveSegType == "lap" {
		standardClimb := "standard"
		update.ClimbType = &standardClimb
	}

	// Normalize historic German labels in segment_type_meta before DB write.
	if update.SegmentTypeMeta != nil && len(*update.SegmentTypeMeta) > 0 {
		var m map[string]any
		if err := json.Unmarshal(*update.SegmentTypeMeta, &m); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "segment_type_meta: invalid JSON", r.URL.Path)
			return
		}
		if sv, ok := m["surface"].(string); ok {
			m["surface"] = normalizeSurfaceLabel(sv)
		}
		if dv, ok := m["difficulty_color"].(string); ok {
			m["difficulty_color"] = normalizeDifficultyColor(dv)
		}
		stripIncompatibleMeta(m, effectiveSegType)
		buf, err := json.Marshal(m)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "re-marshal meta", r.URL.Path)
			return
		}
		*update.SegmentTypeMeta = buf
	}

	segment, err := h.DB.UpdateSegment(r.Context(), id, userID, isAdmin, &update)
	if err != nil {
		if errors.Is(err, database.ErrSegmentValidation) {
			httputil.WriteError(w, http.StatusUnprocessableEntity, "Validation Error", err.Error(), r.URL.Path)
			return
		}
		slog.Error("update segment failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update segment", r.URL.Path)
		return
	}
	if segment == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	// If switching public→private: recompute counts so effort_count/athlete_count
	// no longer include other users' efforts. Track as background_job (ActiveJobsIndicator).
	if update.Visibility != nil && *update.Visibility == "private" && existing.Visibility == "public" {
		entityName := existing.Name
		jID, _ := h.DB.InsertBackgroundJob(r.Context(), userID, "segment_recompute", &id, &entityName, 1)
		shutdown.Go("segment-to-private-"+id, func(ctx context.Context) {
			failed := false
			if err := h.DB.UpdateSegmentEffortRanks(ctx, id); err != nil {
				slog.Warn("update segment: recompute after public→private failed", "segment_id", id, "error", err)
				failed = true
			}
			if jID != "" {
				h.DB.CompleteBackgroundJob(ctx, jID, failed, "")
			}
		})
	}

	httputil.WriteJSON(w, http.StatusOK, segment)
}

// DeleteSegments bulk-deletes multiple segments owned by the authenticated user.
//
// DELETE /api/v1/segments
// Body: {"ids": ["uuid1", "uuid2", ...]}
// OSM-imported lift segments are silently skipped (not counted in the response).
func (h *Handler) DeleteSegments(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	body, ok := httputil.DecodeJSON[struct {
		IDs []string `json:"ids"`
	}](w, r)
	if !ok {
		return
	}
	if len(body.IDs) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "ids array required", r.URL.Path)
		return
	}
	if len(body.IDs) > 500 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "max 500 ids per request", r.URL.Path)
		return
	}

	count, err := h.DB.DeleteSegments(r.Context(), body.IDs, userID)
	if err != nil {
		slog.Error("bulk delete segments failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete segments", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]int64{"deleted": count})
}

func (h *Handler) DeleteSegment(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)

	if err := h.DB.DeleteSegment(r.Context(), userID, id, isAdmin); err != nil {
		if errors.Is(err, database.ErrSegmentOSMLift) {
			httputil.WriteError(w, http.StatusForbidden, "Forbidden", "OSM-imported lift segments cannot be deleted", r.URL.Path)
			return
		}
		// Return 404 for both not-found and not-authorized — prevents segment existence oracle.
		if errors.Is(err, database.ErrSegmentNotFound) || errors.Is(err, database.ErrSegmentNotAuthorized) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
			return
		}
		slog.Error("delete segment failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete segment", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetSegmentLeaderboard(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)

	// Verify segment is visible to the requesting user (prevents private segment data leak via UUID).
	seg, err := h.DB.GetSegment(r.Context(), id, userID, isAdmin, false)
	if err != nil {
		slog.Error("get segment leaderboard: get segment failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment", r.URL.Path)
		return
	}
	if seg == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	sportType := r.URL.Query().Get("sport_type")
	gearID := r.URL.Query().Get("gear_id")

	var isReverse *bool
	if v := r.URL.Query().Get("is_reverse"); v != "" {
		b := v == "true"
		isReverse = &b
	}

	// "only_mine=true" shows the current user's own efforts, including from private activities.
	var onlyUserID string
	if r.URL.Query().Get("only_mine") == "true" {
		onlyUserID = userID
	}

	efforts, err := h.DB.GetSegmentLeaderboard(r.Context(), id, sportType, isReverse, gearID, onlyUserID)
	if err != nil {
		slog.Error("get segment leaderboard failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get leaderboard", r.URL.Path)
		return
	}

	// Strip health-sensitive fields from other users' entries (DSGVO Art. 9:
	// avg_heart_rate and avg_power_w are personal health data).
	for i := range efforts {
		if efforts[i].UserID != userID {
			efforts[i].AvgPowerW = nil
			efforts[i].AvgHeartRate = nil
			efforts[i].AvgSpeedMs = nil
		}
	}

	httputil.WriteJSON(w, http.StatusOK, efforts)
}

func (h *Handler) GetActivitySegmentEfforts(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	activity, err := h.DB.GetActivity(r.Context(), id)
	if err != nil {
		slog.Error("get activity segment efforts: get activity failed", "id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	efforts, err := h.DB.GetActivitySegmentEfforts(r.Context(), id)
	if err != nil {
		slog.Error("get activity segment efforts failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment efforts", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, efforts)
}

// recomputeSegmentEfforts deletes all existing segment_efforts for the given
// segment/user pair and re-matches every eligible activity against it.
// It is the library-level core used by both RecomputeSegment (HTTP-triggered,
// with job tracking) and RecomputeEffortsForActivity (post-fusion, non-fatal).
//
// The caller is responsible for running this inside a shutdown.Go goroutine
// or other long-running context when appropriate.
func (h *Handler) recomputeSegmentEfforts(ctx context.Context, segmentID, userID string) error {
	ids, err := h.DB.ListActivityIDsForSegmentRecompute(ctx, userID, segmentID)
	if err != nil {
		return fmt.Errorf("list activities for recompute: %w", err)
	}

	slog.Info("segment recompute: starting", "segment_id", segmentID, "activity_count", len(ids))
	for _, actID := range ids {
		if err := h.recomputeSegmentEffortsForActivity(ctx, segmentID, actID, userID); err != nil {
			slog.Warn("segment recompute: activity match failed",
				"segment_id", segmentID,
				"activity_id", actID,
				"error", err)
		}
	}
	if err := h.DB.UpdateSegmentEffortRanks(ctx, segmentID); err != nil {
		slog.Warn("segment recompute: final rank update failed", "segment_id", segmentID, "error", err)
	}
	slog.Info("segment recompute: done", "segment_id", segmentID)
	return nil
}

// recomputeSegmentEffortsForActivity deletes and re-matches segment_efforts
// only for a specific (segment_id, activity_id) tuple. Used by
// RecomputeEffortsForActivity (post-fusion cleanup) to avoid the user-wide
// DELETE that the general recomputeSegmentEfforts handler does.
//
// The DELETE and INSERT are wrapped in a single transaction so that a matching
// failure never leaves the pair with zero efforts (R2-10): if the INSERT fails,
// the DELETE is rolled back and the pre-recompute efforts are preserved.
//
// Rank updates happen outside the transaction because they are non-critical and
// idempotent — a partial rank failure does not corrupt effort data.
//
// The caller is responsible for running this inside a shutdown.Go goroutine
// or other long-running context when appropriate.
func (h *Handler) recomputeSegmentEffortsForActivity(ctx context.Context, segmentID, activityID, userID string) error {
	// Load the activity to get sport metadata required by collectSegmentEfforts.
	act, err := h.DB.GetActivity(ctx, activityID)
	if err != nil {
		return fmt.Errorf("load activity for segment rematch: %w", err)
	}
	if act == nil {
		return fmt.Errorf("activity %s not found for segment rematch", activityID)
	}

	subType := ""
	if act.SubType != nil {
		subType = *act.SubType
	}

	// Collect new efforts in-memory before opening the transaction — all reads
	// and pure computation with no side effects.
	// collectSegmentEfforts applies the same spatial pre-filter as the full
	// recompute path (GetSpatialCandidateSegments) so only the relevant segment
	// will produce efforts for this (activity_id, segment_id) pair.
	allEfforts, matchedSegmentIDs, _, _, err := collectSegmentEfforts(ctx, h.DB, activityID, userID, act.SportType, subType, act.GearID, act.StartedAt)
	if err != nil {
		return fmt.Errorf("collect new efforts for segment rematch: %w", err)
	}

	// Filter to only the target segmentID.
	//
	// collectSegmentEfforts may return efforts for all segments that spatially
	// overlap the activity's bbox, not just the requested one.  When the caller
	// loops over N overlapping segments and calls this function for each, the
	// DELETE targets only (segment_id, activity_id) — but inserting the full
	// allEfforts slice would re-insert efforts for segments that were not DELETEd,
	// silently duplicating them.  Primary key uuid-default does not catch this.
	// Filter here to make the function semantics honest: recompute exactly the
	// (segmentID, activityID) tuple.  (R3-1)
	efforts := make([]*model.SegmentEffort, 0, len(allEfforts))
	for _, e := range allEfforts {
		if e.SegmentID == segmentID {
			efforts = append(efforts, e)
		}
	}

	// Transactional DELETE + INSERT: if the INSERT fails the DELETE is rolled back,
	// preserving the pre-recompute state (fixes R2-10 non-atomic delete+match).
	tx, err := h.beginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx for segment rematch: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := database.LockSegmentEffortReplacementTx(ctx, tx, activityID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM segment_efforts WHERE segment_id = $1 AND activity_id = $2`,
		segmentID, activityID,
	); err != nil {
		return fmt.Errorf("delete efforts for segment+activity: %w", err)
	}

	if err := database.InsertSegmentEffortsBatchTx(ctx, tx, efforts); err != nil {
		return fmt.Errorf("insert new efforts for segment+activity: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit segment rematch tx: %w", err)
	}

	// Update ranks outside the transaction — non-critical, idempotent.
	for segID := range matchedSegmentIDs {
		if err := h.DB.UpdateSegmentEffortRanks(ctx, segID); err != nil {
			slog.Warn("segment recompute: update ranks failed",
				"segment_id", segID, "activity_id", activityID, "error", err)
		}
	}
	return nil
}

// RecomputeEffortsForActivity re-matches a single activity against all
// spatially overlapping segments in one pass. Used by postCommitFusionCleanup
// after a fusion-merge.
//
// Complexity:
//
//	Previously O(N²): collectSegmentEfforts was called once per overlapping
//	segment (N calls, each loading the full GPS stream and running the spatial
//	pre-filter).
//
//	Now O(N): collectSegmentEfforts is called exactly once for the activity.
//	The returned efforts are grouped by segment_id in-memory, then each
//	segment gets a dedicated transactional DELETE + batch-INSERT of only its
//	slice of efforts.
//
// Table: activity_tracks.bbox (GEOMETRY Polygon 4326, Mig 000004).
// segment_versions.track (GEOMETRY LineString 4326, Mig 000032 GIST-indexed).
//
// Per-segment failures are logged as warnings and do not abort the loop —
// partial coverage is acceptable since the user can trigger a manual recompute.
func (h *Handler) RecomputeEffortsForActivity(ctx context.Context, activityID, userID string) error {
	// 1. Load activity metadata required by collectSegmentEfforts.
	act, err := h.DB.GetActivity(ctx, activityID)
	if err != nil {
		return fmt.Errorf("load activity for segment rematch: %w", err)
	}
	if act == nil {
		return fmt.Errorf("activity %s not found for segment rematch", activityID)
	}
	subType := ""
	if act.SubType != nil {
		subType = *act.SubType
	}

	// 2. Collect all efforts for this activity in one pass (GPS stream loaded once).
	allEfforts, _, candidateSegmentIDs, _, err := collectSegmentEfforts(ctx, h.DB, activityID, userID, act.SportType, subType, act.GearID, act.StartedAt)
	if err != nil {
		return fmt.Errorf("collect efforts for activity rematch: %w", err)
	}

	// 3. Group efforts by segment_id for O(1) lookup per segment below.
	effortsBySegment := make(map[string][]*model.SegmentEffort, len(allEfforts))
	for _, e := range allEfforts {
		effortsBySegment[e.SegmentID] = append(effortsBySegment[e.SegmentID], e)
	}

	// 4. Recompute only segments that passed the same active-version candidate
	// selector as matching, plus existing platform efforts whose version was
	// valid for this activity time. Raw segment_versions overlap would delete
	// efforts for expired or future versions that are not valid for the activity.
	existingRows, err := h.DB.Pool.Query(ctx, `
		SELECT DISTINCT se.segment_id
		FROM segment_efforts se
		JOIN segment_versions sv ON sv.id = se.segment_version_id
		WHERE se.activity_id = $1
		  AND se.user_id = $2
		  AND se.match_source_type = 'platform_activity'
		  AND se.matching_contract_key = 'course_gps_v1'
		  AND se.matching_contract_version = 1
		  AND sv.matching_contract_key = 'course_gps_v1'
		  AND sv.matching_contract_version = 1
		  AND (sv.valid_from_at IS NULL OR sv.valid_from_at <= $3)
		  AND (sv.valid_until_at IS NULL OR $3 < sv.valid_until_at)
	`, activityID, userID, act.StartedAt)
	if err != nil {
		return fmt.Errorf("query existing active segment efforts: %w", err)
	}
	for existingRows.Next() {
		var segID string
		if err := existingRows.Scan(&segID); err != nil {
			existingRows.Close()
			return fmt.Errorf("scan existing active segment effort: %w", err)
		}
		candidateSegmentIDs[segID] = struct{}{}
	}
	if err := existingRows.Err(); err != nil {
		existingRows.Close()
		return fmt.Errorf("iterate existing active segment efforts: %w", err)
	}
	existingRows.Close()

	segmentIDs := make([]string, 0, len(candidateSegmentIDs))
	for segID := range candidateSegmentIDs {
		segmentIDs = append(segmentIDs, segID)
	}

	// 5. Per segment: transactional DELETE + batch-INSERT of only that segment's efforts.
	// Rank updates happen outside the transaction (non-critical, idempotent).
	for _, segID := range segmentIDs {
		efforts := effortsBySegment[segID] // nil/empty when no GPS match for this segment

		tx, err := h.beginTx(ctx)
		if err != nil {
			slog.Warn("post-fusion segment recompute: begin tx failed",
				"activity_id", activityID,
				"segment_id", segID,
				"err", err)
			continue
		}

		if _, err := tx.Exec(ctx,
			`DELETE FROM segment_efforts WHERE segment_id = $1 AND activity_id = $2`,
			segID, activityID,
		); err != nil {
			tx.Rollback(ctx) //nolint:errcheck
			slog.Warn("post-fusion segment recompute: delete failed",
				"activity_id", activityID,
				"segment_id", segID,
				"err", err)
			continue
		}

		if err := database.InsertSegmentEffortsBatchTx(ctx, tx, efforts); err != nil {
			tx.Rollback(ctx) //nolint:errcheck
			slog.Warn("post-fusion segment recompute: insert failed",
				"activity_id", activityID,
				"segment_id", segID,
				"err", err)
			continue
		}

		if err := tx.Commit(ctx); err != nil {
			slog.Warn("post-fusion segment recompute: commit failed",
				"activity_id", activityID,
				"segment_id", segID,
				"err", err)
			continue
		}

		if err := h.DB.UpdateSegmentEffortRanks(ctx, segID); err != nil {
			slog.Warn("post-fusion segment recompute: update ranks failed",
				"activity_id", activityID,
				"segment_id", segID,
				"err", err)
		}
	}
	return nil
}

// RecomputeSegment re-matches all of the user's GPS activities against a single segment.
// Useful after editing a segment or when matching failed due to a bug.
func (h *Handler) RecomputeSegment(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)
	if !h.requireFeatureAvailable(w, r, capabilities.KeySegmentRecompute) {
		return
	}

	seg, err := h.DB.GetSegment(r.Context(), id, userID, isAdmin, false)
	if err != nil {
		slog.Error("segment recompute: get segment failed", "segment_id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment", r.URL.Path)
		return
	}
	if seg == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	// Aktivitäts-IDs vorab ermitteln (brauchen wir für progress_total).
	ids, err := h.DB.ListActivityIDsForSegmentRecompute(r.Context(), userID, id)
	if err != nil {
		slog.Error("segment recompute: list activities failed", "segment_id", id, "user_id", userID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
		return
	}

	// Vorherige laufende Jobs für dieses Segment abbrechen.
	if err := h.DB.CancelRunningJobsForEntity(r.Context(), userID, id); err != nil {
		slog.Warn("segment recompute: cancel previous jobs failed", "segment_id", id, "error", err)
	}

	// Job in DB anlegen.
	entityName := seg.Name
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "segment_recompute", &id, &entityName, int64(len(ids)))
	if err != nil {
		slog.Warn("segment recompute: failed to insert job", "segment_id", id, "error", err)
		jobID = ""
	}

	shutdown.Go("segment-recompute-"+id, func(ctx context.Context) {
		failed := false

		if !failed {
			slog.Info("segment recompute: starting", "segment_id", id, "activity_count", len(ids))
			for i, actID := range ids {
				if err := h.recomputeSegmentEffortsForActivity(ctx, id, actID, userID); err != nil {
					slog.Warn("segment recompute: activity match failed",
						"segment_id", id,
						"activity_id", actID,
						"error", err)
				}

				// Fortschritt alle 10 Aktivitäten oder am Ende aktualisieren.
				if jobID != "" && (i%10 == 0 || i == len(ids)-1) {
					if err := h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(i+1)); err != nil {
						slog.Warn("segment recompute: update progress failed", "job_id", jobID, "error", err)
					}
				}
			}
			slog.Info("segment recompute: done", "segment_id", id)
		}

		if err := h.DB.UpdateSegmentEffortRanks(ctx, id); err != nil {
			slog.Warn("segment recompute: final rank update failed", "segment_id", id, "error", err)
		}

		if jobID != "" {
			if err := h.DB.CompleteBackgroundJob(ctx, jobID, failed, ""); err != nil {
				slog.Warn("segment recompute: complete job failed", "job_id", jobID, "error", err)
			}
		}
	})

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"status": "recompute started",
		"job_id": jobID,
	})
}

// PublishSegment transitions a private segment to public and triggers a full recompute
// so all users' activities are matched. Uses background_job tracking (visible in ActiveJobsIndicator).
func (h *Handler) PublishSegment(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)
	if !h.requireFeatureAvailable(w, r, capabilities.KeySegmentPublish) {
		return
	}

	// Get segment to verify ownership and retrieve name for job display.
	// Admins may publish any private segment they can read.
	seg, err := h.DB.GetSegment(r.Context(), id, userID, isAdmin, false)

	if err != nil {
		slog.Error("publish segment: get segment failed", "segment_id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment", r.URL.Path)
		return
	}
	if seg == nil || (seg.CreatedBy != userID && !isAdmin) {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found or not owned", r.URL.Path)
		return
	}
	if seg.Visibility != "private" {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "already_public"})
		return
	}

	// List all nearby activities (all users) before publishing, so we know the total.
	ids, err := h.DB.ListAllActivityIDsForSegmentRecompute(r.Context(), id)
	if err != nil {
		slog.Error("publish segment: list activities failed", "segment_id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
		return
	}

	// Set visibility = 'public'. After this point the segment is visible to all users.
	published, err := h.DB.PublishSegment(r.Context(), id, userID, isAdmin)
	if err != nil {
		slog.Error("publish segment failed", "segment_id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to publish segment", r.URL.Path)
		return
	}
	if !published {
		// Race: another request already published it between our check and this call.
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "already_public"})
		return
	}

	// Cancel any in-progress recompute jobs for this segment to prevent races (A8).
	// Must run BEFORE InsertBackgroundJob — otherwise it cancels the job we just created.
	h.DB.CancelRunningJobsForEntity(r.Context(), userID, id)

	// Track the recompute as a background job (visible in ActiveJobsIndicator).
	entityName := seg.Name
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "segment_publish", &id, &entityName, int64(len(ids)))
	if err != nil {
		slog.Warn("publish segment: failed to insert job", "segment_id", id, "error", err)
		jobID = ""
	}

	shutdown.Go("segment-publish-"+id, func(ctx context.Context) {
		failed := false

		if !failed {
			for i, actID := range ids {
				act, err := h.DB.GetActivity(ctx, actID)
				if err != nil || act == nil {
					continue
				}
				if err := h.recomputeSegmentEffortsForActivity(ctx, id, actID, act.UserID); err != nil {
					slog.Warn("publish recompute: activity match failed",
						"segment_id", id,
						"activity_id", actID,
						"error", err)
				}

				if jobID != "" && (i%10 == 0 || i == len(ids)-1) {
					if err := h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(i+1)); err != nil {
						slog.Warn("publish recompute: update progress failed", "job_id", jobID, "error", err)
					}
				}
			}
			slog.Info("segment publish recompute: done", "segment_id", id, "activity_count", len(ids))
		}

		// Final rank/count update — ensures correct values even if zero activities matched.
		if err := h.DB.UpdateSegmentEffortRanks(ctx, id); err != nil {
			slog.Warn("publish recompute: final rank update failed", "segment_id", id, "error", err)
		}

		if jobID != "" {
			if err := h.DB.CompleteBackgroundJob(ctx, jobID, failed, ""); err != nil {
				slog.Warn("publish recompute: complete job failed", "job_id", jobID, "error", err)
			}
		}
	})

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"status": "publish started",
		"job_id": jobID,
	})
}

func (h *Handler) GetActivityDetectedLaps(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	activity, err := h.DB.GetActivity(r.Context(), id)
	if err != nil {
		slog.Error("get detected laps: get activity failed", "id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	laps, err := h.DB.GetDetectedLaps(r.Context(), id)
	if err != nil {
		slog.Error("get detected laps failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get detected laps", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, laps)
}

// GetSegmentFavoriteStatus checks if a segment is favorited by the user.
func (h *Handler) GetSegmentFavoriteStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	segmentID := chi.URLParam(r, "id")
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)

	seg, err := h.DB.GetSegment(r.Context(), segmentID, userID, isAdmin, false)
	if err != nil {
		slog.Error("segment access check failed", "handler", "GetSegmentFavoriteStatus", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment", r.URL.Path)
		return
	}
	if seg == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	isFavorite, err := h.DB.IsSegmentFavoriteByUser(r.Context(), userID, segmentID)
	if err != nil {
		slog.Error("check favorite status failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to check favorite status", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"favorite": isFavorite})
}

// AddSegmentFavorite adds a segment to the user's favorites.
func (h *Handler) AddSegmentFavorite(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	segmentID := chi.URLParam(r, "id")
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)

	seg, err := h.DB.GetSegment(r.Context(), segmentID, userID, isAdmin, false)
	if err != nil {
		slog.Error("segment access check failed", "handler", "AddSegmentFavorite", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment", r.URL.Path)
		return
	}
	if seg == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	if err := h.DB.AddSegmentFavorite(r.Context(), userID, segmentID); err != nil {
		slog.Error("add favorite failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to add favorite", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// RemoveSegmentFavorite removes a segment from the user's favorites.
func (h *Handler) RemoveSegmentFavorite(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	segmentID := chi.URLParam(r, "id")
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)

	seg, err := h.DB.GetSegment(r.Context(), segmentID, userID, isAdmin, false)
	if err != nil {
		slog.Error("segment access check failed", "handler", "RemoveSegmentFavorite", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment", r.URL.Path)
		return
	}
	if seg == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	if err := h.DB.RemoveSegmentFavorite(r.Context(), userID, segmentID); err != nil {
		slog.Error("remove favorite failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to remove favorite", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// GetSegmentPerformances retrieves all efforts on a segment from the user's activities.
func (h *Handler) GetSegmentPerformances(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	segmentID := chi.URLParam(r, "id")
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)

	seg, err := h.DB.GetSegment(r.Context(), segmentID, userID, isAdmin, false)
	if err != nil {
		slog.Error("segment access check failed", "handler", "GetSegmentPerformances", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment", r.URL.Path)
		return
	}
	if seg == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	var isReverse *bool
	if v := r.URL.Query().Get("is_reverse"); v != "" {
		b := v == "true"
		isReverse = &b
	}

	performances, err := h.DB.GetSegmentPerformances(r.Context(), segmentID, userID, isReverse)
	if err != nil {
		slog.Error("get segment performances failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to fetch performances", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"performances": performances,
		"count":        len(performances),
	})
}

// GetSegmentPRs returns all segment efforts where the authenticated user achieved personal rank 1–3.
func (h *Handler) GetSegmentPRs(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	efforts, err := h.DB.GetUserSegmentPRs(r.Context(), userID)
	if err != nil {
		slog.Error("get segment prs failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment PRs", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"efforts": efforts,
		"count":   len(efforts),
	})
}

// GetGlobalTopSegmentEfforts returns all segment efforts where the authenticated user
// is ranked in the global top 10 (rank_overall <= 10).
func (h *Handler) GetGlobalTopSegmentEfforts(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	efforts, err := h.DB.GetUserGlobalTopSegmentEfforts(r.Context(), userID)
	if err != nil {
		slog.Error("get global top segment efforts failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get global top segment efforts", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"efforts": efforts,
		"count":   len(efforts),
	})
}

// normalizeSurfaceLabel maps historic German editor strings onto the new
// parent-group labels used by the Surface-taxonomy overhaul (Migration 000316).
// Modern keys and user-only sub-types pass through unchanged. Whitespace is trimmed.
//
// This helper is permanent (no sunset). Redundant normalize-cost is O(1); eliminates
// risk that third-party API clients or stale imports still write old keys and would
// otherwise hit DifficultyBadge/SurfacesBar silent-no-render paths.
func normalizeSurfaceLabel(s string) string {
	s = strings.TrimSpace(s)
	switch s {
	case "Schotter":
		return "Schotter (grob)"
	case "Erde":
		return "Naturweg"
	default:
		return s
	}
}

// normalizeDifficultyColor maps historic German difficulty-color strings to the
// canonical English keys used by DifficultyBadge and the segment leaderboard UI.
// Modern English keys and empty strings pass through unchanged. Whitespace is trimmed.
//
// This helper is permanent (no sunset) — mirrors normalizeSurfaceLabel in purpose.
func normalizeDifficultyColor(s string) string {
	s = strings.TrimSpace(s)
	switch s {
	case "blau":
		return "blue"
	case "rot":
		return "red"
	case "schwarz":
		return "black"
	default:
		return s
	}
}

// SegmentEnrichmentRequest is the POST body for POST /segments/enrichment-suggest.
type SegmentEnrichmentRequest struct {
	Coordinates   [][]float64 `json:"coordinates"`
	SegType       string      `json:"seg_type"`
	DominantSport string      `json:"dominant_sport,omitempty"`
}

// maxCoordinates is the hard cap on the number of coordinate pairs accepted by
// enrichmentSuggest. Two-layer DoS hardening (see handler comment below):
// Layer 1 — http.MaxBytesReader (1 MB body cap).
// Layer 2 — maxCoordinates (protects against [[0,0]]-repetition attacks that
// fit within the 1 MB body cap but would make Viterbi O(n·|candidates|) too
// expensive at n > 5000).
//
// Rationale for 5000: typical GPS segment sub-tracks are < 500 points; draw-mode
// strokes even smaller.  5000 gives headroom for edge cases without opening a
// DoS window on the graph path.
const maxCoordinates = 5000

// EnrichmentSuggest returns aggregated OSM surface/sac/mtb suggestions for a
// segment LineString.
//
// POST /api/v1/segments/enrichment-suggest
// Body: SegmentEnrichmentRequest
// Response: enrichment.SegmentEnrichmentResponse
//
// DoS-Hardening: two layers of defense before any Viterbi/PostGIS call:
//  1. http.MaxBytesReader → 1 MB body cap.
//  2. maxCoordinates check after decode → protects against [[0,0]]-repetition.
func (h *Handler) EnrichmentSuggest(w http.ResponseWriter, r *http.Request) {
	// Require auth — prevents anonymous DoS on the graph path.
	if _, ok := httputil.RequireUserID(w, r); !ok {
		return
	}

	// Layer 1: hard cap on raw request-body size.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB

	var req SegmentEnrichmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid json (or payload too large)", r.URL.Path)
		return
	}

	// Layer 2: coord-count cap.
	if len(req.Coordinates) > maxCoordinates {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
			fmt.Sprintf("too many coordinates: %d (max %d)", len(req.Coordinates), maxCoordinates), r.URL.Path)
		return
	}
	if len(req.Coordinates) < 2 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "coordinates: need at least 2 points", r.URL.Path)
		return
	}

	if !isEnrichableSegmentType(req.SegType) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "seg_type must be one of standard|trail|lap|wintersport|wassersport", r.URL.Path)
		return
	}

	// Wintersport / Wassersport short-circuit — no graph/postgis call needed.
	if req.SegType == "wintersport" || req.SegType == "wassersport" {
		httputil.WriteJSON(w, http.StatusOK, enrichment.SegmentEnrichmentResponse{
			CoveragePct: 0,
			Reason:      req.SegType,
		})
		return
	}

	if !h.requireFeatureConfigured(w, r, capabilities.KeySegmentEnrichmentSuggest) {
		return
	}
	// Build orb.Point slice (GeoJSON convention: [lng, lat]).
	coords := make([]orb.Point, 0, len(req.Coordinates))
	for _, c := range req.Coordinates {
		if len(c) < 2 {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "coordinate pair malformed", r.URL.Path)
			return
		}
		coords = append(coords, orb.Point{c[0], c[1]})
	}

	// Acquire graph reader — matches the pattern used by re_enrich.go, recompute.go,
	// enrichment.go, fusion_merge.go.  When GraphManager is nil (e.g. in unit tests
	// or when the graph is not yet built), gr stays nil and SuggestSegmentEnrichment
	// falls through to the PostGIS fallback.
	var gr routing.GraphReader
	if h.GraphManager != nil {
		gr = h.GraphManager.Reader()
	}

	resp, err := enrichment.SuggestSegmentEnrichment(r.Context(), h.DB, coords, gr)
	if err != nil {
		slog.Warn("segment enrichment suggest failed", "error", err)
		httputil.WriteError(w, http.StatusServiceUnavailable, "Service Unavailable", "keine OSM-Daten installiert", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}
