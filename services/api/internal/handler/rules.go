package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/automation"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (h *Handler) ListRules(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	items, err := h.DB.ListRules(r.Context(), userID)
	if err != nil {
		slog.Error("list rules failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list rules", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) CreateRule(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var create model.AutomationRuleCreate
	if err := json.NewDecoder(r.Body).Decode(&create); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if create.Name == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if len(create.Conditions) == 0 && len(create.ConditionGroups) == 0 {
		writeError(w, http.StatusBadRequest, "Bad Request", "conditions or condition_groups are required", r.URL.Path)
		return
	}
	if len(create.Actions) == 0 {
		writeError(w, http.StatusBadRequest, "Bad Request", "actions are required", r.URL.Path)
		return
	}

	rule, err := h.DB.CreateRule(r.Context(), userID, &create)
	if err != nil {
		slog.Error("create rule failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create rule", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusCreated, rule)
}

func (h *Handler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r.Context())

	var update model.AutomationRuleUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	rule, err := h.DB.UpdateRule(r.Context(), id, userID, &update)
	if err != nil {
		slog.Error("update rule failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update rule", r.URL.Path)
		return
	}
	if rule == nil {
		writeError(w, http.StatusNotFound, "Not Found", "rule not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, rule)
}

func (h *Handler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r.Context())

	if err := h.DB.DeleteRule(r.Context(), id, userID); err != nil {
		if err.Error() == "rule not found" {
			writeError(w, http.StatusNotFound, "Not Found", "rule not found", r.URL.Path)
			return
		}
		slog.Error("delete rule failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete rule", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ApplyRules(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	// Load all active rules upfront (fast, no heavy work in the request goroutine).
	allRules, err := h.DB.ListRules(r.Context(), userID)
	if err != nil {
		slog.Error("apply rules: list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list rules", r.URL.Path)
		return
	}

	var activeRules []model.AutomationRule
	for _, rule := range allRules {
		if rule.IsActive {
			activeRules = append(activeRules, rule)
		}
	}

	if len(activeRules) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"job_id":   nil,
			"applied":  0,
			"affected": 0,
		})
		return
	}

	// Load activity IDs to know the total for the progress counter.
	activityIDs, err := h.DB.ListActivityIDsForUser(r.Context(), userID)
	if err != nil {
		slog.Error("apply rules: list activities failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
		return
	}

	// Create a background job record so the UI can track progress.
	jobName := "Automation rules"
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "apply_rules", nil, &jobName, int64(len(activityIDs)))
	if err != nil {
		slog.Warn("apply rules: failed to insert background job", "error", err)
		jobID = ""
	}

	// All heavy processing happens in a goroutine; we return 202 immediately.
	go func() {
		ctx := context.Background()
		failed := false

		for i, activityID := range activityIDs {
			if err := h.ApplyRulesToActivity(ctx, activityID, userID, activeRules); err != nil {
				slog.Warn("apply rules to activity", "activity_id", activityID, "error", err)
			}

			if jobID != "" && (i%10 == 0 || i == len(activityIDs)-1) {
				if err := h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(i+1)); err != nil {
					slog.Warn("apply rules: update progress", "job_id", jobID, "error", err)
				}
			}
		}

		slog.Info("apply rules: done", "user_id", userID, "total", len(activityIDs))

		if jobID != "" {
			if err := h.DB.CompleteBackgroundJob(ctx, jobID, failed); err != nil {
				slog.Warn("apply rules: complete job failed", "job_id", jobID, "error", err)
			}
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status": "apply rules started",
		"job_id": jobID,
	})
}

// ApplyRulesToActivity is a thin wrapper around automation.ApplyRulesToActivity
// for use in handler contexts.
func (h *Handler) ApplyRulesToActivity(ctx context.Context, activityID, userID string, allRules []model.AutomationRule) error {
	return automation.ApplyRulesToActivity(ctx, h.DB, activityID, userID, allRules)
}

func (h *Handler) MigrateTagConditions(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	count, err := h.DB.MigrateTagConditionsToRules(r.Context(), userID)
	if err != nil {
		slog.Error("migrate tag conditions failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "migration failed", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"migrated": count})
}

// resolveNameTemplate substitutes a fixed set of {token} placeholders in tmpl
// with values from the activity map. The substitution is a simple sequential
// strings.ReplaceAll pass — tokens cannot be nested or escaped. Adding a new
// token requires explicitly extending this function.
//
// Known limitation: there is no escaping mechanism, so a literal "{city}" in
// an intended output cannot be produced.
func resolveNameTemplate(tmpl string, act map[string]any) string {
	result := tmpl
	if city, ok := act["start_city"].(string); ok {
		result = strings.ReplaceAll(result, "{city}", city)
	}
	if dist, ok := act["distance_m"].(float64); ok {
		result = strings.ReplaceAll(result, "{distance_km}", fmt.Sprintf("%.0f km", dist/1000))
	}
	if sport, ok := act["sport_type"].(string); ok {
		result = strings.ReplaceAll(result, "{sport}", sport)
	}
	return result
}
