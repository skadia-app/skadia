package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/dbutil"
	"github.com/skadia-app/skadia/services/api/internal/geo"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

const (
	commuteAutoSystemType = "commute_auto"
	commuteTagName        = "Pendeln"
	commuteRadiusM        = 200.0
)

type commuteRuleRequest struct {
	ActivityID  string   `json:"activity_id"`
	StartZoneID string   `json:"start_zone_id"`
	EndZoneID   string   `json:"end_zone_id"`
	// For roundtrip: user specifies the intermediate destination
	DestLat     *float64 `json:"dest_lat,omitempty"`
	DestLon     *float64 `json:"dest_lon,omitempty"`
	DestZoneID  string   `json:"dest_zone_id"`
}

type commuteRuleResponse struct {
	RuleID  string `json:"rule_id"`
	Created bool   `json:"created"`
	Message string `json:"message"`
}

func (h *Handler) CreateOrExtendCommuteRule(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	var req commuteRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid request body", r.URL.Path)
		return
	}

	// Advisory lock per user — session-level with pinned connection.
	lockKey := dbutil.AdvisoryLockKey(userID + "_commute_rule")

	conn, err := h.DB.Pool.Acquire(ctx)
	if err != nil {
		slog.Error("commute rule: pool acquire failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "pool acquire failed", r.URL.Path)
		return
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		slog.Error("commute rule: advisory lock failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "lock failed", r.URL.Path)
		return
	}
	defer conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", lockKey) //nolint:errcheck

	var startLat, startLon, endLat, endLon float64
	if req.ActivityID != "" {
		coords, err := h.DB.GetActivityStartEnd(ctx, req.ActivityID, userID)
		if err != nil {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found or has no GPS", r.URL.Path)
			return
		}
		startLat, startLon, endLat, endLon = coords[0], coords[1], coords[2], coords[3]
	} else if req.StartZoneID != "" && req.EndZoneID != "" {
		centers, err := h.DB.GetPrivacyZoneCenters(ctx, []string{req.StartZoneID, req.EndZoneID}, userID)
		if err != nil || len(centers) != 2 {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "zones not found", r.URL.Path)
			return
		}
		startLat, startLon = centers[req.StartZoneID][0], centers[req.StartZoneID][1]
		endLat, endLon = centers[req.EndZoneID][0], centers[req.EndZoneID][1]
	} else {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "activity_id or start_zone_id+end_zone_id required", r.URL.Path)
		return
	}

	// Round-trip check: if start ≈ end, require destination info for dwell-based rule
	isRoundTrip := geo.DistanceHaversine(startLat, startLon, endLat, endLon) <= commuteRadiusM
	if isRoundTrip {
		var destLat, destLon float64
		if req.DestZoneID != "" {
			centers, err := h.DB.GetPrivacyZoneCenters(ctx, []string{req.DestZoneID}, userID)
			if err != nil || len(centers) == 0 {
				httputil.WriteError(w, http.StatusNotFound, "Not Found", "destination zone not found", r.URL.Path)
				return
			}
			coords := centers[req.DestZoneID]
			destLat, destLon = coords[0], coords[1]
		} else if req.DestLat != nil && req.DestLon != nil {
			destLat, destLon = *req.DestLat, *req.DestLon
		} else {
			httputil.WriteError(w, http.StatusUnprocessableEntity, "Round Trip",
				"round-trip activity requires dest_lat/dest_lon or dest_zone_id", r.URL.Path)
			return
		}

		pendelTagID, err := h.DB.GetSystemTagID(ctx, userID, commuteTagName)
		if err != nil {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "Pendeln tag not found — check enrichment settings", r.URL.Path)
			return
		}

		existingRule, err := h.DB.GetRuleBySystemType(ctx, userID, commuteAutoSystemType)
		if err != nil {
			slog.Error("commute rule: GetRuleBySystemType failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load rule", r.URL.Path)
			return
		}

		if existingRule != nil {
			if commuteRouteExists(existingRule, startLat, startLon, destLat, destLon, commuteRadiusM) {
				httputil.WriteJSON(w, http.StatusConflict, commuteRuleResponse{
					RuleID:  existingRule.ID,
					Created: false,
					Message: "Route already exists in commute rule",
				})
				return
			}
			newBranches := makeCommuteRoundtripBranches("else_if", startLat, startLon, destLat, destLon, commuteRadiusM, pendelTagID)
			if err := extendCommuteRuleRaw(ctx, h.DB, existingRule, newBranches); err != nil {
				if err.Error() == "max 20 commute routes reached" {
					httputil.WriteError(w, http.StatusUnprocessableEntity, "Limit Reached", "max 20 commute routes — delete old routes in /workshop/rules", r.URL.Path)
				} else {
					slog.Error("commute rule: extendCommuteRuleRaw failed", "error", err)
					httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to extend rule", r.URL.Path)
				}
				return
			}
			httputil.WriteJSON(w, http.StatusOK, commuteRuleResponse{
				RuleID:  existingRule.ID,
				Created: false,
				Message: "Roundtrip route added to existing commute rule",
			})
			return
		}

		branches := makeCommuteRoundtripBranches("if", startLat, startLon, destLat, destLon, commuteRadiusM, pendelTagID)
		ruleID, err := createCommuteRuleRaw(ctx, h.DB, userID, branches)
		if err != nil {
			slog.Error("commute rule: createCommuteRuleRaw failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create rule", r.URL.Path)
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, commuteRuleResponse{
			RuleID:  ruleID,
			Created: true,
			Message: "Roundtrip commute rule created",
		})
		return
	}

	pendelTagID, err := h.DB.GetSystemTagID(ctx, userID, commuteTagName)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "Pendeln tag not found — check enrichment settings", r.URL.Path)
		return
	}

	existingRule, err := h.DB.GetRuleBySystemType(ctx, userID, commuteAutoSystemType)
	if err != nil {
		slog.Error("commute rule: GetRuleBySystemType failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load rule", r.URL.Path)
		return
	}

	if existingRule != nil {
		if commuteRouteExists(existingRule, startLat, startLon, endLat, endLon, commuteRadiusM) {
			httputil.WriteJSON(w, http.StatusConflict, commuteRuleResponse{
				RuleID:  existingRule.ID,
				Created: false,
				Message: "Route already exists in commute rule",
			})
			return
		}
		if err := extendCommuteRule(ctx, h.DB, existingRule, startLat, startLon, endLat, endLon, commuteRadiusM, pendelTagID); err != nil {
			if err.Error() == "max 20 commute routes reached" {
				httputil.WriteError(w, http.StatusUnprocessableEntity, "Limit Reached", "max 20 commute routes — delete old routes in /workshop/rules", r.URL.Path)
			} else {
				slog.Error("commute rule: extendCommuteRule failed", "error", err)
				httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to extend rule", r.URL.Path)
			}
			return
		}
		httputil.WriteJSON(w, http.StatusOK, commuteRuleResponse{
			RuleID:  existingRule.ID,
			Created: false,
			Message: "Route added to existing commute rule",
		})
		return
	}

	ruleID, err := createCommuteRule(ctx, h.DB, userID, startLat, startLon, endLat, endLon, commuteRadiusM, pendelTagID)
	if err != nil {
		slog.Error("commute rule: createCommuteRule failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create rule", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, commuteRuleResponse{
		RuleID:  ruleID,
		Created: true,
		Message: "Commute rule created",
	})
}

// commuteRouteExists returns true if a branch matching the given start/end coordinates
// (within threshold meters, forward or reverse) already exists in the rule.
func commuteRouteExists(rule *model.AutomationRule, startLat, startLon, endLat, endLon, threshold float64) bool {
	if !rule.HasBranches() {
		return false
	}
	type condition struct {
		Field string         `json:"field"`
		Value map[string]any `json:"value"`
	}
	type condGroup struct {
		Conditions []condition `json:"conditions"`
	}
	type branch struct {
		ConditionGroups []condGroup `json:"condition_groups"`
	}
	var branches []branch
	if err := json.Unmarshal(rule.Branches, &branches); err != nil {
		return false
	}

	for _, b := range branches {
		if len(b.ConditionGroups) == 0 {
			continue
		}
		var bStartLat, bStartLon, bEndLat, bEndLon float64
		var hasStart, hasEnd bool
		for _, c := range b.ConditionGroups[0].Conditions {
			if c.Field != "geofence" {
				continue
			}
			check, _ := c.Value["check"].(string)
			lat, _ := c.Value["lat"].(float64)
			lon, _ := c.Value["lon"].(float64)
			if check == "start" {
				bStartLat, bStartLon = lat, lon
				hasStart = true
			} else if check == "end" {
				bEndLat, bEndLon = lat, lon
				hasEnd = true
			}
		}
		if !hasStart || !hasEnd {
			continue
		}
		// Forward match.
		if geo.DistanceHaversine(startLat, startLon, bStartLat, bStartLon) <= threshold &&
			geo.DistanceHaversine(endLat, endLon, bEndLat, bEndLon) <= threshold {
			return true
		}
		// Reverse match.
		if geo.DistanceHaversine(startLat, startLon, bEndLat, bEndLon) <= threshold &&
			geo.DistanceHaversine(endLat, endLon, bStartLat, bStartLon) <= threshold {
			return true
		}
	}
	return false
}

// makeCommuteBranches builds a pair of branches (branchType + "else_if") for A→B and B→A.
func makeCommuteBranches(branchType string, startLat, startLon, endLat, endLon, radiusM float64, tagID string) json.RawMessage {
	makeBranch := func(t string, sLat, sLon, eLat, eLon float64) map[string]any {
		return map[string]any{
			"type": t,
			"condition_groups": []map[string]any{{
				"logic": "and",
				"conditions": []map[string]any{
					{
						"field": "geofence",
						"op":    "eq",
						"value": map[string]any{
							"check": "start", "mode": "inside", "shape": "circle",
							"lat": sLat, "lon": sLon, "radius_m": radiusM,
						},
					},
					{
						"field": "geofence",
						"op":    "eq",
						"value": map[string]any{
							"check": "end", "mode": "inside", "shape": "circle",
							"lat": eLat, "lon": eLon, "radius_m": radiusM,
						},
					},
				},
			}},
			"actions": []map[string]any{{
				"action": "add_tag",
				"params": map[string]any{"tag_id": tagID},
			}},
		}
	}
	branches := []map[string]any{
		makeBranch(branchType, startLat, startLon, endLat, endLon),
		makeBranch("else_if", endLat, endLon, startLat, startLon),
	}
	data, _ := json.Marshal(branches)
	return data
}

// createCommuteRule inserts a new commute_auto system rule and returns its ID.
func createCommuteRule(ctx context.Context, db *database.DB, userID string, startLat, startLon, endLat, endLon, radiusM float64, tagID string) (string, error) {
	branches := makeCommuteBranches("if", startLat, startLon, endLat, endLon, radiusM, tagID)
	var ruleID string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO automation_rules
			(user_id, name, is_system, system_type, is_active, priority,
			 trigger_event, conditions, conditions_logic, actions, branches, condition_groups)
		VALUES ($1, 'Pendeln', true, 'commute_auto', true, 0,
			'on_enrichment_complete', '[]'::jsonb, 'and', '[]'::jsonb, $2::jsonb, '[]'::jsonb)
		RETURNING id`,
		userID, branches,
	).Scan(&ruleID)
	return ruleID, err
}

// extendCommuteRule appends a new route pair to the branches of an existing commute_auto rule.
func extendCommuteRule(ctx context.Context, db *database.DB, rule *model.AutomationRule, startLat, startLon, endLat, endLon, radiusM float64, tagID string) error {
	newBranches := makeCommuteBranches("else_if", startLat, startLon, endLat, endLon, radiusM, tagID)
	return extendCommuteRuleRaw(ctx, db, rule, newBranches)
}

// extendCommuteRuleRaw appends pre-built branch JSON to an existing commute_auto rule.
func extendCommuteRuleRaw(ctx context.Context, db *database.DB, rule *model.AutomationRule, newBranches json.RawMessage) error {
	var existing []json.RawMessage
	if err := json.Unmarshal(rule.Branches, &existing); err != nil {
		return err
	}
	if len(existing) >= 40 { // 20 routes × 2 branches (roundtrip uses 3)
		return fmt.Errorf("max 20 commute routes reached")
	}
	var newParsed []json.RawMessage
	if err := json.Unmarshal(newBranches, &newParsed); err != nil {
		return err
	}
	all := append(existing, newParsed...)
	allJSON, err := json.Marshal(all)
	if err != nil {
		return err
	}
	_, err = db.Pool.Exec(ctx,
		`UPDATE automation_rules SET branches = $1::jsonb WHERE id = $2`,
		allJSON, rule.ID)
	return err
}

// createCommuteRuleRaw inserts a new commute_auto system rule with pre-built branches JSON.
func createCommuteRuleRaw(ctx context.Context, db *database.DB, userID string, branches json.RawMessage) (string, error) {
	var ruleID string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO automation_rules
			(user_id, name, is_system, system_type, is_active, priority,
			 trigger_event, conditions, conditions_logic, actions, branches, condition_groups)
		VALUES ($1, 'Pendeln', true, 'commute_auto', true, 0,
			'on_enrichment_complete', '[]'::jsonb, 'and', '[]'::jsonb, $2::jsonb, '[]'::jsonb)
		RETURNING id`,
		userID, branches,
	).Scan(&ruleID)
	return ruleID, err
}

// makeCommuteRoundtripBranches builds 3 branches for a roundtrip commute:
// home→work, work→home, and home→(dwell at work)→home.
func makeCommuteRoundtripBranches(branchType string, homeLat, homeLon, workLat, workLon, radiusM float64, tagID string) json.RawMessage {
	makeBranch := func(t string, conditions []map[string]any) map[string]any {
		return map[string]any{
			"type": t,
			"condition_groups": []map[string]any{{
				"logic":      "and",
				"conditions": conditions,
			}},
			"actions": []map[string]any{{
				"action": "add_tag",
				"params": map[string]any{"tag_id": tagID},
			}},
		}
	}

	gf := func(check string, lat, lon, r float64) map[string]any {
		return map[string]any{
			"field": "geofence",
			"op":    "eq",
			"value": map[string]any{
				"check": check, "mode": "inside", "shape": "circle",
				"lat": lat, "lon": lon, "radius_m": r,
			},
		}
	}

	dwellCond := map[string]any{
		"field": "geofence",
		"op":    "eq",
		"value": map[string]any{
			"check": "dwell", "mode": "inside", "shape": "circle",
			"lat": workLat, "lon": workLon, "radius_m": radiusM,
			"min_dwell_s": 1800, // 30 minutes
		},
	}

	branches := []map[string]any{
		makeBranch(branchType, []map[string]any{
			gf("start", homeLat, homeLon, radiusM),
			gf("end", workLat, workLon, radiusM),
		}),
		makeBranch("else_if", []map[string]any{
			gf("start", workLat, workLon, radiusM),
			gf("end", homeLat, homeLon, radiusM),
		}),
		makeBranch("else_if", []map[string]any{
			gf("start", homeLat, homeLon, radiusM),
			gf("end", homeLat, homeLon, radiusM),
			dwellCond,
		}),
	}
	data, _ := json.Marshal(branches)
	return data
}
