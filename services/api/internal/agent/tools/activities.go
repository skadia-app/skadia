package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var validActivitySorts = map[string]string{
	"date":      "started_at",
	"distance":  "distance_m",
	"duration":  "elapsed_time_s",
	"hr":        "avg_heart_rate",
	"elevation": "elevation_gain_m",
}

var metricUnits = map[string]string{
	"total_distance":  "km",
	"total_duration":  "h",
	"count":           "activities",
	"avg_heart_rate":  "bpm",
	"total_elevation": "m",
	"avg_distance":    "km",
}

var metricSQL = map[string]string{
	"total_distance":  "COALESCE(SUM(distance_m), 0) / 1000.0 AS value",
	"total_duration":  "COALESCE(SUM(elapsed_time_s), 0) / 3600.0 AS value",
	"count":           "COUNT(*) AS value",
	"avg_heart_rate":  "ROUND(AVG(avg_heart_rate)::numeric, 1)::float8 AS value",
	"total_elevation": "COALESCE(SUM(elevation_gain_m), 0) AS value",
	"avg_distance":    "ROUND((COALESCE(AVG(distance_m), 0) / 1000.0)::numeric, 2)::float8 AS value",
}

type queryActivitiesArgs struct {
	SportType     string   `json:"sport_type,omitempty"`
	SubType       *string  `json:"sub_type,omitempty"`
	Days          *int     `json:"days,omitempty"`
	Limit         *int     `json:"limit,omitempty"`
	SortBy        string   `json:"sort_by,omitempty"`
	MinDistanceKM *float64 `json:"min_distance_km,omitempty"`
	TitleSearch   string   `json:"title_search,omitempty"`
	GearID        string   `json:"gear_id,omitempty"`
	IsElectric    *bool    `json:"is_electric,omitempty"`
	IsIndoor      *bool    `json:"is_indoor,omitempty"`
	IsVirtual     *bool    `json:"is_virtual,omitempty"`
	IsCommute     *bool    `json:"is_commute,omitempty"`
}

type aggregateStatsArgs struct {
	Metric     string  `json:"metric,omitempty"`
	SportType  string  `json:"sport_type,omitempty"`
	SubType    *string `json:"sub_type,omitempty"`
	Days       *int    `json:"days,omitempty"`
	GroupBy    string  `json:"group_by,omitempty"`
	IsElectric *bool   `json:"is_electric,omitempty"`
	IsIndoor   *bool   `json:"is_indoor,omitempty"`
	IsVirtual  *bool   `json:"is_virtual,omitempty"`
	IsCommute  *bool   `json:"is_commute,omitempty"`
}

type comparePeriodsArgs struct {
	Period1Start string  `json:"period1_start"`
	Period1End   string  `json:"period1_end"`
	Period2Start string  `json:"period2_start"`
	Period2End   string  `json:"period2_end"`
	SportType    string  `json:"sport_type,omitempty"`
	SubType      *string `json:"sub_type,omitempty"`
	IsElectric   *bool   `json:"is_electric,omitempty"`
	IsIndoor     *bool   `json:"is_indoor,omitempty"`
	IsVirtual    *bool   `json:"is_virtual,omitempty"`
	IsCommute    *bool   `json:"is_commute,omitempty"`
}

type getActivityDetailArgs struct {
	ActivityID string `json:"activity_id"`
}

func RegisterActivityTools(reg *Registry) {
	for _, tool := range ActivityTools() {
		reg.Register(tool)
	}
}

func ActivityTools() []*Tool {
	tools := []*Tool{
		{
			Name:        "query_activities",
			Description: "List recent activities with optional sport, modifier, distance, title, gear, and sort filters.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"sport_type":{"type":"string"},"sub_type":{"type":"string"},"days":{"type":"integer","minimum":1},"limit":{"type":"integer","minimum":1,"maximum":50},"sort_by":{"type":"string","enum":["date","distance","duration","hr","elevation"]},"min_distance_km":{"type":"number","minimum":0},"title_search":{"type":"string"},"gear_id":{"type":"string","format":"uuid"},"is_electric":{"type":"boolean"},"is_indoor":{"type":"boolean"},"is_virtual":{"type":"boolean"},"is_commute":{"type":"boolean"}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: true,
			ExecuteRead: executeQueryActivities,
		},
		{
			Name:        "aggregate_stats",
			Description: "Aggregate activity metrics over a recent period, optionally grouped by time or sport.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"metric":{"type":"string","enum":["total_distance","total_duration","count","avg_heart_rate","total_elevation","avg_distance"]},"sport_type":{"type":"string"},"sub_type":{"type":"string"},"days":{"type":"integer","minimum":1},"group_by":{"type":"string","enum":["none","week","month","sport","sport_tuple"]},"is_electric":{"type":"boolean"},"is_indoor":{"type":"boolean"},"is_virtual":{"type":"boolean"},"is_commute":{"type":"boolean"}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: true,
			ExecuteRead: executeAggregateStats,
		},
		{
			Name:        "compare_periods",
			Description: "Compare count, distance, duration, heart rate, and elevation between two date ranges.",
			InputSchema: json.RawMessage(`{"type":"object","required":["period1_start","period1_end","period2_start","period2_end"],"properties":{"period1_start":{"type":"string","format":"date"},"period1_end":{"type":"string","format":"date"},"period2_start":{"type":"string","format":"date"},"period2_end":{"type":"string","format":"date"},"sport_type":{"type":"string"},"sub_type":{"type":"string"},"is_electric":{"type":"boolean"},"is_indoor":{"type":"boolean"},"is_virtual":{"type":"boolean"},"is_commute":{"type":"boolean"}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: true,
			ExecuteRead: executeComparePeriods,
		},
		{
			Name:        "get_activity_detail",
			Description: "Get details for one activity owned by the current user.",
			InputSchema: json.RawMessage(`{"type":"object","required":["activity_id"],"properties":{"activity_id":{"type":"string","format":"uuid"}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: true,
			ExecuteRead: executeGetActivityDetail,
		},
	}
	return append(tools, WriteActivityTools()...)
}

func executeQueryActivities(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args queryActivitiesArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode query_activities args: %w", err)
	}
	sql, params, err := buildQueryActivitiesSQL(userID, args, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	rows, err := queryRows(ctx, deps, sql, params...)
	if err != nil {
		return nil, err
	}
	activities := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		activities = append(activities, formatActivityCompact(row))
	}
	return map[string]any{"activities": activities}, nil
}

func executeAggregateStats(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args aggregateStatsArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode aggregate_stats args: %w", err)
	}
	sql, params := buildAggregateStatsSQL(userID, args, time.Now().UTC())
	rows, err := queryRows(ctx, deps, sql, params...)
	if err != nil {
		return nil, err
	}
	metric := args.metric()
	return map[string]any{
		"metric":     metric,
		"unit":       metricUnits[metric],
		"sport_type": activityEmptyStringNil(args.SportType),
		"sub_type":   stringPointerValue(args.SubType),
		"days":       args.days(),
		"group_by":   args.groupBy(),
		"data":       rows,
	}, nil
}

func executeComparePeriods(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args comparePeriodsArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode compare_periods args: %w", err)
	}
	p1, err := queryPeriodStats(ctx, deps, userID, args.Period1Start, args.Period1End, args)
	if err != nil {
		return nil, err
	}
	p2, err := queryPeriodStats(ctx, deps, userID, args.Period2Start, args.Period2End, args)
	if err != nil {
		return nil, err
	}
	p1["range"] = args.Period1Start + " - " + args.Period1End
	p2["range"] = args.Period2Start + " - " + args.Period2End
	return map[string]any{"period1": p1, "period2": p2}, nil
}

func executeGetActivityDetail(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args getActivityDetailArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode get_activity_detail args: %w", err)
	}
	sql, params, err := buildActivityDetailSQL(userID, args)
	if err != nil {
		return nil, err
	}
	rows, err := queryRows(ctx, deps, sql, params...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return map[string]any{"error": "Activity not found"}, nil
	}
	activity := rows[0]
	activity["distance_km"] = math.Round((numberValue(activity["distance_m"])/1000.0)*100) / 100
	delete(activity, "distance_m")
	activity["duration"] = formatDuration(activity["elapsed_time_s"])
	delete(activity, "elapsed_time_s")
	return map[string]any{"activity": activity}, nil
}

func buildQueryActivitiesSQL(userID string, args queryActivitiesArgs, now time.Time) (string, []any, error) {
	sortCol := validActivitySorts[args.SortBy]
	if sortCol == "" {
		sortCol = "started_at"
	}
	conditions := []string{"a.user_id = $1", "a.started_at >= $2"}
	params := []any{userID, sinceDate(now, args.days())}
	conditions, params = applyActivityFilters(conditions, params, ActivityFilters{
		TableAlias: "a",
		SportType:  args.SportType,
		SubType:    args.SubType,
		IsElectric: args.IsElectric,
		IsIndoor:   args.IsIndoor,
		IsVirtual:  args.IsVirtual,
		IsCommute:  args.IsCommute,
	})
	if args.MinDistanceKM != nil {
		params = append(params, *args.MinDistanceKM*1000)
		conditions = append(conditions, fmt.Sprintf("a.distance_m >= $%d", len(params)))
	}
	if args.TitleSearch != "" {
		params = append(params, "%"+args.TitleSearch+"%")
		conditions = append(conditions, fmt.Sprintf("a.title ILIKE $%d", len(params)))
	}
	if strings.TrimSpace(args.GearID) != "" {
		gearID, err := uuid.Parse(args.GearID)
		if err != nil {
			return "", nil, fmt.Errorf("invalid gear_id: %w", err)
		}
		params = append(params, gearID)
		conditions = append(conditions, fmt.Sprintf("EXISTS (SELECT 1 FROM activity_gear_assignments aga WHERE aga.activity_id = a.id AND aga.user_id = a.user_id AND aga.role_key = 'primary' AND aga.gear_id = $%d)", len(params)))
	}
	sql := "SELECT a.id, a.title, a.sport_type, a.sub_type, a.started_at, " +
		"a.distance_m, a.elapsed_time_s, a.avg_heart_rate, a.elevation_gain_m, " +
		"a.avg_speed_ms, a.is_electric, a.is_indoor, a.is_virtual, a.is_commute, a.start_city " +
		"FROM activities a WHERE " + strings.Join(conditions, " AND ") + " " +
		fmt.Sprintf("ORDER BY a.%s DESC NULLS LAST LIMIT %d", sortCol, args.limit())
	return sql, params, nil
}

func buildAggregateStatsSQL(userID string, args aggregateStatsArgs, now time.Time) (string, []any) {
	conditions := []string{"user_id = $1", "started_at >= $2"}
	params := []any{userID, sinceDate(now, args.days())}
	conditions, params = applyActivityFilters(conditions, params, ActivityFilters{
		SportType:  args.SportType,
		SubType:    args.SubType,
		IsElectric: args.IsElectric,
		IsIndoor:   args.IsIndoor,
		IsVirtual:  args.IsVirtual,
		IsCommute:  args.IsCommute,
	})
	where := strings.Join(conditions, " AND ")
	aggExpr := metricSQL[args.metric()]
	switch args.groupBy() {
	case "week":
		return fmt.Sprintf("SELECT date_trunc('week', started_at)::date AS period, %s FROM activities WHERE %s GROUP BY period ORDER BY period", aggExpr, where), params
	case "month":
		return fmt.Sprintf("SELECT date_trunc('month', started_at)::date AS period, %s FROM activities WHERE %s GROUP BY period ORDER BY period", aggExpr, where), params
	case "sport":
		return fmt.Sprintf("SELECT sport_type AS period, %s FROM activities WHERE %s GROUP BY sport_type ORDER BY value DESC", aggExpr, where), params
	case "sport_tuple":
		periodExpr := "CASE WHEN sub_type IS NULL OR sub_type = '' THEN sport_type ELSE sport_type || '/' || sub_type END AS period"
		return fmt.Sprintf("SELECT %s, sport_type, sub_type, %s FROM activities WHERE %s GROUP BY sport_type, sub_type ORDER BY value DESC", periodExpr, aggExpr, where), params
	default:
		return fmt.Sprintf("SELECT %s, COUNT(*) AS activity_count FROM activities WHERE %s", aggExpr, where), params
	}
}

func buildComparePeriodSQL(userID, start, end string, args comparePeriodsArgs) (string, []any, error) {
	startDate, err := parseDateOnly(start)
	if err != nil {
		return "", nil, fmt.Errorf("invalid period start %q: %w", start, err)
	}
	endDate, err := parseDateOnly(end)
	if err != nil {
		return "", nil, fmt.Errorf("invalid period end %q: %w", end, err)
	}
	conditions := []string{"user_id = $1", "started_at::date BETWEEN $2 AND $3"}
	params := []any{userID, startDate, endDate}
	conditions, params = applyActivityFilters(conditions, params, ActivityFilters{
		SportType:  args.SportType,
		SubType:    args.SubType,
		IsElectric: args.IsElectric,
		IsIndoor:   args.IsIndoor,
		IsVirtual:  args.IsVirtual,
		IsCommute:  args.IsCommute,
	})
	sql := "SELECT COUNT(*) AS count, " +
		"ROUND((COALESCE(SUM(distance_m), 0) / 1000.0)::numeric, 1)::float8 AS total_distance_km, " +
		"COALESCE(SUM(elapsed_time_s), 0) AS total_duration_s, " +
		"ROUND(COALESCE(AVG(avg_heart_rate), 0)::numeric, 1)::float8 AS avg_heart_rate, " +
		"ROUND(COALESCE(SUM(elevation_gain_m), 0)::numeric, 0)::float8 AS total_elevation_m " +
		"FROM activities WHERE " + strings.Join(conditions, " AND ")
	return sql, params, nil
}

func buildActivityDetailSQL(userID string, args getActivityDetailArgs) (string, []any, error) {
	activityID, err := uuid.Parse(args.ActivityID)
	if err != nil {
		return "", nil, fmt.Errorf("invalid activity_id: %w", err)
	}
	sql := "SELECT a.id, a.title, a.sport_type, a.sub_type, a.is_electric, a.is_indoor, a.is_virtual, a.is_commute, a.started_at, a.elapsed_time_s, " +
		"a.distance_m, a.elevation_gain_m, " +
		"a.avg_heart_rate, a.max_heart_rate, a.avg_speed_ms, a.avg_power_w, " +
		"a.start_city, a.start_state, a.start_country, " +
		"a.tss, a.description, a.weather_temp_c, a.weather_condition, " +
		"g.name AS gear_name, " +
		"(SELECT COUNT(*) FROM activity_photos ap WHERE ap.activity_id = a.id AND ap.user_id = a.user_id) AS photo_count, " +
		"(SELECT COUNT(*) FROM activity_geo_tags gt WHERE gt.activity_id = a.id) AS geo_tag_count " +
		"FROM activities a " +
		"LEFT JOIN activity_gear_assignments aga ON aga.activity_id = a.id AND aga.user_id = a.user_id AND aga.role_key = 'primary' " +
		"LEFT JOIN gear g ON g.id = aga.gear_id AND g.user_id = a.user_id " +
		"WHERE a.user_id = $1 AND a.id = $2"
	return sql, []any{userID, activityID}, nil
}

func queryPeriodStats(ctx context.Context, deps *Deps, userID, start, end string, args comparePeriodsArgs) (map[string]any, error) {
	sql, params, err := buildComparePeriodSQL(userID, start, end, args)
	if err != nil {
		return nil, err
	}
	rows, err := queryRows(ctx, deps, sql, params...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return map[string]any{}, nil
	}
	return rows[0], nil
}

func queryRows(ctx context.Context, deps *Deps, sql string, args ...any) ([]map[string]any, error) {
	pool, err := readonlyPool(deps)
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectToolRows(rows)
}

func collectToolRows(rows pgx.Rows) ([]map[string]any, error) {
	fields := rows.FieldDescriptions()
	result := make([]map[string]any, 0)
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make(map[string]any, len(fields))
		for i, field := range fields {
			row[field.Name] = values[i]
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func readonlyPool(deps *Deps) (*pgxpool.Pool, error) {
	if deps == nil {
		return nil, fmt.Errorf("tool deps are required")
	}
	if deps.Readonly != nil {
		return deps.Readonly, nil
	}
	if deps.DB != nil {
		if deps.DB.ReadonlyPool != nil {
			return deps.DB.ReadonlyPool, nil
		}
		if deps.DB.Pool != nil {
			return deps.DB.Pool, nil
		}
	}
	return nil, fmt.Errorf("readonly database pool is required")
}

func (a queryActivitiesArgs) days() int {
	if a.Days == nil || *a.Days <= 0 {
		return 30
	}
	return *a.Days
}

func (a queryActivitiesArgs) limit() int {
	if a.Limit == nil {
		return 10
	}
	return min(max(*a.Limit, 1), 50)
}

func (a aggregateStatsArgs) days() int {
	if a.Days == nil || *a.Days <= 0 {
		return 30
	}
	return *a.Days
}

func (a aggregateStatsArgs) metric() string {
	if metricSQL[a.Metric] == "" {
		return "count"
	}
	return a.Metric
}

func (a aggregateStatsArgs) groupBy() string {
	switch a.GroupBy {
	case "week", "month", "sport", "sport_tuple":
		return a.GroupBy
	default:
		return "none"
	}
}

func sinceDate(now time.Time, days int) time.Time {
	return dateOnly(now.Year(), now.Month(), now.Day()).AddDate(0, 0, -days)
}

func dateOnly(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func parseDateOnly(value string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, err
	}
	return dateOnly(t.Year(), t.Month(), t.Day()), nil
}

func activityEmptyStringNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func stringPointerValue(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
