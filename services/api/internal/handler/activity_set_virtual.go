package handler

// activity_set_virtual.go — POST /activities/{id}/set-virtual
//
// Toggles the is_virtual flag on an activity owned by the requesting user.
// Writes an activity_edits audit row (edit_type='is_virtual') inside the
// same transaction so that a partial failure cannot produce a flag change
// without an audit trail.
//
// Request body:  { "is_virtual": true|false }
// Response body: { "is_virtual": true|false }

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// SetActivityVirtual handles POST /activities/{id}/set-virtual.
func (h *Handler) SetActivityVirtual(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")

	// Ownership check — GetActivity returns nil (no error) when not found.
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("set-virtual: get activity failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	// Decode request body.
	var req struct {
		IsVirtual bool `json:"is_virtual"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	// BEGIN transaction — UPDATE + audit INSERT must be atomic.
	tx, err := h.beginTx(r.Context())
	if err != nil {
		slog.Error("set-virtual: begin tx failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to begin transaction", r.URL.Path)
		return
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(r.Context())
		}
	}()

	// Update is_virtual on the activity row. Also mirror to exclude_from_heatmap
	// when enabling virtual (consistent with insert-time logic in activities_db.go
	// where is_virtual implies exclude_from_heatmap=true). When disabling virtual
	// we leave exclude_from_heatmap unchanged — the user controls it independently.
	if req.IsVirtual {
		_, err = tx.Exec(r.Context(),
			`UPDATE activities
			    SET is_virtual = true, exclude_from_heatmap = true, updated_at = NOW()
			  WHERE id = $1 AND user_id = $2`,
			activityID, userID,
		)
	} else {
		_, err = tx.Exec(r.Context(),
			`UPDATE activities
			    SET is_virtual = false, updated_at = NOW()
			  WHERE id = $1 AND user_id = $2`,
			activityID, userID,
		)
	}
	if err != nil {
		slog.Error("set-virtual: update activity failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update activity", r.URL.Path)
		return
	}

	// Audit log — insert inside the same transaction.
	auditData, _ := json.Marshal(map[string]any{"value": req.IsVirtual})
	if _, err = tx.Exec(r.Context(),
		`INSERT INTO activity_edits (activity_id, user_id, edit_type, edit_data)
		 VALUES ($1, $2, 'is_virtual', $3)`,
		activityID, userID, auditData,
	); err != nil {
		slog.Error("set-virtual: audit insert failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to write audit record", r.URL.Path)
		return
	}

	if err = tx.Commit(r.Context()); err != nil {
		slog.Error("set-virtual: commit failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to commit transaction", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"is_virtual": req.IsVirtual})
}
