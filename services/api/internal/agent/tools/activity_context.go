package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/model"
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
	QueryKind  string   `json:"query_kind,omitempty"`
	Mode       string   `json:"mode,omitempty"`
	StoredOnly *bool    `json:"stored_only,omitempty"`
	Limit      *int     `json:"limit,omitempty"`
}

type liveWeatherPoint struct {
	Lat        float64
	Lng        float64
	ElevationM float64
	At         time.Time
}

type liveWeatherConfig struct {
	Policy          string
	ProviderEnabled bool
	LimitPerHour    int
}

type liveWeatherQuotaBucket struct {
	Hour  time.Time
	Count int
}

var (
	liveWeatherQuotaMu      sync.Mutex
	liveWeatherQuotaBuckets = map[string]liveWeatherQuotaBucket{}
)

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
		Name:             "query_weather_at_point_time",
		Description:      "Return stored or explicitly enabled live weather for an owned activity or explicit point/time.",
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"activity_id":{"type":"string","format":"uuid"},"at_time":{"type":"string","format":"date-time"},"offset_s":{"type":"number","minimum":0},"lat":{"type":"number","minimum":-90,"maximum":90},"lng":{"type":"number","minimum":-180,"maximum":180},"time":{"type":"string","format":"date-time"},"query_kind":{"type":"string","enum":["stored_activity_weather","activity_point_time","current_place","dated_place"]},"mode":{"type":"string","enum":["stored_only","stored_then_fetch","fetch_only"]},"stored_only":{"type":"boolean"},"limit":{"type":"integer","minimum":1,"maximum":20}},"additionalProperties":false}`),
		Permission:       PermissionRead,
		RateLimitPerHour: 120,
		Timeout:          15 * time.Second,
		IsUserOwned:      true,
		ExecuteRead:      executeQueryWeatherAtPointTime,
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
	mode, err := normalizeWeatherMode(args)
	if err != nil {
		return nil, err
	}
	queryKind := weatherQueryKind(args)
	rateLimit := liveWeatherQuotaNotConsumed()

	if args.ActivityID != "" && mode != "fetch_only" {
		sql, params, err := buildWeatherAtPointTimeSQL(userID, args)
		if err != nil {
			return nil, err
		}
		rows, err := queryActivityContextRows(ctx, deps, userID, sql, params...)
		if err != nil {
			return nil, err
		}
		if len(rows) > 0 || mode == "stored_only" {
			status := "ok"
			detailStatus := "ok"
			if len(rows) == 0 {
				status = "unavailable"
				detailStatus = "no_stored_weather"
			}
			return weatherToolResponse(status, detailStatus, queryKind, mode, rateLimit, map[string]any{
				"weather": rows,
				"count":   len(rows),
				"source":  weatherSourceForStoredRows(rows),
			}), nil
		}
	}

	if mode == "stored_only" {
		return weatherToolResponse("unavailable", "no_stored_weather", queryKind, mode, rateLimit, map[string]any{
			"weather": []map[string]any{},
			"count":   0,
		}), nil
	}

	cfg := liveWeatherConfigFromEnv()
	if blocked := liveWeatherPolicyBlockedResponse(cfg, mode, queryKind, rateLimit); blocked != nil {
		return blocked, nil
	}
	point, err := resolveLiveWeatherPoint(ctx, deps, userID, args)
	if err != nil {
		return nil, err
	}
	if point == nil {
		return weatherToolResponse("unavailable", "no_point_for_time", queryKind, mode, rateLimit, map[string]any{
			"weather": []map[string]any{},
			"count":   0,
		}), nil
	}

	quota := consumeLiveWeatherQuota(userID, cfg, time.Now().UTC())
	if quota["status"] == "rate_limited" {
		return weatherToolResponse("rate_limited", "rate_limited", queryKind, mode, quota, map[string]any{
			"weather":         []map[string]any{},
			"count":           0,
			"blocking_reason": "rate_limited",
		}), nil
	}
	readings, err := enrichment.FetchWeatherReadings(ctx, []enrichment.WeatherRequest{{
		ReadingType: "single",
		Lat:         point.Lat,
		Lng:         point.Lng,
		ElevationM:  point.ElevationM,
		At:          point.At,
	}})
	if err != nil {
		return weatherToolResponse("unavailable", "live_weather_unreachable", queryKind, mode, quota, map[string]any{
			"weather": []map[string]any{},
			"count":   0,
		}), nil
	}
	weather := weatherReadingsForTool(readings)
	return weatherToolResponse("ok", "ok", queryKind, mode, quota, map[string]any{
		"weather": weather,
		"count":   len(weather),
		"source":  "live_provider",
	}), nil
}

func weatherQueryKind(args queryWeatherAtPointTimeArgs) string {
	if strings.TrimSpace(args.QueryKind) != "" {
		return strings.TrimSpace(args.QueryKind)
	}
	if args.ActivityID != "" && (args.OffsetS != nil || strings.TrimSpace(args.AtTime) != "") {
		return "activity_point_time"
	}
	if args.Lat != nil && args.Lng != nil && strings.TrimSpace(args.Time) != "" {
		return "dated_place"
	}
	if args.Lat != nil && args.Lng != nil {
		return "current_place"
	}
	return "stored_activity_weather"
}

func weatherToolResponse(status, detailStatus, queryKind, mode string, rateLimit map[string]any, extra map[string]any) map[string]any {
	out := map[string]any{
		"status":        status,
		"detail_status": detailStatus,
		"query_kind":    queryKind,
		"mode":          mode,
		"rate_limit":    rateLimit,
		"missing":       weatherMissingForDetail(detailStatus),
		"needs":         weatherNeedsForDetail(detailStatus),
		"next_action":   weatherNextActionForDetail(detailStatus),
		"user_message":  weatherToolUserMessage(detailStatus),
	}
	if status == "rate_limited" {
		out["blocking_reason"] = "rate_limited"
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}

func weatherMissingForDetail(detailStatus string) []string {
	switch detailStatus {
	case "no_stored_weather":
		return []string{"stored_activity_weather"}
	case "live_weather_disabled":
		return []string{"live_weather_policy"}
	case "skipped_by_privacy_policy":
		return []string{"external_weather_policy"}
	case "live_weather_manual_confirmation_not_supported":
		return []string{"external_weather_confirmation_flow"}
	case "live_weather_unconfigured":
		return []string{"weather_provider_config"}
	case "no_point_for_time":
		return []string{"weather_point_time"}
	case "live_weather_unreachable":
		return []string{"weather_provider_reachable"}
	default:
		return []string{}
	}
}

func weatherNeedsForDetail(detailStatus string) []string {
	switch detailStatus {
	case "ok":
		return []string{}
	case "rate_limited":
		return []string{"wait_for_weather_rate_limit"}
	case "no_stored_weather":
		return []string{"stored_weather_or_live_weather"}
	case "no_point_for_time":
		return []string{"lat_lng_time_or_activity_stream_point"}
	default:
		return []string{"weather_configuration"}
	}
}

func weatherNextActionForDetail(detailStatus string) string {
	switch detailStatus {
	case "ok":
		return "answer_with_weather_result"
	case "rate_limited":
		return "report_weather_rate_limit"
	case "no_stored_weather":
		return "report_no_stored_weather_or_try_live_if_allowed"
	case "no_point_for_time":
		return "ask_for_place_and_time_or_activity_point"
	case "live_weather_unreachable":
		return "report_weather_provider_unreachable"
	default:
		return "report_weather_capability_limit"
	}
}

func weatherToolUserMessage(detailStatus string) string {
	switch detailStatus {
	case "ok":
		return "Wetterdaten wurden gefunden."
	case "no_stored_weather":
		return "Zu dieser Anfrage sind keine gespeicherten Wetterdaten vorhanden."
	case "live_weather_disabled":
		return "Live-Wetterabfragen sind deaktiviert."
	case "skipped_by_privacy_policy":
		return "Externe Wetterabfragen sind durch die Datenschutzrichtlinie blockiert."
	case "live_weather_manual_confirmation_not_supported":
		return "Externe Wetterabfragen mit manueller Bestaetigung werden hier noch nicht unterstuetzt."
	case "live_weather_unconfigured":
		return "Live-Wetterabfragen sind nicht konfiguriert."
	case "no_point_for_time":
		return "Fuer diese Wetterabfrage fehlt ein eindeutiger Ort und Zeitpunkt."
	case "live_weather_unreachable":
		return "Der Wetteranbieter ist derzeit nicht erreichbar."
	case "rate_limited":
		return "Das Wetter-Abfragelimit ist erreicht."
	default:
		return "Wetterdaten sind fuer diese Anfrage nicht verfuegbar."
	}
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

func normalizeWeatherMode(args queryWeatherAtPointTimeArgs) (string, error) {
	mode := strings.TrimSpace(args.Mode)
	if mode == "" {
		mode = "stored_only"
		if args.Lat != nil && args.Lng != nil && strings.TrimSpace(args.Time) != "" {
			mode = "stored_then_fetch"
		}
		if args.StoredOnly != nil && !*args.StoredOnly {
			mode = "stored_then_fetch"
		}
	}
	switch mode {
	case "stored_only", "stored_then_fetch", "fetch_only":
		return mode, nil
	default:
		return "", fmt.Errorf("invalid mode %q", args.Mode)
	}
}

func weatherSourceForStoredRows(rows []map[string]any) any {
	if len(rows) == 0 {
		return nil
	}
	return "stored_activity_weather"
}

func liveWeatherConfigFromEnv() liveWeatherConfig {
	limit, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("SKADIA_AGENT_LIVE_WEATHER_LIMIT_PER_HOUR")))
	return liveWeatherConfig{
		Policy:          normalizeLiveWeatherPolicy(os.Getenv("SKADIA_AGENT_LIVE_WEATHER_POLICY")),
		ProviderEnabled: parseLiveWeatherBool(os.Getenv("SKADIA_AGENT_LIVE_WEATHER_PROVIDER_ENABLED")),
		LimitPerHour:    limit,
	}
}

func normalizeLiveWeatherPolicy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "allowed":
		return "allowed"
	case "manual_confirm":
		return "manual_confirm"
	case "blocked", "privacy_blocked":
		return "blocked"
	default:
		return "disabled"
	}
}

func parseLiveWeatherBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}

func liveWeatherPolicyBlockedResponse(cfg liveWeatherConfig, mode, queryKind string, rateLimit map[string]any) map[string]any {
	switch cfg.Policy {
	case "disabled":
		return weatherToolResponse("unavailable", "live_weather_disabled", queryKind, mode, rateLimit, map[string]any{
			"weather": []map[string]any{},
			"count":   0,
		})
	case "blocked":
		return weatherToolResponse("unavailable", "skipped_by_privacy_policy", queryKind, mode, rateLimit, map[string]any{
			"weather": []map[string]any{},
			"count":   0,
		})
	case "manual_confirm":
		return weatherToolResponse("unavailable", "live_weather_manual_confirmation_not_supported", queryKind, mode, rateLimit, map[string]any{
			"weather":         []map[string]any{},
			"count":           0,
			"blocking_reason": "manual_confirmation_not_supported",
		})
	case "allowed":
		if !cfg.ProviderEnabled {
			return weatherToolResponse("unavailable", "live_weather_unconfigured", queryKind, mode, rateLimit, map[string]any{
				"weather": []map[string]any{},
				"count":   0,
			})
		}
	}
	return nil
}

func liveWeatherQuotaNotConsumed() map[string]any {
	return map[string]any{
		"status":         "not_consumed",
		"limit_per_hour": nil,
		"remaining":      nil,
		"reset_at":       nil,
		"consumed":       false,
	}
}

func consumeLiveWeatherQuota(userID string, cfg liveWeatherConfig, now time.Time) map[string]any {
	resetAt := now.UTC().Truncate(time.Hour).Add(time.Hour)
	if cfg.LimitPerHour <= 0 {
		return map[string]any{
			"status":         "available",
			"limit_per_hour": nil,
			"remaining":      nil,
			"reset_at":       resetAt,
			"consumed":       true,
		}
	}

	hour := now.UTC().Truncate(time.Hour)
	key := userID + "|" + hour.Format(time.RFC3339)
	liveWeatherQuotaMu.Lock()
	defer liveWeatherQuotaMu.Unlock()
	bucket := liveWeatherQuotaBuckets[key]
	if !bucket.Hour.Equal(hour) {
		bucket = liveWeatherQuotaBucket{Hour: hour}
	}
	if bucket.Count >= cfg.LimitPerHour {
		return map[string]any{
			"status":          "rate_limited",
			"limit_per_hour":  cfg.LimitPerHour,
			"remaining":       0,
			"reset_at":        resetAt,
			"consumed":        false,
			"blocking_reason": "rate_limited",
		}
	}
	bucket.Count++
	liveWeatherQuotaBuckets[key] = bucket
	return map[string]any{
		"status":         "available",
		"limit_per_hour": cfg.LimitPerHour,
		"remaining":      cfg.LimitPerHour - bucket.Count,
		"reset_at":       resetAt,
		"consumed":       true,
	}
}

func resolveLiveWeatherPoint(ctx context.Context, deps *Deps, userID string, args queryWeatherAtPointTimeArgs) (*liveWeatherPoint, error) {
	if args.Lat != nil || args.Lng != nil || args.Time != "" || args.AtTime != "" {
		return explicitLiveWeatherPoint(args)
	}
	if args.ActivityID == "" {
		return nil, nil
	}
	sql, params, err := buildWeatherActivityPointSQL(userID, args)
	if err != nil {
		return nil, err
	}
	rows, err := queryActivityContextRows(ctx, deps, userID, sql, params...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return liveWeatherPointFromRow(rows[0])
}

func explicitLiveWeatherPoint(args queryWeatherAtPointTimeArgs) (*liveWeatherPoint, error) {
	atRaw := args.Time
	if atRaw == "" {
		atRaw = args.AtTime
	}
	if args.Lat == nil || args.Lng == nil || atRaw == "" {
		return nil, nil
	}
	at, err := time.Parse(time.RFC3339, atRaw)
	if err != nil {
		return nil, fmt.Errorf("invalid time %q: %w", atRaw, err)
	}
	return &liveWeatherPoint{
		Lat: *args.Lat,
		Lng: *args.Lng,
		At:  at,
	}, nil
}

func buildWeatherActivityPointSQL(userID string, args queryWeatherAtPointTimeArgs) (string, []any, error) {
	activityID, err := uuid.Parse(args.ActivityID)
	if err != nil {
		return "", nil, fmt.Errorf("invalid activity_id: %w", err)
	}
	params := []any{userID, activityID.String()}
	targetExpr := "a.started_at"
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
	sql := fmt.Sprintf(`SELECT sp.time, sp.lat, sp.lng, sp.altitude_m
FROM activities a
LEFT JOIN LATERAL (
  SELECT s.time, s.lat, s.lng, s.altitude_m
  FROM activity_streams s
  WHERE s.activity_id = a.id AND s.lat IS NOT NULL AND s.lng IS NOT NULL
  ORDER BY ABS(EXTRACT(EPOCH FROM (s.time - %s))) ASC NULLS LAST, s.time ASC
  LIMIT 1
) sp ON true
WHERE a.user_id = $1 AND a.id::text = $2`, targetExpr)
	return sql, params, nil
}

func liveWeatherPointFromRow(row map[string]any) (*liveWeatherPoint, error) {
	lat, latOK := floatFromAny(row["lat"])
	lng, lngOK := floatFromAny(row["lng"])
	at, timeOK := timeFromAny(row["time"])
	if !latOK || !lngOK || !timeOK {
		return nil, nil
	}
	elevation, _ := floatFromAny(row["altitude_m"])
	return &liveWeatherPoint{Lat: lat, Lng: lng, ElevationM: elevation, At: at}, nil
}

func weatherReadingsForTool(readings []model.WeatherReading) []map[string]any {
	out := make([]map[string]any, 0, len(readings))
	for _, r := range readings {
		out = append(out, map[string]any{
			"reading_type":     r.ReadingType,
			"label":            r.Label,
			"recorded_at":      r.RecordedAt,
			"lat":              r.Lat,
			"lng":              r.Lng,
			"elevation_m":      r.ElevationM,
			"temp_c":           r.TempC,
			"apparent_temp_c":  r.ApparentTempC,
			"precipitation_mm": r.PrecipitationMM,
			"wind_speed_ms":    r.WindSpeedMS,
			"wind_gusts_ms":    r.WindGustsMS,
			"humidity":         r.Humidity,
			"uv_index":         r.UVIndex,
			"freezing_level_m": r.FreezingLevelM,
			"condition":        r.Condition,
			"tags":             r.Tags,
			"model":            r.Model,
		})
	}
	return out
}

func floatFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

func timeFromAny(value any) (time.Time, bool) {
	switch v := value.(type) {
	case time.Time:
		return v, true
	default:
		return time.Time{}, false
	}
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
