package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type queryActivityTerrainSummaryArgs struct {
	ActivityID string  `json:"activity_id,omitempty"`
	TitleQuery string  `json:"title_query,omitempty"`
	DateFrom   string  `json:"date_from,omitempty"`
	DateTo     string  `json:"date_to,omitempty"`
	SportQuery string  `json:"sport_query,omitempty"`
	SportType  string  `json:"sport_type,omitempty"`
	SubType    *string `json:"sub_type,omitempty"`
	Days       *int    `json:"days,omitempty"`
	AllHistory bool    `json:"all_history,omitempty"`
	Limit      *int    `json:"limit,omitempty"`
	IsElectric *bool   `json:"is_electric,omitempty"`
	IsIndoor   *bool   `json:"is_indoor,omitempty"`
	IsVirtual  *bool   `json:"is_virtual,omitempty"`
	IsCommute  *bool   `json:"is_commute,omitempty"`
}

type queryWeatherAtPointTimeArgs struct {
	ActivityID string   `json:"activity_id,omitempty"`
	AtTime     string   `json:"at_time,omitempty"`
	OffsetS    *float64 `json:"offset_s,omitempty"`
	Lat        *float64 `json:"lat,omitempty"`
	Lng        *float64 `json:"lng,omitempty"`
	Time       string   `json:"time,omitempty"`
	StoredOnly *bool    `json:"stored_only,omitempty"`
	Limit      *int     `json:"limit,omitempty"`
}

func RegisterActivityContextTools(reg *Registry) {
	for _, tool := range ActivityContextTools() {
		reg.Register(tool)
	}
}

func ActivityContextTools() []*Tool {
	return []*Tool{
		QueryActivityTerrainSummaryTool(),
		QueryWeatherAtPointTimeTool(),
	}
}

func QueryActivityTerrainSummaryTool() *Tool {
	return &Tool{
		Name:        "query_activity_terrain_summary",
		Description: "Return stored terrain, elevation, grade, and environmental context for owned activities.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"activity_id":{"type":"string","format":"uuid"},"title_query":{"type":"string"},"date_from":{"type":"string","format":"date"},"date_to":{"type":"string","format":"date"},"sport_query":{"type":"string","description":"Natural sport phrase to resolve via Skadia taxonomy"},"sport_type":{"type":"string"},"sub_type":{"type":"string"},"days":{"type":"integer","minimum":1},"all_history":{"type":"boolean"},"limit":{"type":"integer","minimum":1,"maximum":50},"is_electric":{"type":"boolean"},"is_indoor":{"type":"boolean"},"is_virtual":{"type":"boolean"},"is_commute":{"type":"boolean"}},"additionalProperties":false}`),
		Permission:  PermissionRead,
		IsUserOwned: true,
		ExecuteRead: executeQueryActivityTerrainSummary,
	}
}

func QueryWeatherAtPointTimeTool() *Tool {
	return &Tool{
		Name:        "query_weather_at_point_time",
		Description: "Return stored weather for an owned activity near a requested point/time. External weather fetches are not performed.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"activity_id":{"type":"string","format":"uuid"},"at_time":{"type":"string","format":"date-time"},"offset_s":{"type":"number","minimum":0},"lat":{"type":"number","minimum":-90,"maximum":90},"lng":{"type":"number","minimum":-180,"maximum":180},"time":{"type":"string","format":"date-time"},"stored_only":{"type":"boolean","default":true},"limit":{"type":"integer","minimum":1,"maximum":20}},"additionalProperties":false}`),
		Permission:  PermissionRead,
		IsUserOwned: true,
		ExecuteRead: executeQueryWeatherAtPointTime,
	}
}

func executeQueryActivityTerrainSummary(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args queryActivityTerrainSummaryArgs
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode query_activity_terrain_summary args: %w", err)
	}
	sql, params, criteria, err := buildActivityTerrainSummarySQL(userID, args, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	rows, err := queryActivityContextRows(ctx, deps, userID, sql, params...)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"activities": rows,
		"count":      len(rows),
		"criteria":   criteria,
		"status":     terrainSummaryStatus(rows),
	}, nil
}

func executeQueryWeatherAtPointTime(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args queryWeatherAtPointTimeArgs
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode query_weather_at_point_time args: %w", err)
	}
	if args.ActivityID == "" {
		return map[string]any{
			"status": "unsupported_no_stored_activity",
			"note":   "stored_only is the only supported mode; explicit coordinates are not fetched from external providers",
		}, nil
	}
	sql, params, err := buildWeatherAtPointTimeSQL(userID, args)
	if err != nil {
		return nil, err
	}
	rows, err := queryActivityContextRows(ctx, deps, userID, sql, params...)
	if err != nil {
		return nil, err
	}
	status := "ok"
	if len(rows) == 0 {
		status = "no_stored_weather"
	}
	return map[string]any{
		"weather": rows,
		"count":   len(rows),
		"status":  status,
	}, nil
}

func buildActivityTerrainSummarySQL(userID string, args queryActivityTerrainSummaryArgs, now time.Time) (string, []any, map[string]any, error) {
	criteria := activitySelectionCriteria{
		SportQuery:  args.SportQuery,
		SportType:   args.SportType,
		SubType:     args.SubType,
		Days:        args.Days,
		DateFrom:    args.DateFrom,
		DateTo:      args.DateTo,
		TitleSearch: args.TitleQuery,
		IsElectric:  args.IsElectric,
		IsIndoor:    args.IsIndoor,
		IsVirtual:   args.IsVirtual,
		IsCommute:   args.IsCommute,
		AllHistory:  args.AllHistory,
	}
	if args.ActivityID != "" {
		parsed, err := uuid.Parse(args.ActivityID)
		if err != nil {
			return "", nil, nil, fmt.Errorf("invalid activity_id: %w", err)
		}
		criteria.ActivityIDs = []string{parsed.String()}
		criteria.AllHistory = true
	}
	plan, err := buildActivitySelectionPlan(userID, criteria, now)
	if err != nil {
		return "", nil, nil, err
	}
	limit := clampIntPtr(args.Limit, 10, 1, 50)
	sql := `WITH selected AS (
	SELECT a.id, a.title, a.sport_type, a.sub_type, a.started_at,
	       a.distance_m, a.elevation_gain_m, a.elevation_loss_m, a.min_elevation_m, a.max_elevation_m,
	       a.avg_grade_pct, a.avg_pos_grade_pct, a.avg_neg_grade_pct, a.max_pos_grade_pct, a.max_neg_grade_pct
	FROM activities a
	WHERE ` + strings.Join(plan.Conditions, " AND ") + fmt.Sprintf(`
	ORDER BY a.started_at DESC NULLS LAST, a.id
	LIMIT %d
)
SELECT s.id::text AS activity_id, s.title, s.sport_type, s.sub_type, s.started_at,
       s.distance_m, s.elevation_gain_m, s.elevation_loss_m, s.min_elevation_m, s.max_elevation_m,
       s.avg_grade_pct, s.avg_pos_grade_pct, s.avg_neg_grade_pct, s.max_pos_grade_pct, s.max_neg_grade_pct,
       COALESCE((SELECT jsonb_agg(jsonb_build_object('type', e.type, 'percentage', e.percentage, 'length_m', e.length_m, 'ranges', e.ranges) ORDER BY e.percentage DESC, e.type)
                 FROM activity_enrichments e WHERE e.activity_id = s.id AND e.category = 'surface'), '[]'::jsonb) AS surfaces,
       COALESCE((SELECT jsonb_agg(jsonb_build_object('type', e.type, 'percentage', e.percentage, 'length_m', e.length_m, 'ranges', e.ranges) ORDER BY e.percentage DESC, e.type)
                 FROM activity_enrichments e WHERE e.activity_id = s.id AND e.category = 'highway'), '[]'::jsonb) AS highways,
       COALESCE((SELECT jsonb_agg(jsonb_build_object('category', e.category, 'type', e.type, 'percentage', e.percentage, 'ranges', e.ranges) ORDER BY e.category, e.percentage DESC, e.type)
                 FROM activity_enrichments e WHERE e.activity_id = s.id AND e.category IN ('sac_scale','mtb_scale','vf_scale','vf_unknown')), '[]'::jsonb) AS difficulty,
       (SELECT e.percentage FROM activity_enrichments e WHERE e.activity_id = s.id AND e.category = 'forest_pct' AND e.type = '') AS forest_pct,
       (SELECT e.percentage FROM activity_enrichments e WHERE e.activity_id = s.id AND e.category = 'protected_pct' AND e.type = '') AS protected_pct,
       (SELECT COUNT(*) FROM activity_geo_tags gt WHERE gt.activity_id = s.id AND gt.category IN ('still_water','flowing_water')) AS water_feature_count,
       (SELECT jsonb_build_object('path', m.path, 'resample_spacing_m', m.resample_spacing_m, 'computed_at', m.computed_at)
          FROM activity_enrichment_meta m WHERE m.activity_id = s.id) AS enrichment_meta,
       COALESCE((SELECT jsonb_object_agg(category, count ORDER BY category)
                 FROM (
                   SELECT e.category, COUNT(*) AS count
                   FROM activity_enrichments e
                   WHERE e.activity_id = s.id
                   GROUP BY e.category
                 ) counts), '{}'::jsonb) AS enrichment_categories,
       COALESCE((SELECT jsonb_agg(jsonb_build_object('category', gt.category, 'name', gt.name, 'source', gt.source, 'distance_m', gt.dist_m, 'length_m', gt.length_m) ORDER BY gt.category, gt.dist_m NULLS LAST, gt.name)
                 FROM activity_geo_tags gt
                 WHERE gt.activity_id = s.id AND gt.category IN ('summit','pass','still_water','flowing_water','forest')), '[]'::jsonb) AS geo_context,
       jsonb_build_object(
         'surfaces', EXISTS (SELECT 1 FROM activity_enrichments e WHERE e.activity_id = s.id AND e.category = 'surface'),
         'highways', EXISTS (SELECT 1 FROM activity_enrichments e WHERE e.activity_id = s.id AND e.category = 'highway'),
         'forest', EXISTS (SELECT 1 FROM activity_enrichments e WHERE e.activity_id = s.id AND e.category = 'forest_pct'),
         'protected', EXISTS (SELECT 1 FROM activity_enrichments e WHERE e.activity_id = s.id AND e.category = 'protected_pct'),
         'water', EXISTS (SELECT 1 FROM activity_geo_tags gt WHERE gt.activity_id = s.id AND gt.category IN ('still_water','flowing_water')),
         'noise', false,
         'traffic', false,
         'note', 'noise and traffic are not stored per activity in activity_enrichments'
       ) AS data_status
FROM selected s`, limit)
	return sql, plan.Params, plan.CriteriaForResponse, nil
}

func buildWeatherAtPointTimeSQL(userID string, args queryWeatherAtPointTimeArgs) (string, []any, error) {
	activityID, err := uuid.Parse(args.ActivityID)
	if err != nil {
		return "", nil, fmt.Errorf("invalid activity_id: %w", err)
	}
	limit := clampIntPtr(args.Limit, 5, 1, 20)
	params := []any{userID, activityID.String()}
	targetExpr := "NULL::timestamptz"
	if args.AtTime != "" {
		at, err := time.Parse(time.RFC3339, args.AtTime)
		if err != nil {
			return "", nil, fmt.Errorf("invalid at_time %q: %w", args.AtTime, err)
		}
		params = append(params, at)
		targetExpr = fmt.Sprintf("$%d::timestamptz", len(params))
	} else if args.Time != "" {
		at, err := time.Parse(time.RFC3339, args.Time)
		if err != nil {
			return "", nil, fmt.Errorf("invalid time %q: %w", args.Time, err)
		}
		params = append(params, at)
		targetExpr = fmt.Sprintf("$%d::timestamptz", len(params))
	} else if args.OffsetS != nil {
		params = append(params, *args.OffsetS)
		targetExpr = fmt.Sprintf("a.started_at + ($%d::double precision * interval '1 second')", len(params))
	}
	params = append(params, optionalFloatValue(args.Lng), optionalFloatValue(args.Lat))
	targetLngExpr := fmt.Sprintf("$%d::double precision", len(params)-1)
	targetLatExpr := fmt.Sprintf("$%d::double precision", len(params))
	targetPointExpr := fmt.Sprintf(`CASE
       WHEN %s IS NOT NULL AND %s IS NOT NULL THEN ST_SetSRID(ST_Point(%s, %s), 4326)::geography
       WHEN ts.lng IS NOT NULL AND ts.lat IS NOT NULL THEN ST_SetSRID(ST_Point(ts.lng, ts.lat), 4326)::geography
       ELSE NULL::geography
       END`, targetLngExpr, targetLatExpr, targetLngExpr, targetLatExpr)
	sql := fmt.Sprintf(`SELECT aw.id::text AS weather_id, aw.activity_id::text AS activity_id, a.title AS activity_title,
       aw.reading_type, aw.label, aw.recorded_at, aw.lat, aw.lng, aw.elevation_m,
       aw.temp_c, aw.apparent_temp_c, aw.precipitation_mm, aw.wind_speed_ms, aw.wind_gusts_ms,
       aw.humidity, aw.uv_index, aw.freezing_level_m, aw.condition, aw.tags,
       ts.time AS nearest_stream_time, ts.lat AS nearest_stream_lat, ts.lng AS nearest_stream_lng,
       CASE WHEN %s IS NULL OR aw.recorded_at IS NULL THEN NULL
            ELSE ABS(EXTRACT(EPOCH FROM (aw.recorded_at - %s))) END AS seconds_from_target,
       CASE WHEN %s IS NULL OR aw.lat IS NULL OR aw.lng IS NULL THEN NULL
            ELSE ST_Distance(ST_SetSRID(ST_Point(aw.lng, aw.lat), 4326)::geography, %s) END AS distance_from_target_m
FROM activity_weather aw
JOIN activities a ON a.id = aw.activity_id
LEFT JOIN LATERAL (
  SELECT s.time, s.lat, s.lng
  FROM activity_streams s
  WHERE s.activity_id = a.id AND s.lat IS NOT NULL AND s.lng IS NOT NULL
  ORDER BY CASE WHEN %s IS NULL THEN 1 ELSE 0 END,
           ABS(EXTRACT(EPOCH FROM (s.time - %s))) ASC NULLS LAST,
           s.time ASC
  LIMIT 1
) ts ON true
WHERE a.user_id = $1 AND a.id::text = $2
ORDER BY
  CASE WHEN %s IS NULL OR aw.recorded_at IS NULL THEN 1 ELSE 0 END,
  seconds_from_target ASC NULLS LAST,
  distance_from_target_m ASC NULLS LAST,
  aw.recorded_at ASC NULLS LAST,
  aw.reading_type
LIMIT %d`, targetExpr, targetExpr, targetPointExpr, targetPointExpr, targetExpr, targetExpr, targetExpr, limit)
	return sql, params, nil
}

func queryActivityContextRows(ctx context.Context, deps *Deps, userID, sql string, args ...any) ([]map[string]any, error) {
	if deps != nil && deps.DB != nil {
		var out []map[string]any
		err := deps.DB.WithUserTx(ctx, userID, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, sql, args...)
			if err != nil {
				return err
			}
			defer rows.Close()
			out, err = collectToolRows(rows)
			return err
		})
		return out, err
	}
	return queryRows(ctx, deps, sql, args...)
}

func terrainSummaryStatus(rows []map[string]any) string {
	if len(rows) == 0 {
		return "no_matching_activity"
	}
	for _, row := range rows {
		if row["elevation_gain_m"] != nil ||
			hasJSONContent(row["surfaces"]) ||
			hasJSONContent(row["highways"]) ||
			hasJSONContent(row["difficulty"]) ||
			row["forest_pct"] != nil ||
			row["protected_pct"] != nil ||
			isPositiveNumber(row["water_feature_count"]) {
			return "ok"
		}
	}
	return "missing_terrain_data"
}

func hasJSONContent(v any) bool {
	switch raw := v.(type) {
	case []byte:
		return len(strings.TrimSpace(string(raw))) > 2
	case string:
		return len(strings.TrimSpace(raw)) > 2
	case json.RawMessage:
		return len(strings.TrimSpace(string(raw))) > 2
	default:
		return v != nil
	}
}

func isPositiveNumber(v any) bool {
	switch n := v.(type) {
	case int:
		return n > 0
	case int32:
		return n > 0
	case int64:
		return n > 0
	case float32:
		return n > 0
	case float64:
		return n > 0
	default:
		return false
	}
}

func optionalFloatValue(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}
