package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/automation"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/rules"
)

func (h *Handler) ListRules(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	items, err := h.DB.ListRules(r.Context(), userID)
	if err != nil {
		slog.Error("list rules failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list rules", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, items)
}

func (h *Handler) CreateRule(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	create, ok := httputil.DecodeJSON[model.AutomationRuleCreate](w, r)
	if !ok {
		return
	}
	if create.Name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if len(create.Conditions) == 0 && len(create.ConditionGroups) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "conditions or condition_groups are required", r.URL.Path)
		return
	}
	if len(create.Actions) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "actions are required", r.URL.Path)
		return
	}

	rule, err := h.DB.CreateRule(r.Context(), userID, &create)
	if err != nil {
		slog.Error("create rule failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create rule", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, rule)
}

func (h *Handler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	update, ok := httputil.DecodeJSON[model.AutomationRuleUpdate](w, r)
	if !ok {
		return
	}

	rule, err := h.DB.UpdateRule(r.Context(), id, userID, &update)
	if err != nil {
		if err.Error() == "this system rule is read-only" {
			httputil.WriteError(w, http.StatusForbidden, "Forbidden", "this system rule is read-only", r.URL.Path)
			return
		}
		slog.Error("update rule failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update rule", r.URL.Path)
		return
	}
	if rule == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "rule not found", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, rule)
}

func (h *Handler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	if err := h.DB.DeleteRule(r.Context(), id, userID); err != nil {
		if err.Error() == "rule not found" {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "rule not found", r.URL.Path)
			return
		}
		slog.Error("delete rule failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete rule", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ApplyRules(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	if r.URL.Query().Get("dry_run") == "true" {
		h.dryRunAllRules(w, r, userID)
		return
	}

	// Load all active rules upfront (fast, no heavy work in the request goroutine).
	allRules, err := h.DB.ListRules(r.Context(), userID)
	if err != nil {
		slog.Error("apply rules: list failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list rules", r.URL.Path)
		return
	}

	var activeRules []model.AutomationRule
	for _, rule := range allRules {
		if rule.IsActive {
			activeRules = append(activeRules, rule)
		}
	}

	if len(activeRules) == 0 {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
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
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
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
			if err := h.ApplyRulesToActivity(ctx, activityID, userID, activeRules, ""); err != nil {
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
			if err := h.DB.CompleteBackgroundJob(ctx, jobID, failed, ""); err != nil {
				slog.Warn("apply rules: complete job failed", "job_id", jobID, "error", err)
			}
		}
	}()

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"status": "apply rules started",
		"job_id": jobID,
	})
}

// ApplyRulesToActivity is a thin wrapper around automation.ApplyRulesToActivity
// for use in handler contexts.
func (h *Handler) ApplyRulesToActivity(ctx context.Context, activityID, userID string, allRules []model.AutomationRule, triggerEvent string) error {
	return automation.ApplyRulesToActivity(ctx, h.DB, activityID, userID, allRules, triggerEvent)
}

func (h *Handler) dryRunAllRules(w http.ResponseWriter, r *http.Request, userID string) {
	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	allRules, err := h.DB.ListRules(r.Context(), userID)
	if err != nil {
		slog.Error("dry-run all rules: list rules", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list rules", r.URL.Path)
		return
	}
	actIDs, err := h.DB.ListActivityIDsForUser(r.Context(), userID)
	if err != nil {
		slog.Error("dry-run all rules: list activities", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
		return
	}

	// Tag-Namen vorladen für menschenlesbare Action-Strings
	tagNameByID := map[string]string{}
	if tags, err := h.DB.ListTags(r.Context(), userID); err == nil {
		for _, t := range tags {
			tagNameByID[t.ID] = t.Name
		}
	}

	syncSystemTypes := map[string]bool{
		"electric_sync": true,
		"virtual_sync":  true,
		"indoor_sync":   true,
		"commute_sync":  true,
	}

	type ruleMatch struct {
		RuleName string `json:"rule_name"`
		Action   string `json:"action"`
	}
	type matchResult struct {
		ActivityID    string      `json:"activity_id"`
		ActivityTitle string      `json:"activity_title"`
		StartedAt     string      `json:"started_at"`
		SportType     string      `json:"sport_type"`
		DistanceM     float64     `json:"distance_m"`
		Rules         []ruleMatch `json:"rules"`
	}

	var matches []matchResult
	totalMatches := 0

	for _, actID := range actIDs {
		actMap, err := h.DB.GetActivityAsMap(r.Context(), actID)
		if err != nil || actMap == nil {
			continue
		}

		var actRules []ruleMatch
		for _, rule := range allRules {
			if !rule.IsActive {
				continue
			}
			if rule.SystemType != nil && syncSystemTypes[*rule.SystemType] {
				continue
			}
			if automation.EvaluateRule(rule, actMap) {
				var actions []rules.Action
				_ = json.Unmarshal(rule.Actions, &actions)
				for _, a := range actions {
					actionStr := a.Action
					if a.Action == "add_tag" || a.Action == "remove_tag" {
						var p struct {
							TagID string `json:"tag_id"`
						}
						_ = json.Unmarshal(a.Params, &p)
						if name, ok := tagNameByID[p.TagID]; ok {
							actionStr = a.Action + ": " + name
						}
					}
					actRules = append(actRules, ruleMatch{RuleName: rule.Name, Action: actionStr})
				}
			}
		}

		if len(actRules) > 0 {
			totalMatches++
			if totalMatches > offset && len(matches) < limit {
				title, _ := actMap["title"].(string)
				sportType, _ := actMap["sport_type"].(string)
				distanceM, _ := actMap["distance_m"].(float64)

				// Fallback-Titel aus Sport + Stadt
				if title == "" {
					city, _ := actMap["start_city"].(string)
					if sportType != "" && city != "" {
						title = sportType + " in " + city
					} else if sportType != "" {
						title = sportType
					}
				}

				startedAtStr := ""
				if sa, ok := actMap["started_at"]; ok && sa != nil {
					if t, ok := sa.(time.Time); ok {
						startedAtStr = t.Format("2006-01-02")
					}
				}

				matches = append(matches, matchResult{
					ActivityID:    actID,
					ActivityTitle: title,
					StartedAt:     startedAtStr,
					SportType:     sportType,
					DistanceM:     distanceM,
					Rules:         actRules,
				})
			}
		}
	}

	if matches == nil {
		matches = []matchResult{}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"matches":       matches,
		"total_matches": totalMatches,
		"truncated":     totalMatches > offset+limit,
	})
}

// ApplySingleRule evaluates a single rule against all activities for the user.
// POST /api/v1/automation-rules/{id}/apply?dry_run=true|false
func (h *Handler) ApplySingleRule(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	ruleID := chi.URLParam(r, "id")
	dryRun := r.URL.Query().Get("dry_run") == "true"

	allRules, err := h.DB.ListRules(r.Context(), userID)
	if err != nil {
		slog.Error("apply single rule: list rules", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list rules", r.URL.Path)
		return
	}
	var targetRule *model.AutomationRule
	for i := range allRules {
		if allRules[i].ID == ruleID {
			targetRule = &allRules[i]
			break
		}
	}
	if targetRule == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "rule not found", r.URL.Path)
		return
	}

	if dryRun {
		h.applySingleRuleDryRun(w, r, userID, *targetRule)
		return
	}

	jobName := fmt.Sprintf("Regel '%s' anwenden", targetRule.Name)
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "rule_apply", &ruleID, &jobName, 0)
	if err != nil {
		slog.Error("apply single rule: insert job", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create job", r.URL.Path)
		return
	}
	rule := *targetRule
	go func() {
		ctx := context.Background()
		applyErr := h.applySingleRuleToAll(ctx, userID, rule, jobID)
		failed := applyErr != nil
		errText := ""
		if applyErr != nil {
			errText = applyErr.Error()
		}
		if err := h.DB.CompleteBackgroundJob(ctx, jobID, failed, errText); err != nil {
			slog.Warn("apply single rule: complete job", "job_id", jobID, "error", err)
		}
	}()

	httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

func (h *Handler) applySingleRuleDryRun(w http.ResponseWriter, r *http.Request, userID string, rule model.AutomationRule) {
	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	activityIDs, err := h.DB.ListActivityIDsForUser(r.Context(), userID)
	if err != nil {
		slog.Error("apply single rule dry-run: list activities", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
		return
	}

	type matchResult struct {
		ActivityID    string  `json:"activity_id"`
		ActivityTitle string  `json:"activity_title"`
		StartedAt     string  `json:"started_at"`
		SportType     string  `json:"sport_type"`
		DistanceM     float64 `json:"distance_m"`
	}

	var matches []matchResult
	totalMatches := 0

	for _, actID := range activityIDs {
		actMap, err := h.DB.GetActivityAsMap(r.Context(), actID)
		if err != nil || actMap == nil {
			continue
		}
		if automation.EvaluateRule(rule, actMap) {
			totalMatches++
			if totalMatches > offset && len(matches) < limit {
				title, _ := actMap["title"].(string)
				sportType, _ := actMap["sport_type"].(string)
				distanceM, _ := actMap["distance_m"].(float64)

				// Fallback-Titel aus Sport + Stadt
				if title == "" {
					city, _ := actMap["start_city"].(string)
					if sportType != "" && city != "" {
						title = sportType + " in " + city
					} else if sportType != "" {
						title = sportType
					}
				}

				startedAtStr := ""
				if sa, ok := actMap["started_at"]; ok && sa != nil {
					if t, ok := sa.(time.Time); ok {
						startedAtStr = t.Format("2006-01-02")
					}
				}

				matches = append(matches, matchResult{
					ActivityID:    actID,
					ActivityTitle: title,
					StartedAt:     startedAtStr,
					SportType:     sportType,
					DistanceM:     distanceM,
				})
			}
		}
	}

	// Ensure nil slice serialises as [] not null
	if matches == nil {
		matches = []matchResult{}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"matches":       matches,
		"total_matches": totalMatches,
		"truncated":     totalMatches > offset+limit,
	})
}

func (h *Handler) applySingleRuleToAll(ctx context.Context, userID string, rule model.AutomationRule, jobID string) error {
	actIDs, err := h.DB.ListActivityIDsForUser(ctx, userID)
	if err != nil {
		return err
	}
	singleRule := []model.AutomationRule{rule}
	var errorCount int
	for i, actID := range actIDs {
		if err := automation.ApplyRulesToActivity(ctx, h.DB, actID, userID, singleRule, ""); err != nil {
			errorCount++
			slog.Warn("apply single rule to activity", "activity_id", actID, "rule_id", rule.ID, "error", err)
		}
		if i%50 == 0 || i == len(actIDs)-1 {
			if err := h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(i+1)); err != nil {
				slog.Warn("apply single rule: update progress", "job_id", jobID, "error", err)
			}
		}
	}
	slog.Info("apply single rule: done", "user_id", userID, "rule_id", rule.ID, "total", len(actIDs), "errors", errorCount)
	if errorCount > 0 {
		return fmt.Errorf("%d errors processing %d activities", errorCount, len(actIDs))
	}
	return nil
}
