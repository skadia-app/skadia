package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/rules"
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
	if len(create.Conditions) == 0 {
		writeError(w, http.StatusBadRequest, "Bad Request", "conditions are required", r.URL.Path)
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

	var update model.AutomationRuleUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	rule, err := h.DB.UpdateRule(r.Context(), id, &update)
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

	if err := h.DB.DeleteRule(r.Context(), id); err != nil {
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

	// Load all active rules
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
			"applied":  0,
			"affected": 0,
		})
		return
	}

	// Load all activity IDs
	activityIDs, err := h.DB.ListActivityIDsForUser(r.Context(), userID)
	if err != nil {
		slog.Error("apply rules: list activities failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
		return
	}

	applied := 0
	affected := 0

	for _, activityID := range activityIDs {
		actMap, err := h.DB.GetActivityAsMap(r.Context(), activityID)
		if err != nil || actMap == nil {
			continue
		}

		activityChanged := false
		for _, rule := range activeRules {
			var conditions []rules.Condition
			if err := json.Unmarshal(rule.Conditions, &conditions); err != nil {
				continue
			}
			var actions []rules.Action
			if err := json.Unmarshal(rule.Actions, &actions); err != nil {
				continue
			}

			if !rules.Evaluate(conditions, actMap) {
				continue
			}

			changes := rules.Apply(actions)
			for _, change := range changes {
				if err := h.DB.ApplyRuleChange(r.Context(), activityID, change.Field, change.Value); err != nil {
					slog.Warn("apply rule change failed",
						"activity_id", activityID, "rule_id", rule.ID, "field", change.Field, "error", err)
					continue
				}
				activityChanged = true
			}

			if activityChanged {
				if err := h.DB.IncrementRuleApplied(r.Context(), rule.ID); err != nil {
					slog.Warn("increment rule applied failed", "rule_id", rule.ID, "error", err)
				}
				applied++
			}
		}

		if activityChanged {
			affected++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"applied":  applied,
		"affected": affected,
	})
}
