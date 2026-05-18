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
	"avg_speed":       "km/h",
	"avg_power":       "W",
}

var metricSQL = map[string]string{
	"total_distance":  "COALESCE(SUM(distance_m), 0) / 1000.0 AS value",
	"total_duration":  "COALESCE(SUM(elapsed_time_s), 0) / 3600.0 AS value",
	"count":           "COUNT(*) AS value",
	"avg_heart_rate":  "ROUND(AVG(avg_heart_rate)::numeric, 1)::float8 AS value",
	"total_elevation": "COALESCE(SUM(elevation_gain_m), 0) AS value",
	"avg_distance":    "ROUND((COALESCE(AVG(distance_m), 0) / 1000.0)::numeric, 2)::float8 AS value",
	"avg_speed":       "ROUND((COALESCE(AVG(avg_speed_ms), 0) * 3.6)::numeric, 2)::float8 AS value",
	"avg_power":       "ROUND(COALESCE(AVG(avg_power_w), 0)::numeric, 1)::float8 AS value",
}

var recordOrderSQL = map[string]string{
	"longest_distance":  "a.distance_m DESC NULLS LAST",
	"longest_duration":  "a.elapsed_time_s DESC NULLS LAST",
	"most_elevation":    "a.elevation_gain_m DESC NULLS LAST",
	"highest_avg_speed": "a.avg_speed_ms DESC NULLS LAST",
	"highest_avg_power": "a.avg_power_w DESC NULLS LAST",
	"highest_hr":        "a.max_heart_rate DESC NULLS LAST, a.avg_heart_rate DESC NULLS LAST",
}

var recordValueSQL = map[string]string{
	"longest_distance":  "ROUND((a.distance_m / 1000.0)::numeric, 2)::float8",
	"longest_duration":  "a.elapsed_time_s",
	"most_elevation":    "a.elevation_gain_m",
	"highest_avg_speed": "ROUND((a.avg_speed_ms * 3.6)::numeric, 2)::float8",
	"highest_avg_power": "a.avg_power_w",
	"highest_hr":        "COALESCE(a.max_heart_rate, a.avg_heart_rate)",
}

var recordUnits = map[string]string{
	"longest_distance":  "km",
	"longest_duration":  "s",
	"most_elevation":    "m",
	"highest_avg_speed": "km/h",
	"highest_avg_power": "W",
	"highest_hr":        "bpm",
}

type queryActivitiesArgs struct {
	SportQuery    string   `json:"sport_query,omitempty"`
	SportType     string   `json:"sport_type,omitempty"`
	SubType       *string  `json:"sub_type,omitempty"`
	Days          *int     `json:"days,omitempty"`
	DateFrom      string   `json:"date_from,omitempty"`
	DateTo        string   `json:"date_to,omitempty"`
	Limit         *int     `json:"limit,omitempty"`
	SortBy        string   `json:"sort_by,omitempty"`
	MinDistanceKM *float64 `json:"min_distance_km,omitempty"`
	MaxDistanceKM *float64 `json:"max_distance_km,omitempty"`
	TitleSearch   string   `json:"title_search,omitempty"`
	GearID        string   `json:"gear_id,omitempty"`
	TagName       string   `json:"tag_name,omitempty"`
	IsElectric    *bool    `json:"is_electric,omitempty"`
	IsIndoor      *bool    `json:"is_indoor,omitempty"`
	IsVirtual     *bool    `json:"is_virtual,omitempty"`
	IsCommute     *bool    `json:"is_commute,omitempty"`
	CountryCodes  []string `json:"country_codes,omitempty"`
}

type aggregateStatsArgs struct {
	Metric        string   `json:"metric,omitempty"`
	SportQuery    string   `json:"sport_query,omitempty"`
	SportType     string   `json:"sport_type,omitempty"`
	SubType       *string  `json:"sub_type,omitempty"`
	Days          *int     `json:"days,omitempty"`
	DateFrom      string   `json:"date_from,omitempty"`
	DateTo        string   `json:"date_to,omitempty"`
	AllHistory    bool     `json:"all_history,omitempty"`
	GroupBy       string   `json:"group_by,omitempty"`
	MinDistanceKM *float64 `json:"min_distance_km,omitempty"`
	MaxDistanceKM *float64 `json:"max_distance_km,omitempty"`
	TitleSearch   string   `json:"title_search,omitempty"`
	GearID        string   `json:"gear_id,omitempty"`
	TagName       string   `json:"tag_name,omitempty"`
	IsElectric    *bool    `json:"is_electric,omitempty"`
	IsIndoor      *bool    `json:"is_indoor,omitempty"`
	IsVirtual     *bool    `json:"is_virtual,omitempty"`
	IsCommute     *bool    `json:"is_commute,omitempty"`
	CountryCodes  []string `json:"country_codes,omitempty"`
}

type aggregateTimeSeriesArgs struct {
	Metric       string   `json:"metric,omitempty"`
	Bucket       string   `json:"bucket,omitempty"`
	SportQuery   string   `json:"sport_query,omitempty"`
	SportType    string   `json:"sport_type,omitempty"`
	SubType      *string  `json:"sub_type,omitempty"`
	Days         *int     `json:"days,omitempty"`
	DateFrom     string   `json:"date_from,omitempty"`
	DateTo       string   `json:"date_to,omitempty"`
	IsElectric   *bool    `json:"is_electric,omitempty"`
	IsIndoor     *bool    `json:"is_indoor,omitempty"`
	IsVirtual    *bool    `json:"is_virtual,omitempty"`
	IsCommute    *bool    `json:"is_commute,omitempty"`
	CountryCodes []string `json:"country_codes,omitempty"`
}

type queryRecordsArgs struct {
	RecordType    string   `json:"record_type,omitempty"`
	SportQuery    string   `json:"sport_query,omitempty"`
	SportType     string   `json:"sport_type,omitempty"`
	SubType       *string  `json:"sub_type,omitempty"`
	Days          *int     `json:"days,omitempty"`
	DateFrom      string   `json:"date_from,omitempty"`
	DateTo        string   `json:"date_to,omitempty"`
	AllHistory    bool     `json:"all_history,omitempty"`
	Limit         *int     `json:"limit,omitempty"`
	MinDistanceKM *float64 `json:"min_distance_km,omitempty"`
	MaxDistanceKM *float64 `json:"max_distance_km,omitempty"`
	TitleSearch   string   `json:"title_search,omitempty"`
	GearID        string   `json:"gear_id,omitempty"`
	TagName       string   `json:"tag_name,omitempty"`
	IsElectric    *bool    `json:"is_electric,omitempty"`
	IsIndoor      *bool    `json:"is_indoor,omitempty"`
	IsVirtual     *bool    `json:"is_virtual,omitempty"`
	IsCommute     *bool    `json:"is_commute,omitempty"`
	CountryCodes  []string `json:"country_codes,omitempty"`
}

type comparePeriodsArgs struct {
	Period1Start string   `json:"period1_start"`
	Period1End   string   `json:"period1_end"`
	Period2Start string   `json:"period2_start"`
	Period2End   string   `json:"period2_end"`
	SportQuery   string   `json:"sport_query,omitempty"`
	SportType    string   `json:"sport_type,omitempty"`
	SubType      *string  `json:"sub_type,omitempty"`
	IsElectric   *bool    `json:"is_electric,omitempty"`
	IsIndoor     *bool    `json:"is_indoor,omitempty"`
	IsVirtual    *bool    `json:"is_virtual,omitempty"`
	IsCommute    *bool    `json:"is_commute,omitempty"`
	CountryCodes []string `json:"country_codes,omitempty"`
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
			Description: "List recent activities with optional date, sport, modifier, distance, title, gear, tag, and sort filters.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"sport_query":{"type":"string","description":"Natural sport phrase to resolve via Skadia taxonomy, e.g. swim, Freiwasser, MTB, Gravel"},"sport_type":{"type":"string"},"sub_type":{"type":"string"},"days":{"type":"integer","minimum":1},"date_from":{"type":"string","format":"date"},"date_to":{"type":"string","format":"date"},"limit":{"type":"integer","minimum":1,"maximum":50},"sort_by":{"type":"string","enum":["date","distance","duration","hr","elevation"]},"min_distance_km":{"type":"number","minimum":0},"max_distance_km":{"type":"number","minimum":0},"title_search":{"type":"string"},"gear_id":{"type":"string","format":"uuid"},"tag_name":{"type":"string"},"is_electric":{"type":"boolean"},"is_indoor":{"type":"boolean"},"is_virtual":{"type":"boolean"},"is_commute":{"type":"boolean"},"country_codes":{"type":"array","items":{"type":"string","pattern":"^[A-Za-z]{2}$"}}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: true,
			ExecuteRead: executeQueryActivities,
		},
		PreviewActivitySelectionTool(),
		{
			Name:        "aggregate_stats",
			Description: "Aggregate activity metrics over a recent period, optionally grouped by time or sport.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"metric":{"type":"string","enum":["total_distance","total_duration","count","avg_heart_rate","total_elevation","avg_distance","avg_speed","avg_power"]},"sport_query":{"type":"string","description":"Natural sport phrase to resolve via Skadia taxonomy"},"sport_type":{"type":"string"},"sub_type":{"type":"string"},"days":{"type":"integer","minimum":1},"date_from":{"type":"string","format":"date"},"date_to":{"type":"string","format":"date"},"all_history":{"type":"boolean"},"group_by":{"type":"string","enum":["none","week","month","sport","sport_tuple"]},"min_distance_km":{"type":"number","minimum":0},"max_distance_km":{"type":"number","minimum":0},"title_search":{"type":"string"},"gear_id":{"type":"string","format":"uuid"},"tag_name":{"type":"string"},"is_electric":{"type":"boolean"},"is_indoor":{"type":"boolean"},"is_virtual":{"type":"boolean"},"is_commute":{"type":"boolean"},"country_codes":{"type":"array","items":{"type":"string","pattern":"^[A-Za-z]{2}$"}}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: true,
			ExecuteRead: executeAggregateStats,
		},
		{
			Name:        "aggregate_time_series",
			Description: "Aggregate user-owned activity metrics into day, week, month, or year buckets for trend questions.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"metric":{"type":"string","enum":["total_distance","total_duration","count","avg_heart_rate","total_elevation","avg_distance","avg_speed","avg_power"]},"bucket":{"type":"string","enum":["day","week","month","year"]},"sport_query":{"type":"string","description":"Natural sport phrase to resolve via Skadia taxonomy"},"sport_type":{"type":"string"},"sub_type":{"type":"string"},"days":{"type":"integer","minimum":1},"date_from":{"type":"string","format":"date"},"date_to":{"type":"string","format":"date"},"is_electric":{"type":"boolean"},"is_indoor":{"type":"boolean"},"is_virtual":{"type":"boolean"},"is_commute":{"type":"boolean"},"country_codes":{"type":"array","items":{"type":"string","pattern":"^[A-Za-z]{2}$"}}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: true,
			ExecuteRead: executeAggregateTimeSeries,
		},
		{
			Name:        "query_records",
			Description: "Find top activities by distance, duration, elevation, speed, power, or heart rate with reusable sport/date/gear/tag filters.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"record_type":{"type":"string","enum":["longest_distance","longest_duration","most_elevation","highest_avg_speed","highest_avg_power","highest_hr"]},"sport_query":{"type":"string","description":"Natural sport phrase to resolve via Skadia taxonomy"},"sport_type":{"type":"string"},"sub_type":{"type":"string"},"days":{"type":"integer","minimum":1},"date_from":{"type":"string","format":"date"},"date_to":{"type":"string","format":"date"},"all_history":{"type":"boolean"},"limit":{"type":"integer","minimum":1,"maximum":20},"min_distance_km":{"type":"number","minimum":0},"max_distance_km":{"type":"number","minimum":0},"title_search":{"type":"string"},"gear_id":{"type":"string","format":"uuid"},"tag_name":{"type":"string"},"is_electric":{"type":"boolean"},"is_indoor":{"type":"boolean"},"is_virtual":{"type":"boolean"},"is_commute":{"type":"boolean"},"country_codes":{"type":"array","items":{"type":"string","pattern":"^[A-Za-z]{2}$"}}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: true,
			ExecuteRead: executeQueryRecords,
		},
		{
			Name:        "compare_periods",
			Description: "Compare count, distance, duration, heart rate, and elevation between two date ranges.",
			InputSchema: json.RawMessage(`{"type":"object","required":["period1_start","period1_end","period2_start","period2_end"],"properties":{"period1_start":{"type":"string","format":"date"},"period1_end":{"type":"string","format":"date"},"period2_start":{"type":"string","format":"date"},"period2_end":{"type":"string","format":"date"},"sport_query":{"type":"string","description":"Natural sport phrase to resolve via Skadia taxonomy"},"sport_type":{"type":"string"},"sub_type":{"type":"string"},"is_electric":{"type":"boolean"},"is_indoor":{"type":"boolean"},"is_virtual":{"type":"boolean"},"is_commute":{"type":"boolean"},"country_codes":{"type":"array","items":{"type":"string","pattern":"^[A-Za-z]{2}$"}}},"additionalProperties":false}`),
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
	sql, params, err := buildAggregateStatsSQL(userID, args, time.Now().UTC())
	if err != nil {
		return nil, err
	}
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

func executeAggregateTimeSeries(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args aggregateTimeSeriesArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode aggregate_time_series args: %w", err)
	}
	sql, params, criteria, err := buildAggregateTimeSeriesSQL(userID, args, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	rows, err := queryRows(ctx, deps, sql, params...)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		row["period"] = dateString(row["period"])
	}
	metric := args.metric()
	return map[string]any{
		"metric":   metric,
		"unit":     metricUnits[metric],
		"bucket":   args.bucket(),
		"criteria": criteria,
		"data":     rows,
	}, nil
}

func executeQueryRecords(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args queryRecordsArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode query_records args: %w", err)
	}
	sql, params, criteria, err := buildQueryRecordsSQL(userID, args, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	rows, err := queryRows(ctx, deps, sql, params...)
	if err != nil {
		return nil, err
	}
	records := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		record := formatActivityCompact(row)
		record["record_value"] = row["record_value"]
		record["record_unit"] = recordUnits[args.recordType()]
		if args.recordType() == "longest_duration" {
			record["record_formatted"] = formatDuration(row["record_value"])
		}
		records = append(records, record)
	}
	return map[string]any{
		"record_type": args.recordType(),
		"unit":        recordUnits[args.recordType()],
		"criteria":    criteria,
		"records":     records,
		"count":       len(records),
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
	plan, err := buildActivitySelectionPlan(userID, activitySelectionCriteria{
		SportType:     args.SportType,
		SportQuery:    args.SportQuery,
		SubType:       args.SubType,
		Days:          args.Days,
		DateFrom:      args.DateFrom,
		DateTo:        args.DateTo,
		MinDistanceKM: args.MinDistanceKM,
		MaxDistanceKM: args.MaxDistanceKM,
		TitleSearch:   args.TitleSearch,
		GearID:        args.GearID,
		TagName:       args.TagName,
		IsElectric:    args.IsElectric,
		IsIndoor:      args.IsIndoor,
		IsVirtual:     args.IsVirtual,
		IsCommute:     args.IsCommute,
		CountryCodes:  args.CountryCodes,
	}, now)
	if err != nil {
		return "", nil, err
	}
	sql := "SELECT a.id, a.title, a.sport_type, a.sub_type, a.started_at, " +
		"a.distance_m, a.elapsed_time_s, a.avg_heart_rate, a.elevation_gain_m, " +
		"a.avg_speed_ms, a.is_electric, a.is_indoor, a.is_virtual, a.is_commute, a.start_city " +
		"FROM activities a WHERE " + strings.Join(plan.Conditions, " AND ") + " " +
		fmt.Sprintf("ORDER BY a.%s DESC NULLS LAST LIMIT %d", sortCol, args.limit())
	return sql, plan.Params, nil
}

func buildAggregateStatsSQL(userID string, args aggregateStatsArgs, now time.Time) (string, []any, error) {
	plan, err := buildActivitySelectionPlan(userID, activitySelectionCriteria{
		SportQuery:    args.SportQuery,
		SportType:     args.SportType,
		SubType:       args.SubType,
		Days:          args.Days,
		DateFrom:      args.DateFrom,
		DateTo:        args.DateTo,
		AllHistory:    args.AllHistory,
		MinDistanceKM: args.MinDistanceKM,
		MaxDistanceKM: args.MaxDistanceKM,
		TitleSearch:   args.TitleSearch,
		GearID:        args.GearID,
		TagName:       args.TagName,
		IsElectric:    args.IsElectric,
		IsIndoor:      args.IsIndoor,
		IsVirtual:     args.IsVirtual,
		IsCommute:     args.IsCommute,
		CountryCodes:  args.CountryCodes,
	}, now)
	if err != nil {
		return "", nil, err
	}
	where := strings.Join(plan.Conditions, " AND ")
	aggExpr := metricSQL[args.metric()]
	switch args.groupBy() {
	case "week":
		return fmt.Sprintf("SELECT date_trunc('week', a.started_at)::date AS period, %s FROM activities a WHERE %s GROUP BY period ORDER BY period", aggExpr, where), plan.Params, nil
	case "month":
		return fmt.Sprintf("SELECT date_trunc('month', a.started_at)::date AS period, %s FROM activities a WHERE %s GROUP BY period ORDER BY period", aggExpr, where), plan.Params, nil
	case "sport":
		return fmt.Sprintf("SELECT a.sport_type AS period, %s FROM activities a WHERE %s GROUP BY a.sport_type ORDER BY value DESC", aggExpr, where), plan.Params, nil
	case "sport_tuple":
		periodExpr := "CASE WHEN a.sub_type IS NULL OR a.sub_type = '' THEN a.sport_type ELSE a.sport_type || '/' || a.sub_type END AS period"
		return fmt.Sprintf("SELECT %s, a.sport_type, a.sub_type, %s FROM activities a WHERE %s GROUP BY a.sport_type, a.sub_type ORDER BY value DESC", periodExpr, aggExpr, where), plan.Params, nil
	default:
		return fmt.Sprintf("SELECT %s, COUNT(*) AS activity_count FROM activities a WHERE %s", aggExpr, where), plan.Params, nil
	}
}

func buildAggregateTimeSeriesSQL(userID string, args aggregateTimeSeriesArgs, now time.Time) (string, []any, map[string]any, error) {
	days := args.Days
	if days == nil && args.DateFrom == "" && args.DateTo == "" {
		defaultDays := 365
		days = &defaultDays
	}
	plan, err := buildActivitySelectionPlan(userID, activitySelectionCriteria{
		SportQuery:   args.SportQuery,
		SportType:    args.SportType,
		SubType:      args.SubType,
		Days:         days,
		DateFrom:     args.DateFrom,
		DateTo:       args.DateTo,
		IsElectric:   args.IsElectric,
		IsIndoor:     args.IsIndoor,
		IsVirtual:    args.IsVirtual,
		IsCommute:    args.IsCommute,
		CountryCodes: args.CountryCodes,
	}, now)
	if err != nil {
		return "", nil, nil, err
	}
	aggExpr := metricSQL[args.metric()]
	where := strings.Join(plan.Conditions, " AND ")
	sql := fmt.Sprintf("SELECT date_trunc('%s', a.started_at)::date AS period, %s, COUNT(*) AS activity_count FROM activities a WHERE %s GROUP BY period ORDER BY period", args.bucket(), aggExpr, where)
	return sql, plan.Params, plan.CriteriaForResponse, nil
}

func buildQueryRecordsSQL(userID string, args queryRecordsArgs, now time.Time) (string, []any, map[string]any, error) {
	days := args.Days
	if days == nil && args.DateFrom == "" && args.DateTo == "" && !args.AllHistory {
		defaultDays := 365
		days = &defaultDays
	}
	plan, err := buildActivitySelectionPlan(userID, activitySelectionCriteria{
		SportQuery:    args.SportQuery,
		SportType:     args.SportType,
		SubType:       args.SubType,
		Days:          days,
		DateFrom:      args.DateFrom,
		DateTo:        args.DateTo,
		AllHistory:    args.AllHistory,
		MinDistanceKM: args.MinDistanceKM,
		MaxDistanceKM: args.MaxDistanceKM,
		TitleSearch:   args.TitleSearch,
		GearID:        args.GearID,
		TagName:       args.TagName,
		IsElectric:    args.IsElectric,
		IsIndoor:      args.IsIndoor,
		IsVirtual:     args.IsVirtual,
		IsCommute:     args.IsCommute,
		CountryCodes:  args.CountryCodes,
	}, now)
	if err != nil {
		return "", nil, nil, err
	}
	recordType := args.recordType()
	sql := "SELECT a.id, a.title, a.sport_type, a.sub_type, a.started_at, " +
		"a.distance_m, a.elapsed_time_s, a.avg_heart_rate, a.max_heart_rate, a.elevation_gain_m, " +
		"a.avg_speed_ms, a.avg_power_w, a.is_electric, a.is_indoor, a.is_virtual, a.is_commute, a.start_city, " +
		recordValueSQL[recordType] + " AS record_value " +
		"FROM activities a WHERE " + strings.Join(plan.Conditions, " AND ") + " " +
		fmt.Sprintf("ORDER BY %s, a.started_at DESC NULLS LAST LIMIT %d", recordOrderSQL[recordType], args.limit())
	return sql, plan.Params, plan.CriteriaForResponse, nil
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
	criteria, err := normalizeActivitySelectionCriteria(activitySelectionCriteria{
		SportQuery:   args.SportQuery,
		SportType:    args.SportType,
		SubType:      args.SubType,
		IsElectric:   args.IsElectric,
		IsIndoor:     args.IsIndoor,
		IsVirtual:    args.IsVirtual,
		IsCommute:    args.IsCommute,
		CountryCodes: args.CountryCodes,
	})
	if err != nil {
		return "", nil, err
	}
	conditions, params = applyActivityFilters(conditions, params, ActivityFilters{
		SportType:    criteria.SportType,
		SubType:      criteria.SubType,
		IsElectric:   criteria.IsElectric,
		IsIndoor:     criteria.IsIndoor,
		IsVirtual:    criteria.IsVirtual,
		IsCommute:    criteria.IsCommute,
		CountryCodes: criteria.CountryCodes,
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
		"a.start_city, a.start_state, a.start_country, a.visited_country_codes, " +
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
	if deps.DB != nil {
		if deps.DB.Pool != nil {
			return deps.DB.Pool, nil
		}
		if deps.DB.ReadonlyPool != nil {
			return deps.DB.ReadonlyPool, nil
		}
	}
	if deps.Readonly != nil {
		return deps.Readonly, nil
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

func (a aggregateTimeSeriesArgs) metric() string {
	if metricSQL[a.Metric] == "" {
		return "count"
	}
	return a.Metric
}

func (a aggregateTimeSeriesArgs) bucket() string {
	switch a.Bucket {
	case "day", "week", "year":
		return a.Bucket
	default:
		return "month"
	}
}

func (a queryRecordsArgs) recordType() string {
	if recordOrderSQL[a.RecordType] == "" {
		return "longest_distance"
	}
	return a.RecordType
}

func (a queryRecordsArgs) limit() int {
	if a.Limit == nil {
		return 5
	}
	return min(max(*a.Limit, 1), 20)
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
