package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type getHealthStatusArgs struct {
	Aspect string `json:"aspect,omitempty"`
	Days   int    `json:"days,omitempty"`
}

type getTrainingLoadArgs struct {
	Days int `json:"days,omitempty"`
}

type getAnalyticsArgs struct {
	Analysis string `json:"analysis"`
}

type getHRVTrendArgs struct {
	Days int `json:"days,omitempty"`
}

func RegisterHealthTools(reg *Registry) {
	for _, tool := range HealthTools() {
		reg.Register(tool)
	}
}

func HealthTools() []*Tool {
	return []*Tool{
		{
			Name:        "get_health_status",
			Description: "Retrieve health and recovery data: HRV trend, sleep metrics, readiness, body weight, and resting heart rate.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"aspect":{"type":"string","enum":["hrv","sleep","readiness","weight","resting_hr","all"],"default":"all"},"days":{"type":"integer","minimum":1,"default":30}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: true,
			ExecuteRead: executeGetHealthStatus,
		},
		{
			Name:        "get_training_load",
			Description: "Fetch Performance Management Chart data: CTL, ATL, TSB, TSS, and current ACWR.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"days":{"type":"integer","minimum":1,"default":30}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: true,
			ExecuteRead: executeGetTrainingLoad,
		},
		{
			Name:        "get_analytics",
			Description: "Retrieve cached analytics results such as anomaly detection, clustering, readiness, correlations, DFA alpha1, and route similarity.",
			InputSchema: json.RawMessage(`{"type":"object","required":["analysis"],"properties":{"analysis":{"type":"string","enum":["anomaly","clustering","readiness","correlations","dfa_alpha1","route_similarity"]}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: true,
			ExecuteRead: executeGetAnalytics,
		},
		{
			Name:        "get_hrv_trend",
			Description: "Legacy alias for get_health_status with aspect hrv.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"days":{"type":"integer","minimum":1,"default":30}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: true,
			ExecuteRead: executeGetHRVTrendLegacy,
		},
		{
			Name:        "get_readiness",
			Description: "Legacy alias for get_health_status with aspect readiness.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: true,
			ExecuteRead: executeGetReadinessLegacy,
		},
	}
}

func executeGetHealthStatus(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args getHealthStatusArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode get_health_status args: %w", err)
	}
	pool, err := readonlyPool(deps)
	if err != nil {
		return nil, err
	}
	return getHealthStatus(ctx, pool, userID, args)
}

func executeGetTrainingLoad(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args getTrainingLoadArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode get_training_load args: %w", err)
	}
	pool, err := readonlyPool(deps)
	if err != nil {
		return nil, err
	}
	return getTrainingLoad(ctx, pool, userID, args)
}

func executeGetAnalytics(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args getAnalyticsArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode get_analytics args: %w", err)
	}
	pool, err := readonlyPool(deps)
	if err != nil {
		return nil, err
	}
	return getAnalytics(ctx, pool, userID, args)
}

func executeGetHRVTrendLegacy(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args getHRVTrendArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode get_hrv_trend args: %w", err)
	}
	pool, err := readonlyPool(deps)
	if err != nil {
		return nil, err
	}
	result, err := getHealthStatus(ctx, pool, userID, getHealthStatusArgs{Aspect: "hrv", Days: args.days()})
	if err != nil {
		return nil, err
	}
	hrv, _ := result["hrv"].(map[string]any)
	return map[string]any{"hrv_data": hrv["data"]}, nil
}

func executeGetReadinessLegacy(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	if len(raw) > 0 {
		var args map[string]any
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("decode get_readiness args: %w", err)
		}
	}
	pool, err := readonlyPool(deps)
	if err != nil {
		return nil, err
	}
	result, err := getHealthStatus(ctx, pool, userID, getHealthStatusArgs{Aspect: "readiness", Days: 30})
	if err != nil {
		return nil, err
	}
	return map[string]any{"readiness": result["readiness"]}, nil
}

func getHealthStatus(ctx context.Context, pool *pgxpool.Pool, userID string, args getHealthStatusArgs) (map[string]any, error) {
	aspect := args.aspect()
	since := sinceDate(time.Now().UTC(), args.days())
	result := make(map[string]any)

	if aspect == "hrv" || aspect == "all" {
		hrv, err := getHealthHRV(ctx, pool, userID, since)
		if err != nil {
			return nil, err
		}
		result["hrv"] = hrv
	}
	if aspect == "sleep" || aspect == "all" {
		sleep, err := getHealthSleep(ctx, pool, userID, since)
		if err != nil {
			return nil, err
		}
		result["sleep"] = sleep
	}
	if aspect == "readiness" || aspect == "all" {
		readiness, err := getLatestAnalyticsData(ctx, pool, userID, "readiness")
		if err != nil {
			return nil, err
		}
		result["readiness"] = readiness
	}
	if aspect == "weight" || aspect == "all" {
		weight, err := queryRowsFromPool(ctx, pool, `
			SELECT measured_at_utc::date AS date,
			       ROUND((weight_g / 1000.0)::numeric, 1)::float8 AS weight_kg,
			       body_fat_pct
			FROM health_weight_entries
			WHERE user_id = $1::uuid AND measured_at_utc >= $2
			ORDER BY measured_at_utc`, userID, since)
		if err != nil {
			return nil, err
		}
		normalizeDateFields(weight, "date")
		result["weight"] = weight
	}
	if aspect == "resting_hr" || aspect == "all" {
		restingHR, err := queryRowsFromPool(ctx, pool, `
			SELECT calendar_date AS date, resting_heart_rate AS resting_hr_bpm
			FROM health_daily_summary
			WHERE user_id = $1::uuid AND calendar_date >= $2 AND resting_heart_rate IS NOT NULL
			ORDER BY calendar_date`, userID, since)
		if err != nil {
			return nil, err
		}
		normalizeDateFields(restingHR, "date")
		result["resting_hr"] = restingHR
	}
	return result, nil
}

func getHealthHRV(ctx context.Context, pool *pgxpool.Pool, userID string, since time.Time) (map[string]any, error) {
	rows, err := queryRowsFromPool(ctx, pool, `
		SELECT measured_at_utc::date AS date, rmssd_ms, sdrr_ms, extra_data
		FROM health_snapshots
		WHERE user_id = $1::uuid AND measured_at_utc >= $2
		ORDER BY measured_at_utc`, userID, since)
	if err != nil {
		return nil, err
	}
	data := make([]map[string]any, 0, len(rows))
	var rmssdTotal float64
	for _, row := range rows {
		entry := map[string]any{
			"date":     dateString(row["date"]),
			"rmssd_ms": row["rmssd_ms"],
			"sdrr_ms":  row["sdrr_ms"],
		}
		if extra, ok := decodeJSONMap(row["extra_data"]); ok {
			for _, key := range []string{"weekly_avg", "baseline_low", "baseline_high", "status"} {
				if value, exists := extra[key]; exists {
					entry[key] = value
				}
			}
		}
		rmssdTotal += numberValue(row["rmssd_ms"])
		data = append(data, entry)
	}
	var latest any
	var avgRMSSD any
	if len(data) > 0 {
		latest = data[len(data)-1]
		avgRMSSD = math.Round((rmssdTotal/float64(len(data)))*10) / 10
	}
	return map[string]any{
		"data":      data,
		"latest":    latest,
		"avg_rmssd": avgRMSSD,
	}, nil
}

func getHealthSleep(ctx context.Context, pool *pgxpool.Pool, userID string, since time.Time) (map[string]any, error) {
	data, err := queryRowsFromPool(ctx, pool, `
		SELECT calendar_date AS date,
		       (COALESCE(deep_seconds, 0) + COALESCE(light_seconds, 0) + COALESCE(rem_seconds, 0)) AS total_sleep_s,
		       deep_seconds, light_seconds, rem_seconds, awake_seconds, score_overall AS score
		FROM health_sleep
		WHERE user_id = $1::uuid AND calendar_date >= $2
		ORDER BY calendar_date`, userID, since)
	if err != nil {
		return nil, err
	}
	normalizeDateFields(data, "date")
	var scoreTotal float64
	var sleepTotal float64
	for _, row := range data {
		scoreTotal += numberValue(row["score"])
		sleepTotal += numberValue(row["total_sleep_s"])
	}
	var avgScore any
	var avgTotalH any
	if len(data) > 0 {
		avgScore = math.Round((scoreTotal/float64(len(data)))*10) / 10
		avgTotalH = math.Round((sleepTotal/3600.0/float64(len(data)))*10) / 10
	}
	return map[string]any{
		"data":        data,
		"avg_score":   avgScore,
		"avg_total_h": avgTotalH,
	}, nil
}

func getTrainingLoad(ctx context.Context, pool *pgxpool.Pool, userID string, args getTrainingLoadArgs) (map[string]any, error) {
	rows, err := queryRowsFromPool(ctx, pool, `
		SELECT date, tss, ctl, atl, tsb
		FROM pmc_daily
		WHERE user_id = $1::uuid AND date >= $2
		ORDER BY date`, userID, sinceDate(time.Now().UTC(), args.days()))
	if err != nil {
		return nil, err
	}
	normalizeDateFields(rows, "date")
	if len(rows) == 0 {
		return map[string]any{"pmc_data": []map[string]any{}, "current": nil}, nil
	}
	latest := copyMap(rows[len(rows)-1])
	if ctl := numberValue(latest["ctl"]); ctl > 0 {
		latest["acwr"] = math.Round((numberValue(latest["atl"])/ctl)*100) / 100
	} else {
		latest["acwr"] = nil
	}
	return map[string]any{"pmc_data": rows, "current": latest}, nil
}

func getAnalytics(ctx context.Context, pool *pgxpool.Pool, userID string, args getAnalyticsArgs) (map[string]any, error) {
	row, err := querySingleRowFromPool(ctx, pool, `
		SELECT data, computed_at
		FROM analytics_cache
		WHERE user_id = $1::uuid AND cache_key = $2
		ORDER BY computed_at DESC
		LIMIT 1`, userID, args.Analysis)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"analysis": args.Analysis}
	if row == nil {
		result["data"] = nil
		result["note"] = "No cached result available"
		return result, nil
	}
	result["computed_at"] = timestampString(row["computed_at"])
	result["data"] = decodeJSONValue(row["data"])
	return result, nil
}

func getLatestAnalyticsData(ctx context.Context, pool *pgxpool.Pool, userID, key string) (any, error) {
	row, err := querySingleRowFromPool(ctx, pool, `
		SELECT data
		FROM analytics_cache
		WHERE user_id = $1::uuid AND cache_key = $2
		ORDER BY computed_at DESC
		LIMIT 1`, userID, key)
	if err != nil || row == nil {
		return nil, err
	}
	return decodeJSONValue(row["data"]), nil
}

func queryRowsFromPool(ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) ([]map[string]any, error) {
	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectToolRows(rows)
}

func querySingleRowFromPool(ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) (map[string]any, error) {
	rows, err := queryRowsFromPool(ctx, pool, sql, args...)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return rows[0], nil
}

func decodeJSONValue(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case []byte:
		var out any
		if err := json.Unmarshal(v, &out); err == nil {
			return out
		}
	case string:
		var out any
		if err := json.Unmarshal([]byte(v), &out); err == nil {
			return out
		}
	}
	return value
}

func decodeJSONMap(value any) (map[string]any, bool) {
	decoded := decodeJSONValue(value)
	out, ok := decoded.(map[string]any)
	return out, ok
}

func normalizeDateFields(rows []map[string]any, field string) {
	for _, row := range rows {
		row[field] = dateString(row[field])
	}
}

func dateString(value any) string {
	switch v := value.(type) {
	case time.Time:
		return v.Format("2006-01-02")
	case *time.Time:
		if v == nil {
			return ""
		}
		return v.Format("2006-01-02")
	default:
		return stringValue(value)
	}
}

func timestampString(value any) string {
	switch v := value.(type) {
	case time.Time:
		return v.Format("2006-01-02 15:04:05")
	case *time.Time:
		if v == nil {
			return ""
		}
		return v.Format("2006-01-02 15:04:05")
	default:
		return stringValue(value)
	}
}

func copyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (a getHealthStatusArgs) aspect() string {
	switch a.Aspect {
	case "hrv", "sleep", "readiness", "weight", "resting_hr":
		return a.Aspect
	default:
		return "all"
	}
}

func (a getHealthStatusArgs) days() int {
	if a.Days <= 0 {
		return 30
	}
	return a.Days
}

func (a getTrainingLoadArgs) days() int {
	if a.Days <= 0 {
		return 30
	}
	return a.Days
}

func (a getHRVTrendArgs) days() int {
	if a.Days <= 0 {
		return 30
	}
	return a.Days
}
