package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/automation"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

type mergeTagsArgs struct {
	SourceTagID string `json:"source_tag_id"`
	TargetTagID string `json:"target_tag_id"`
}

type applyAutomationRuleRetroArgs struct {
	RuleID string `json:"rule_id"`
}

type startRecomputeActivitiesArgs struct {
	ActivityIDs []string `json:"activity_ids"`
	Scope       []string `json:"scope,omitempty"`
}

type savePlannedRouteArgs struct {
	PreviewToken       string          `json:"preview_token"`
	Name               string          `json:"name"`
	Description        *string         `json:"description,omitempty"`
	SportType          string          `json:"sport_type"`
	SubType            *string         `json:"sub_type,omitempty"`
	IsElectric         bool            `json:"is_electric,omitempty"`
	IsIndoor           bool            `json:"is_indoor,omitempty"`
	IsVirtual          bool            `json:"is_virtual,omitempty"`
	IsCommute          bool            `json:"is_commute,omitempty"`
	Difficulty         *string         `json:"difficulty,omitempty"`
	Profile            string          `json:"profile"`
	GeometryGeoJSON    string          `json:"geometry_geojson"`
	Waypoints          json.RawMessage `json:"waypoints,omitempty"`
	Segments           json.RawMessage `json:"segments,omitempty"`
	TotalDistanceM     *float64        `json:"total_distance_m,omitempty"`
	TotalElevationGain *float64        `json:"total_elevation_gain,omitempty"`
	TotalElevationLoss *float64        `json:"total_elevation_loss,omitempty"`
	EstimatedTimeMin   *int            `json:"estimated_time_min,omitempty"`
	ElevationProfile   json.RawMessage `json:"elevation_profile,omitempty"`
	Enrichment         json.RawMessage `json:"enrichment,omitempty"`
	RoutingEngine      *string         `json:"routing_engine,omitempty"`
}

func MiscWriteTools() []*Tool {
	return []*Tool{
		MergeTagsTool(),
		ApplyAutomationRuleRetroactivelyTool(),
		StartRecomputeActivitiesTool(),
		SavePlannedRouteTool(),
	}
}

func RegisterMiscWriteTools(reg *Registry) {
	for _, tool := range MiscWriteTools() {
		reg.Register(tool)
	}
}

func MergeTagsTool() *Tool {
	return &Tool{
		Name:                 "merge_tags",
		Description:          "Merge one user-owned tag into another after user confirmation.",
		InputSchema:          json.RawMessage(`{"type":"object","required":["source_tag_id","target_tag_id"],"properties":{"source_tag_id":{"type":"string","format":"uuid"},"target_tag_id":{"type":"string","format":"uuid"}},"additionalProperties":false}`),
		Permission:           PermissionWriteDestructive,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeMergeTags,
		ExecuteWrite:         executeMergeTags,
	}
}

func ApplyAutomationRuleRetroactivelyTool() *Tool {
	return &Tool{
		Name:                 "apply_automation_rule_retroactively",
		Description:          "Apply one existing automation rule to all current user activities in a background job after confirmation.",
		InputSchema:          json.RawMessage(`{"type":"object","required":["rule_id"],"properties":{"rule_id":{"type":"string","format":"uuid"}},"additionalProperties":false}`),
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeApplyAutomationRuleRetroactively,
		ExecuteWrite:         executeApplyAutomationRuleRetroactively,
	}
}

func StartRecomputeActivitiesTool() *Tool {
	return &Tool{
		Name:                 "start_recompute_activities",
		Description:          "Start a background recompute for concrete activity IDs. This recomputes derived gear/lift data only; it does not re-import original files.",
		InputSchema:          json.RawMessage(`{"type":"object","required":["activity_ids"],"properties":{"activity_ids":{"type":"array","minItems":1,"maxItems":5000,"items":{"type":"string","format":"uuid"}},"scope":{"type":"array","items":{"type":"string","enum":["gear_usage","lift_stats"]}}},"additionalProperties":false}`),
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeStartRecomputeActivities,
		PrepareWrite:         prepareStartRecomputeActivities,
		ExecuteWrite:         executeStartRecomputeActivities,
	}
}

func SavePlannedRouteTool() *Tool {
	return &Tool{
		Name:                 "save_planned_route",
		Description:          "Save a planned route from a previous route preview after user confirmation.",
		InputSchema:          json.RawMessage(`{"type":"object","required":["preview_token","name","sport_type","profile","geometry_geojson"],"properties":{"preview_token":{"type":"string","format":"uuid"},"name":{"type":"string","minLength":1},"description":{"type":"string"},"sport_type":{"type":"string","minLength":1},"sub_type":{"type":"string"},"is_electric":{"type":"boolean"},"is_indoor":{"type":"boolean"},"is_virtual":{"type":"boolean"},"is_commute":{"type":"boolean"},"difficulty":{"type":"string"},"profile":{"type":"string","minLength":1},"geometry_geojson":{"type":"string","maxLength":500000},"waypoints":{"type":"array"},"segments":{"type":"array"},"total_distance_m":{"type":"number"},"total_elevation_gain":{"type":"number"},"total_elevation_loss":{"type":"number"},"estimated_time_min":{"type":"integer"},"elevation_profile":{},"enrichment":{},"routing_engine":{"type":"string"}},"additionalProperties":false}`),
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeSavePlannedRoute,
		PrepareWrite:         prepareSavePlannedRoute,
		ExecuteWrite:         executeSavePlannedRoute,
	}
}

func summarizeMergeTags(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (string, error) {
	var args mergeTagsArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}
	if args.SourceTagID == args.TargetTagID {
		return "", fmt.Errorf("source and target tag must differ")
	}
	source, err := deps.DB.GetTagByID(ctx, args.SourceTagID, userID)
	if err != nil || source == nil {
		return "", fmt.Errorf("source tag not found")
	}
	target, err := deps.DB.GetTagByID(ctx, args.TargetTagID, userID)
	if err != nil || target == nil {
		return "", fmt.Errorf("target tag not found")
	}
	return fmt.Sprintf("Tag '%s' in '%s' zusammenführen", source.Name, target.Name), nil
}

func executeMergeTags(ctx context.Context, deps *WriteExecutionDeps, userID string, raw json.RawMessage) (any, error) {
	var args mergeTagsArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	if args.SourceTagID == args.TargetTagID {
		return nil, fmt.Errorf("source and target tag must differ")
	}
	merged, err := deps.DB.MergeTagTx(ctx, deps.Tx, args.TargetTagID, args.SourceTagID, userID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"status": "ok", "target_tag_id": merged.ID, "source_tag_id": args.SourceTagID}, nil
}

func summarizeApplyAutomationRuleRetroactively(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (string, error) {
	var args applyAutomationRuleRetroArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}
	rule, err := deps.DB.GetRule(ctx, args.RuleID, userID)
	if err != nil || rule == nil {
		return "", fmt.Errorf("automation rule not found")
	}
	return fmt.Sprintf("Regel '%s' rückwirkend auf alle Aktivitäten anwenden", rule.Name), nil
}

func executeApplyAutomationRuleRetroactively(ctx context.Context, deps *WriteExecutionDeps, userID string, raw json.RawMessage) (any, error) {
	var args applyAutomationRuleRetroArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	rule, err := deps.DB.GetRule(ctx, args.RuleID, userID)
	if err != nil || rule == nil {
		return nil, fmt.Errorf("automation rule not found")
	}
	activityIDs, err := deps.DB.ListActivityIDsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	jobName := fmt.Sprintf("Regel '%s' anwenden", rule.Name)
	jobID := uuid.NewString()
	ruleCopy := *rule
	db := deps.DB
	if err := appendAfterCommit(deps, func(jobCtx context.Context) {
		if err := db.InsertBackgroundJobWithID(jobCtx, jobID, userID, "rule_apply", &args.RuleID, &jobName, int64(len(activityIDs))); err != nil {
			slog.Warn("agent apply automation rule: insert job failed", "job_id", jobID, "error", err)
			return
		}
		failed := false
		errText := ""
		for i, activityID := range activityIDs {
			if err := automation.ApplyRulesToActivity(jobCtx, db, activityID, userID, []model.AutomationRule{ruleCopy}, ""); err != nil {
				failed = true
				if errText == "" {
					errText = err.Error()
				}
				slog.Warn("agent apply automation rule failed", "rule_id", ruleCopy.ID, "activity_id", activityID, "error", err)
			}
			if i%50 == 0 || i == len(activityIDs)-1 {
				_ = db.UpdateBackgroundJobProgress(jobCtx, jobID, int64(i+1))
			}
		}
		_ = db.CompleteBackgroundJob(jobCtx, jobID, failed, errText)
	}); err != nil {
		return nil, err
	}
	return map[string]any{"status": "started", "job_id": jobID, "activity_count": len(activityIDs)}, nil
}

func summarizeStartRecomputeActivities(_ context.Context, _ *Deps, _ string, raw json.RawMessage) (string, error) {
	var args startRecomputeActivitiesArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d Aktivitäten neu berechnen", len(args.ActivityIDs)), nil
}

func prepareStartRecomputeActivities(_ context.Context, _ *Deps, _ string, raw json.RawMessage) (*PreparedWrite, error) {
	var args startRecomputeActivitiesArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	args.ActivityIDs = dedupeNonEmpty(args.ActivityIDs)
	if len(args.ActivityIDs) == 0 {
		return nil, fmt.Errorf("activity_ids required")
	}
	if len(args.ActivityIDs) > 5000 {
		return nil, fmt.Errorf("too many activities selected: %d > 5000", len(args.ActivityIDs))
	}
	if len(args.Scope) == 0 {
		args.Scope = []string{"gear_usage", "lift_stats"}
	}
	prepared, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	return &PreparedWrite{
		Summary: fmt.Sprintf("%d Aktivitäten neu berechnen", len(args.ActivityIDs)),
		Preview: map[string]any{
			"kind":           "activity_recompute",
			"activity_count": len(args.ActivityIDs),
			"scope":          args.Scope,
			"sample_ids":     sampleStrings(args.ActivityIDs, 10),
		},
		Args: prepared,
	}, nil
}

func executeStartRecomputeActivities(ctx context.Context, deps *WriteExecutionDeps, userID string, raw json.RawMessage) (any, error) {
	var args startRecomputeActivitiesArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	if len(args.ActivityIDs) == 0 || len(args.ActivityIDs) > 5000 {
		return nil, fmt.Errorf("invalid activity selection")
	}
	for _, id := range args.ActivityIDs {
		if err := requireOwnedActivity(ctx, deps, id, userID); err != nil {
			return nil, err
		}
	}
	jobName := "Agent-Aktivitäts-Recompute"
	jobID := uuid.NewString()
	db := deps.DB
	scope := recomputeScopeSet(args.Scope)
	ids := append([]string(nil), args.ActivityIDs...)
	if err := appendAfterCommit(deps, func(jobCtx context.Context) {
		if err := db.InsertBackgroundJobWithID(jobCtx, jobID, userID, "activity_recompute_all", nil, &jobName, int64(len(ids))); err != nil {
			slog.Warn("agent activity recompute: insert job failed", "job_id", jobID, "error", err)
			return
		}
		failed := false
		errText := ""
		for i, activityID := range ids {
			if scope["lift_stats"] {
				if err := db.RecomputeLiftStats(jobCtx, userID, activityID); err != nil {
					failed = true
					if errText == "" {
						errText = err.Error()
					}
				}
			}
			if scope["gear_usage"] {
				if _, err := db.RefreshGearUsageForActivityMetricChange(jobCtx, activityID, userID); err != nil {
					failed = true
					if errText == "" {
						errText = err.Error()
					}
				}
			}
			if i%20 == 0 || i == len(ids)-1 {
				_ = db.UpdateBackgroundJobProgress(jobCtx, jobID, int64(i+1))
			}
		}
		_ = db.CompleteBackgroundJob(jobCtx, jobID, failed, errText)
	}); err != nil {
		return nil, err
	}
	return map[string]any{"status": "started", "job_id": jobID, "activity_count": len(args.ActivityIDs)}, nil
}

func summarizeSavePlannedRoute(_ context.Context, _ *Deps, _ string, raw json.RawMessage) (string, error) {
	var args savePlannedRouteArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}
	return fmt.Sprintf("Route '%s' speichern", strings.TrimSpace(args.Name)), nil
}

func prepareSavePlannedRoute(_ context.Context, _ *Deps, userID string, raw json.RawMessage) (*PreparedWrite, error) {
	args, err := decodeSavePlannedRouteArgs(raw)
	if err != nil {
		return nil, err
	}
	snapshot, err := verifyRoutePreviewToken(userID, args.PreviewToken, args.GeometryGeoJSON, args.Profile)
	if err != nil {
		return nil, err
	}
	prepared, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	return &PreparedWrite{
		Summary: fmt.Sprintf("Route '%s' speichern", args.Name),
		Preview: map[string]any{
			"kind":             "planned_route",
			"name":             args.Name,
			"sport_type":       args.SportType,
			"sub_type":         args.SubType,
			"profile":          args.Profile,
			"preview_hash":     snapshot.Hash,
			"total_distance_m": args.TotalDistanceM,
		},
		Args: prepared,
	}, nil
}

func executeSavePlannedRoute(ctx context.Context, deps *WriteExecutionDeps, userID string, raw json.RawMessage) (any, error) {
	args, err := decodeSavePlannedRouteArgs(raw)
	if err != nil {
		return nil, err
	}
	if _, err := consumeRoutePreviewToken(userID, args.PreviewToken, args.GeometryGeoJSON, args.Profile); err != nil {
		return nil, err
	}
	route, err := args.toPlannedRoute(userID)
	if err != nil {
		return nil, err
	}
	routeID, err := deps.DB.CreateRouteTx(ctx, deps.Tx, route)
	if err != nil {
		return nil, err
	}
	return map[string]any{"status": "ok", "route_id": routeID}, nil
}

func decodeSavePlannedRouteArgs(raw json.RawMessage) (savePlannedRouteArgs, error) {
	var args savePlannedRouteArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return args, err
	}
	args.Name = strings.TrimSpace(args.Name)
	args.PreviewToken = strings.TrimSpace(args.PreviewToken)
	args.SportType = strings.TrimSpace(args.SportType)
	args.Profile = strings.TrimSpace(args.Profile)
	if args.PreviewToken == "" || args.Name == "" || args.SportType == "" || args.Profile == "" {
		return args, fmt.Errorf("preview_token, name, sport_type, and profile are required")
	}
	if err := validateRouteGeometry(args.GeometryGeoJSON); err != nil {
		return args, err
	}
	if len(args.Waypoints) == 0 {
		args.Waypoints = json.RawMessage(`[]`)
	}
	if len(args.Segments) == 0 {
		args.Segments = json.RawMessage(`[]`)
	}
	return args, nil
}

func (args savePlannedRouteArgs) toPlannedRoute(userID string) (*model.PlannedRoute, error) {
	var waypoints []model.RouteWaypoint
	if err := json.Unmarshal(args.Waypoints, &waypoints); err != nil {
		return nil, fmt.Errorf("decode waypoints: %w", err)
	}
	var segments []model.RouteSegment
	if err := json.Unmarshal(args.Segments, &segments); err != nil {
		return nil, fmt.Errorf("decode segments: %w", err)
	}
	return &model.PlannedRoute{
		UserID:             userID,
		Name:               args.Name,
		Description:        args.Description,
		SportType:          args.SportType,
		SubType:            args.SubType,
		IsElectric:         args.IsElectric,
		IsIndoor:           args.IsIndoor,
		IsVirtual:          args.IsVirtual,
		IsCommute:          args.IsCommute,
		Difficulty:         args.Difficulty,
		Profile:            args.Profile,
		GeometryGeoJSON:    args.GeometryGeoJSON,
		Waypoints:          waypoints,
		Segments:           segments,
		TotalDistanceM:     args.TotalDistanceM,
		TotalElevationGain: args.TotalElevationGain,
		TotalElevationLoss: args.TotalElevationLoss,
		EstimatedTimeMin:   args.EstimatedTimeMin,
		ElevationProfile:   args.ElevationProfile,
		Enrichment:         args.Enrichment,
		RoutingEngine:      args.RoutingEngine,
	}, nil
}

func appendAfterCommit(deps *WriteExecutionDeps, fn func(context.Context)) error {
	if deps == nil || deps.AfterCommit == nil {
		return fmt.Errorf("after-commit hooks are not configured")
	}
	*deps.AfterCommit = append(*deps.AfterCommit, fn)
	return nil
}

func recomputeScopeSet(scope []string) map[string]bool {
	out := map[string]bool{}
	if len(scope) == 0 {
		out["gear_usage"] = true
		out["lift_stats"] = true
		return out
	}
	for _, item := range scope {
		out[item] = true
	}
	return out
}

func dedupeNonEmpty(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func sampleStrings(in []string, limit int) []string {
	if len(in) <= limit {
		return append([]string(nil), in...)
	}
	return append([]string(nil), in[:limit]...)
}
