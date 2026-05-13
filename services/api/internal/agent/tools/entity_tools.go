package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type resolveEntityArgs struct {
	EntityType string `json:"entity_type"`
	Query      string `json:"query"`
	Limit      *int   `json:"limit,omitempty"`
}

type findGearArgs struct {
	Query          string `json:"query"`
	Type           string `json:"type,omitempty"`
	IncludeRetired bool   `json:"include_retired,omitempty"`
	Limit          *int   `json:"limit,omitempty"`
}

type backgroundJobStatusArgs struct {
	JobID string `json:"job_id"`
}

func EntityTools() []*Tool {
	return []*Tool{
		ResolveEntityTool(),
		FindGearTool(),
		GetBackgroundJobStatusTool(),
	}
}

func RegisterEntityTools(reg *Registry) {
	for _, tool := range EntityTools() {
		reg.Register(tool)
	}
}

func ResolveEntityTool() *Tool {
	return &Tool{
		Name:        "resolve_entity",
		Description: "Resolve a user-owned activity, gear item, tag, planned route, or segment by partial name before using an ID-based tool.",
		InputSchema: json.RawMessage(`{"type":"object","required":["entity_type","query"],"properties":{"entity_type":{"type":"string","enum":["activity","gear","tag","route","segment"]},"query":{"type":"string","minLength":1},"limit":{"type":"integer","minimum":1,"maximum":20}},"additionalProperties":false}`),
		Permission:  PermissionRead,
		IsUserOwned: true,
		ExecuteRead: executeResolveEntity,
	}
}

func FindGearTool() *Tool {
	return &Tool{
		Name:        "find_gear",
		Description: "Find user-owned gear by name and optional type, returning IDs suitable for gear write tools.",
		InputSchema: json.RawMessage(`{"type":"object","required":["query"],"properties":{"query":{"type":"string","minLength":1},"type":{"type":"string"},"include_retired":{"type":"boolean"},"limit":{"type":"integer","minimum":1,"maximum":20}},"additionalProperties":false}`),
		Permission:  PermissionRead,
		IsUserOwned: true,
		ExecuteRead: executeFindGear,
	}
}

func GetBackgroundJobStatusTool() *Tool {
	return &Tool{
		Name:        "get_background_job_status",
		Description: "Read the status of a user-owned background job by ID.",
		InputSchema: json.RawMessage(`{"type":"object","required":["job_id"],"properties":{"job_id":{"type":"string","format":"uuid"}},"additionalProperties":false}`),
		Permission:  PermissionRead,
		IsUserOwned: true,
		ExecuteRead: executeGetBackgroundJobStatus,
	}
}

func executeResolveEntity(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args resolveEntityArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode resolve_entity args: %w", err)
	}
	limit := boundedLimit(args.Limit, 10, 20)
	query := "%" + strings.TrimSpace(args.Query) + "%"
	var sql string
	var params []any
	switch args.EntityType {
	case "activity":
		sql = `
			SELECT id::text, COALESCE(title, sport_type) AS name, 'activity' AS entity_type,
			       started_at, sport_type, distance_m
			FROM activities
			WHERE user_id = $1 AND COALESCE(title, '') ILIKE $2
			ORDER BY started_at DESC
			LIMIT $3`
		params = []any{userID, query, limit}
	case "gear":
		sql = `
			SELECT id::text, name, 'gear' AS entity_type, type, status, is_retired
			FROM gear
			WHERE user_id = $1 AND name ILIKE $2
			ORDER BY is_retired ASC, name ASC
			LIMIT $3`
		params = []any{userID, query, limit}
	case "tag":
		sql = `
			SELECT id::text, name, 'tag' AS entity_type, category, activity_count
			FROM tags
			WHERE user_id = $1 AND name ILIKE $2
			ORDER BY name ASC
			LIMIT $3`
		params = []any{userID, query, limit}
	case "route":
		sql = `
			SELECT id::text, name, 'route' AS entity_type, sport_type, sub_type, total_distance_m AS distance_m
			FROM planned_routes
			WHERE user_id = $1 AND name ILIKE $2
			ORDER BY updated_at DESC
			LIMIT $3`
		params = []any{userID, query, limit}
	case "segment":
		sql = `
			SELECT DISTINCT s.id::text, s.name, 'segment' AS entity_type, s.sport_group, s.distance_m, s.effort_count
			FROM segments s
			LEFT JOIN segment_efforts se ON se.segment_id = s.id AND se.user_id = $1
			WHERE s.name ILIKE $2
			  AND (s.created_by = $1 OR s.visibility = 'public' OR se.id IS NOT NULL)
			ORDER BY effort_count DESC NULLS LAST, name ASC
			LIMIT $3`
		params = []any{userID, query, limit}
	default:
		return nil, fmt.Errorf("unsupported entity_type: %s", args.EntityType)
	}
	rows, err := queryRows(ctx, deps, sql, params...)
	if err != nil {
		return nil, err
	}
	return map[string]any{"entity_type": args.EntityType, "query": strings.TrimSpace(args.Query), "matches": rows, "count": len(rows)}, nil
}

func executeFindGear(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args findGearArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode find_gear args: %w", err)
	}
	limit := boundedLimit(args.Limit, 10, 20)
	sql := `
		SELECT id::text, name, type, scope, status, is_retired,
		       total_distance_m, total_time_s, total_activities
		FROM gear
		WHERE user_id = $1
		  AND name ILIKE $2
		  AND ($3 = '' OR type = $3 OR type LIKE ($3 || '.%'))
		  AND ($4::boolean OR COALESCE(is_retired, false) = false)
		ORDER BY is_retired ASC, name ASC
		LIMIT $5`
	rows, err := queryRows(ctx, deps, sql, userID, "%"+strings.TrimSpace(args.Query)+"%", strings.TrimSpace(args.Type), args.IncludeRetired, limit)
	if err != nil {
		return nil, err
	}
	return map[string]any{"gear": rows, "count": len(rows)}, nil
}

func executeGetBackgroundJobStatus(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args backgroundJobStatusArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode get_background_job_status args: %w", err)
	}
	sql := `
		SELECT id::text, job_type, entity_id::text, entity_name, status, phase,
		       progress_current, progress_total, started_at, finished_at, error
		FROM background_jobs
		WHERE user_id = $1 AND id = $2`
	rows, err := queryRows(ctx, deps, sql, userID, args.JobID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return map[string]any{"job_id": args.JobID, "found": false}, nil
	}
	return map[string]any{"found": true, "job": rows[0]}, nil
}

func boundedLimit(value *int, fallback, max int) int {
	if value == nil {
		return fallback
	}
	if *value < 1 {
		return fallback
	}
	if *value > max {
		return max
	}
	return *value
}
