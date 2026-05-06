package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

type resolveSportConflictRequest struct {
	SportType  string `json:"sport_type"`
	SubType    string `json:"sub_type,omitempty"`
	IsElectric *bool  `json:"is_electric,omitempty"`
	IsIndoor   *bool  `json:"is_indoor,omitempty"`
	IsVirtual  *bool  `json:"is_virtual,omitempty"`
}

// ResolveSportConflict handles PATCH /api/v1/activities/{id}/resolve-sport-conflict
func (h *Handler) ResolveSportConflict(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activity, activityID, ok := h.requireActivity(w, r, userID)
	if !ok {
		return
	}

	// Already resolved?
	if !activity.SportConflictPending {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "already_resolved"})
		return
	}

	var req resolveSportConflictRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			slog.Warn("sport conflict resolve: invalid request body", "error", err)
		}
	}

	var resolution *database.SportConflictResolution
	if req.SportType != "" || req.IsElectric != nil || req.IsIndoor != nil || req.IsVirtual != nil {
		rawSportType := req.SportType
		rawSubType := req.SubType
		if rawSportType == "" {
			rawSportType = activity.SportType
			rawSubType = derefSubType(activity.SubType)
		}
		tuple, patch, err := sport.NormalizeSelectableSportTuple(rawSportType, rawSubType)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
			return
		}
		resolution = &database.SportConflictResolution{
			SportType:  tuple.SportType,
			SubType:    tuple.SubType,
			IsElectric: firstBool(req.IsElectric, patch.IsElectric),
			IsIndoor:   firstBool(req.IsIndoor, patch.IsIndoor),
			IsVirtual:  req.IsVirtual,
		}
	}

	// Resolve via DB method (TOCTOU-safe: WHERE sport_conflict_pending = true)
	affected, err := h.DB.ResolveSportConflict(r.Context(), activityID, userID, resolution)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to resolve conflict", r.URL.Path)
		return
	}
	if !affected {
		// Another request resolved first (TOCTOU)
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "already_resolved"})
		return
	}

	// Resolve queue entry (graceful if missing)
	_ = h.DB.ResolveImportQueueByActivity(r.Context(), userID, activityID)

	// Trigger recompute if sport changed (reuses existing reprocessActivity from recompute.go)
	if sportConflictResolutionChangesActivity(activity, resolution) {
		// Create background_jobs entry so ActiveJobsIndicator shows progress
		entityID := activityID
		activityTitle := activityID
		if activity.Title != nil && *activity.Title != "" {
			activityTitle = *activity.Title
		} else if activity.TitleAuto != nil && *activity.TitleAuto != "" {
			activityTitle = *activity.TitleAuto
		}
		entityName := fmt.Sprintf("Sport-Konflikt: %s", activityTitle)
		jobID, jobErr := h.DB.InsertBackgroundJob(r.Context(), userID, "activity_recompute", &entityID, &entityName, 1)
		if jobErr != nil {
			slog.Warn("sport conflict: failed to create background job", "error", jobErr)
		}

		go func() {
			failed := false
			defer func() {
				if rec := recover(); rec != nil {
					slog.Error("sport conflict resolve: recompute panicked", "id", activityID, "panic", rec)
					failed = true
				}
				if jobID != "" {
					_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, failed, "")
				}
			}()
			ctx := context.Background()
			if reErr := h.reprocessActivity(ctx, activityID, userID, nil, 0, false, nil); reErr != nil {
				slog.Warn("sport conflict resolve: recompute failed", "id", activityID, "error", reErr)
				failed = true
			}
		}()
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}

func firstBool(preferred, fallback *bool) *bool {
	if preferred != nil {
		return preferred
	}
	return fallback
}

func derefSubType(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func sportConflictResolutionChangesActivity(activity *model.Activity, resolution *database.SportConflictResolution) bool {
	if activity == nil || resolution == nil {
		return false
	}
	if resolution.SportType != activity.SportType || derefSubType(activity.SubType) != resolution.SubType {
		return true
	}
	if resolution.IsElectric != nil && *resolution.IsElectric != activity.IsElectric {
		return true
	}
	if resolution.IsIndoor != nil && *resolution.IsIndoor != activity.IsIndoor {
		return true
	}
	if resolution.IsVirtual != nil && *resolution.IsVirtual != activity.IsVirtual {
		return true
	}
	return false
}
