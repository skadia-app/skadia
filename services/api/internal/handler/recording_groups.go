package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
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
	SportType string `json:"sport_type"`
}

// SetRecordingGroupSportType godoc
// PATCH /api/v1/recording-groups/{id}/sport-type
// Body: {"sport_type": "running"}
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

	if err := h.DB.SetRecordingGroupSportType(r.Context(), groupID, body.SportType); err != nil {
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

	err = h.DB.AddRecordingGroupMember(r.Context(), groupID, body.ActivityID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505": // unique_violation: activity already in a group
				// Check if it is already a member of *this* group — idempotent case.
				members, listErr := h.DB.GetRecordingGroupMembers(r.Context(), groupID)
				if listErr == nil {
					for _, m := range members {
						if m.ActivityID == body.ActivityID {
							// Already a member of this group — idempotent success.
							httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
							return
						}
					}
				}
				// Activity belongs to a different group.
				httputil.WriteError(w, http.StatusConflict, "Conflict", "activity already belongs to another recording group", r.URL.Path)
				return
			}
		}
		slog.Error("add group member", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to add group member", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
