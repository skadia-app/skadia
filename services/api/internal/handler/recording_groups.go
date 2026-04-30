package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/dbutil"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// requireGroupOwner loads the recording group identified by the "id" URL param,
// checks that it belongs to userID, and returns it.
// On error it writes the appropriate HTTP response and returns (nil, false).
func (h *Handler) requireGroupOwner(w http.ResponseWriter, r *http.Request, userID, groupID string) (*database.RecordingGroupWithMembers, bool) {
	group, err := h.DB.GetRecordingGroup(r.Context(), groupID)
	if err != nil {
		slog.Error("get recording group", "group_id", groupID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get recording group", r.URL.Path)
		return nil, false
	}
	if group == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "recording group not found", r.URL.Path)
		return nil, false
	}
	if group.UserID != userID {
		httputil.WriteError(w, http.StatusForbidden, "Forbidden", "access denied", r.URL.Path)
		return nil, false
	}
	// Wrap in RecordingGroupWithMembers for a uniform return type; members are not loaded here.
	return &database.RecordingGroupWithMembers{RecordingGroup: *group}, true
}

// GetRecordingGroups godoc
// GET /api/v1/recording-groups?limit=50&offset=0
func (h *Handler) GetRecordingGroups(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Trigger clique integration if fusion cache is stale.
	// This ensures recording_groups is populated even when the client never
	// called the old /fusion-queue endpoint.
	// Pass the already-known invalidation sequence to avoid a second DB call inside
	// MaybeTriggerFusionQueueCompute.
	stale, invalidationSeq, err := h.DB.IsFusionCacheStale(r.Context(), userID)
	if err != nil {
		slog.Warn("get recording groups: cache staleness check failed", "error", err)
	} else if stale {
		h.triggerFusionQueueCompute(r.Context(), userID, invalidationSeq)
	}

	groups, err := h.DB.GetRecordingGroupsForUser(r.Context(), userID, limit, offset)
	if err != nil {
		slog.Error("get recording groups", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get recording groups", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, groups)
}

// GetRecordingGroup godoc
// GET /api/v1/recording-groups/{id}
func (h *Handler) GetRecordingGroup(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	groupID := chi.URLParam(r, "id")

	group, ok := h.requireGroupOwner(w, r, userID, groupID)
	if !ok {
		return
	}

	members, err := h.DB.GetRecordingGroupMembers(r.Context(), groupID)
	if err != nil {
		slog.Error("get recording group members", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get recording group members", r.URL.Path)
		return
	}
	group.Members = members

	httputil.WriteJSON(w, http.StatusOK, group)
}

// GetRecordingGroupMembers godoc
// GET /api/v1/recording-groups/{id}/members
func (h *Handler) GetRecordingGroupMembers(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	groupID := chi.URLParam(r, "id")

	if _, ok := h.requireGroupOwner(w, r, userID, groupID); !ok {
		return
	}

	members, err := h.DB.GetRecordingGroupMembers(r.Context(), groupID)
	if err != nil {
		slog.Error("get recording group members", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get recording group members", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, members)
}

// DissolveRecordingGroup godoc
// POST /api/v1/recording-groups/{id}/dissolve
func (h *Handler) DissolveRecordingGroup(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	groupID := chi.URLParam(r, "id")

	if _, ok := h.requireGroupOwner(w, r, userID, groupID); !ok {
		return
	}

	if err := h.DB.DissolveRecordingGroup(r.Context(), groupID); err != nil {
		slog.Error("dissolve recording group", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to dissolve recording group", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type setRecordingGroupPrimaryRequest struct {
	ActivityID string `json:"activity_id"`
}

// SetRecordingGroupPrimary godoc
// PATCH /api/v1/recording-groups/{id}/primary
// Body: {"activity_id": "uuid"}
func (h *Handler) SetRecordingGroupPrimary(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	groupID := chi.URLParam(r, "id")

	body, ok := httputil.DecodeJSON[setRecordingGroupPrimaryRequest](w, r)
	if !ok {
		return
	}

	if _, ok := h.requireGroupOwner(w, r, userID, groupID); !ok {
		return
	}

	if err := h.DB.SetRecordingGroupPrimary(r.Context(), groupID, body.ActivityID); err != nil {
		slog.Error("set recording group primary", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to set primary activity", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type setRecordingGroupSportTypeRequest struct {
	SportType  string  `json:"sport_type"`
	SubType    *string `json:"sub_type"`
	IsElectric *bool   `json:"is_electric"`
	IsIndoor   *bool   `json:"is_indoor"`
	IsVirtual  *bool   `json:"is_virtual"`
}

// SetRecordingGroupSportType godoc
// PATCH /api/v1/recording-groups/{id}/sport-type
// Body: {"sport_type": "cycling", "sub_type": "mountain", "is_electric": true}
func (h *Handler) SetRecordingGroupSportType(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	groupID := chi.URLParam(r, "id")

	body, ok := httputil.DecodeJSON[setRecordingGroupSportTypeRequest](w, r)
	if !ok {
		return
	}

	if _, ok := h.requireGroupOwner(w, r, userID, groupID); !ok {
		return
	}

	tuple := database.RecordingGroupSportTuple{}
	if body.SportType != "" {
		rawSubType := ""
		if body.SubType != nil {
			rawSubType = *body.SubType
		}
		normalized, patch, err := sport.NormalizeSportTuple(body.SportType, rawSubType)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
			return
		}
		tuple.SportType = normalized.SportType
		if normalized.SubType != "" {
			tuple.SubType = &normalized.SubType
		}
		tuple.IsElectric = body.IsElectric
		tuple.IsIndoor = body.IsIndoor
		tuple.IsVirtual = body.IsVirtual
		if patch.IsElectric != nil {
			tuple.IsElectric = patch.IsElectric
		}
		if patch.IsIndoor != nil {
			tuple.IsIndoor = patch.IsIndoor
		} else if sport.IsIndoor(normalized.SportType, normalized.SubType) && tuple.IsIndoor == nil {
			v := true
			tuple.IsIndoor = &v
		}
	} else {
		tuple.IsElectric = body.IsElectric
		tuple.IsIndoor = body.IsIndoor
		tuple.IsVirtual = body.IsVirtual
	}

	if err := h.DB.SetRecordingGroupSportType(r.Context(), groupID, tuple); err != nil {
		slog.Error("set recording group sport type", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to set sport type", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type addGroupMemberRequest struct {
	ActivityID string `json:"activity_id"`
}

// AddGroupMember godoc
// POST /api/v1/recording-groups/{id}/members
// Body: {"activity_id": "uuid"}
// Idempotent: returns 200 OK if the activity is already a member of this group.
// Returns 409 Conflict if the activity already belongs to a different group.
func (h *Handler) AddGroupMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	groupID := chi.URLParam(r, "id")

	body, ok := httputil.DecodeJSON[addGroupMemberRequest](w, r)
	if !ok {
		return
	}
	if body.ActivityID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "activity_id is required", r.URL.Path)
		return
	}

	// Verify the group exists and belongs to this user.
	if _, ok := h.requireGroupOwner(w, r, userID, groupID); !ok {
		return
	}

	// Verify the activity exists and belongs to this user.
	activity, err := h.DB.GetActivity(r.Context(), body.ActivityID)
	if err != nil {
		slog.Error("add group member: get activity", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get activity", r.URL.Path)
		return
	}
	if activity == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}
	if activity.UserID != userID {
		httputil.WriteError(w, http.StatusForbidden, "Forbidden", "access denied", r.URL.Path)
		return
	}

	// Acquire the same advisory lock that FusionMerge holds for this user.
	// This closes the TOCTOU window between the fusion-source EXISTS check below
	// and the INSERT: without this lock, a concurrent /fusion-merge could mark
	// the activity as a source between the check and the insert, producing a
	// zombie (activity simultaneously in recording_group_members and
	// activity_fusion_memberships.source_id).  R4-4.
	//
	// Using the same "fusion:"+userID key as FusionMerge means the two handlers
	// are mutually exclusive per user — they will never overlap.
	lockKey := dbutil.AdvisoryLockKey("fusion:" + userID)
	tx, err := h.DB.Pool.Begin(r.Context())
	if err != nil {
		slog.Error("add group member: begin tx", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to begin transaction", r.URL.Path)
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	var lockAcquired bool
	if err := tx.QueryRow(r.Context(), `SELECT pg_try_advisory_xact_lock($1)`, lockKey).Scan(&lockAcquired); err != nil {
		slog.Error("add group member: advisory lock query", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "advisory lock: "+err.Error(), r.URL.Path)
		return
	}
	if !lockAcquired {
		slog.Warn("add group member: advisory lock contention — fusion operation in progress for this user", "user_id", userID)
		httputil.WriteError(w, http.StatusConflict, "Conflict", "another fusion operation is in progress for this user", r.URL.Path)
		return
	}

	// Reject fusion-source activities: once an activity is a merge-source it is
	// hidden from the activities list (source_id IS NOT NULL in
	// activity_fusion_memberships).  Adding it back to a recording group would
	// regenerate the zombie state that trigger 000313 prevents on the merge path.
	// The trigger is defence-in-depth; this check produces a clear 400 for callers
	// instead of a silent FK anomaly.  (R3-3)
	// Runs INSIDE the advisory lock so FusionMerge cannot insert a membership row
	// between this check and the INSERT below.
	var isFusionSource bool
	if err := tx.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM activity_fusion_memberships WHERE source_id = $1)`,
		body.ActivityID,
	).Scan(&isFusionSource); err != nil {
		slog.Error("add group member: check fusion-source", "activity_id", body.ActivityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to check fusion source state", r.URL.Path)
		return
	}
	if isFusionSource {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
			"cannot add a fusion-source activity to a recording group", r.URL.Path)
		return
	}

	if _, err := tx.Exec(r.Context(), "SAVEPOINT sp_add_member"); err != nil {
		slog.Error("add group member: savepoint", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to add group member", r.URL.Path)
		return
	}
	if err := h.DB.AddRecordingGroupMemberTx(r.Context(), tx, groupID, body.ActivityID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// unique_violation: activity is already a member of some group.
			// Roll back to the SAVEPOINT to un-abort the transaction, then check
			// whether it is *this* group (idempotent 200) or a different group (409).
			// Without the ROLLBACK TO SAVEPOINT the TX stays in aborted state and
			// the follow-up SELECT would fail with "transaction aborted".
			if _, rollErr := tx.Exec(r.Context(), "ROLLBACK TO SAVEPOINT sp_add_member"); rollErr != nil {
				slog.Error("add group member: rollback savepoint", "error", rollErr)
				httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to add group member", r.URL.Path)
				return
			}
			var alreadyInThisGroup bool
			if qErr := tx.QueryRow(r.Context(),
				`SELECT EXISTS(SELECT 1 FROM recording_group_members WHERE group_id = $1 AND activity_id = $2)`,
				groupID, body.ActivityID,
			).Scan(&alreadyInThisGroup); qErr != nil {
				slog.Error("add group member: check existing membership", "error", qErr)
				httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to check group membership", r.URL.Path)
				return
			}
			if alreadyInThisGroup {
				// Already a member of this group — idempotent success.
				httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
				return
			}
			// Activity belongs to a different group.
			httputil.WriteError(w, http.StatusConflict, "Conflict", "activity already belongs to another recording group", r.URL.Path)
			return
		}
		slog.Error("add group member", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to add group member", r.URL.Path)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		slog.Error("add group member: commit tx", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to commit transaction", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
