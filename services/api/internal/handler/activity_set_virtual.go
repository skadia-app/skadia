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
//
// exclude_from_heatmap side-effect:
//   - on-toggle (is_virtual=true):  sets exclude_from_heatmap=true; saves the
//     previous exclude_from_heatmap value in the audit row as "prev_exclude_from_heatmap".
//   - off-toggle (is_virtual=false): restores the saved prev_exclude_from_heatmap value
//     from the most recent is_virtual=true audit row.  Falls back to false when no
//     prior audit row exists (conservative default).

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// setVirtualAuditData is the JSON payload stored in activity_edits.edit_data for
// is_virtual edits.  PrevExcludeFromHeatmap is only populated for on-toggles.
type setVirtualAuditData struct {
	Value                  bool  `json:"value"`
	PrevExcludeFromHeatmap *bool `json:"prev_exclude_from_heatmap,omitempty"`
}

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

	// Decode request body — DisallowUnknownFields rejects typos / frontend contract drift.
	var req struct {
		IsVirtual bool `json:"is_virtual"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
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
	defer tx.Rollback(r.Context()) //nolint:errcheck — no-op after Commit

	if req.IsVirtual {
		// on-toggle: save the current exclude_from_heatmap value so we can restore it
		// if the user later removes the virtual flag.
		prevExclude := activity.ExcludeFromHeatmap

		_, err = tx.Exec(r.Context(),
			`UPDATE activities
			    SET is_virtual = true, exclude_from_heatmap = true, updated_at = NOW()
			  WHERE id = $1 AND user_id = $2`,
			activityID, userID,
		)
		if err != nil {
			slog.Error("set-virtual: update activity failed", "id", activityID, "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update activity", r.URL.Path)
			return
		}

		// Audit: store value=true + prev_exclude_from_heatmap for restore on off-toggle.
		auditData, _ := json.Marshal(setVirtualAuditData{
			Value:                  true,
			PrevExcludeFromHeatmap: &prevExclude,
		})
		if _, err = tx.Exec(r.Context(),
			`INSERT INTO activity_edits (activity_id, user_id, edit_type, edit_data)
			 VALUES ($1, $2, 'is_virtual', $3)`,
			activityID, userID, auditData,
		); err != nil {
			slog.Error("set-virtual: audit insert failed", "id", activityID, "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to write audit record", r.URL.Path)
			return
		}
	} else {
		// off-toggle: restore the exclude_from_heatmap value saved during the most recent
		// on-toggle.  Fall back to false when no prior audit row exists.
		var prevExclude bool // conservative default
		var rawAudit []byte
		_ = tx.QueryRow(r.Context(),
			`SELECT edit_data
			   FROM activity_edits
			  WHERE activity_id = $1
			    AND edit_type   = 'is_virtual'
			    AND (edit_data->>'value')::boolean = true
			  ORDER BY created_at DESC
			  LIMIT 1`,
			activityID,
		).Scan(&rawAudit)
		if len(rawAudit) > 0 {
			var ad setVirtualAuditData
			if jsonErr := json.Unmarshal(rawAudit, &ad); jsonErr == nil && ad.PrevExcludeFromHeatmap != nil {
				prevExclude = *ad.PrevExcludeFromHeatmap
			}
		}

		_, err = tx.Exec(r.Context(),
			`UPDATE activities
			    SET is_virtual = false, exclude_from_heatmap = $3, updated_at = NOW()
			  WHERE id = $1 AND user_id = $2`,
			activityID, userID, prevExclude,
		)
		if err != nil {
			slog.Error("set-virtual: update activity failed", "id", activityID, "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update activity", r.URL.Path)
			return
		}

		// Audit: just record value=false (no prev_exclude_from_heatmap needed on off).
		auditData, _ := json.Marshal(setVirtualAuditData{Value: false})
		if _, err = tx.Exec(r.Context(),
			`INSERT INTO activity_edits (activity_id, user_id, edit_type, edit_data)
			 VALUES ($1, $2, 'is_virtual', $3)`,
			activityID, userID, auditData,
		); err != nil {
			slog.Error("set-virtual: audit insert failed", "id", activityID, "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to write audit record", r.URL.Path)
			return
		}
	}

	if err = tx.Commit(r.Context()); err != nil {
		slog.Error("set-virtual: commit failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to commit transaction", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"is_virtual": req.IsVirtual})
}
