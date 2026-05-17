package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var geoFeatureCategoryMap = map[string]string{
	"lake":  "still_water",
	"river": "flowing_water",
}

var sacScaleOrder = map[string]int{
	"hiking":                    1,
	"mountain_hiking":           2,
	"demanding_mountain_hiking": 3,
	"alpine_hiking":             4,
	"demanding_alpine_hiking":   5,
	"difficult_alpine_hiking":   6,
}

var mtbScaleDigits = regexp.MustCompile(`\d+`)

type queryGeoFeaturesArgs struct {
	FeatureType  string   `json:"feature_type,omitempty"`
	MinElevation *float64 `json:"min_elevation,omitempty"`
	Days         *int     `json:"days,omitempty"`
	Limit        *int     `json:"limit,omitempty"`
	SportQuery   string   `json:"sport_query,omitempty"`
	SportType    string   `json:"sport_type,omitempty"`
	SubType      *string  `json:"sub_type,omitempty"`
	IsElectric   *bool    `json:"is_electric,omitempty"`
	IsIndoor     *bool    `json:"is_indoor,omitempty"`
	IsVirtual    *bool    `json:"is_virtual,omitempty"`
	IsCommute    *bool    `json:"is_commute,omitempty"`
	ActivityIDs  []string `json:"activity_ids,omitempty"`
	DateFrom     string   `json:"date_from,omitempty"`
	DateTo       string   `json:"date_to,omitempty"`
	GroupBy      string   `json:"group_by,omitempty"`
	OrderBy      string   `json:"order_by,omitempty"`
}

type searchPOIsNearbyArgs struct {
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	RadiusM  *int    `json:"radius_m,omitempty"`
	Category string  `json:"category,omitempty"`
	Limit    *int    `json:"limit,omitempty"`
}

type searchTrailsByDifficultyArgs struct {
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	RadiusM     *int    `json:"radius_m,omitempty"`
	MaxSacScale string  `json:"max_sac_scale,omitempty"`
	MaxMTBScale string  `json:"max_mtb_scale,omitempty"`
	Surface     string  `json:"surface,omitempty"`
	Limit       *int    `json:"limit,omitempty"`
}

func RegisterGeoTools(reg *Registry) {
	for _, tool := range GeoTools() {
		reg.Register(tool)
	}
}

func GeoTools() []*Tool {
	return []*Tool{
		{
			Name:        "query_geo_features",
			Description: "Query the user's visited summits, passes, waters, forests, settlements, borders, or gastronomy stops.",
			InputSchema: queryGeoFeaturesInputSchema(),
			Permission:  PermissionRead,
			IsUserOwned: true,
			ExecuteRead: executeQueryGeoFeatures,
			RequiredCapabilities: []string{
				capabilities.KeyGeoPOIEnrichment,
			},
		},
		{
			Name:        "query_activity_geo_context",
			Description: "Answer hybrid questions that combine owned activities with visited geo features, such as highest summit per year and the activity sport used there.",
			InputSchema: queryGeoFeaturesInputSchema(),
			Permission:  PermissionRead,
			IsUserOwned: true,
			ExecuteRead: executeQueryGeoFeatures,
			RequiredCapabilities: []string{
				capabilities.KeyGeoPOIEnrichment,
			},
		},
		{
			Name:        "search_pois_nearby",
			Description: "Search shared OSM POIs near a coordinate.",
			InputSchema: json.RawMessage(`{"type":"object","required":["lat","lng"],"properties":{"lat":{"type":"number"},"lng":{"type":"number"},"radius_m":{"type":"integer","minimum":1},"category":{"type":"string","enum":["summit","pass","settlement","gastronomy","accommodation","shelter","viewpoint","camping","transport","shop","sport","information","landmark","religious"]},"limit":{"type":"integer","minimum":1,"maximum":100}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: false,
			ExecuteRead: executeSearchPOIsNearby,
			RequiredCapabilities: []string{
				capabilities.KeyLocalPOISearch,
			},
		},
		{
			Name:        "search_trails_by_difficulty",
			Description: "Search shared OSM trails by hiking or MTB difficulty near a coordinate.",
			InputSchema: json.RawMessage(`{"type":"object","required":["lat","lng"],"properties":{"lat":{"type":"number"},"lng":{"type":"number"},"radius_m":{"type":"integer","minimum":1},"max_sac_scale":{"type":"string","enum":["hiking","mountain_hiking","demanding_mountain_hiking","alpine_hiking","demanding_alpine_hiking","difficult_alpine_hiking"]},"max_mtb_scale":{"type":"string","pattern":"^[sS]?[0-5]$"},"surface":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":100}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: false,
			ExecuteRead: executeSearchTrailsByDifficulty,
			RequiredCapabilities: []string{
				capabilities.KeySurfaceEnrichment,
			},
		},
	}
}

func queryGeoFeaturesInputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"feature_type":{"type":"string","enum":["lake","river","summit","pass","forest","settlement","border","gastronomy"]},"min_elevation":{"type":"number"},"days":{"type":"integer","minimum":1},"limit":{"type":"integer","minimum":1,"maximum":100},"sport_query":{"type":"string","description":"Natural sport phrase to resolve via Skadia taxonomy"},"sport_type":{"type":"string"},"sub_type":{"type":"string"},"is_electric":{"type":"boolean"},"is_indoor":{"type":"boolean"},"is_virtual":{"type":"boolean"},"is_commute":{"type":"boolean"},"activity_ids":{"type":"array","items":{"type":"string","format":"uuid"}},"date_from":{"type":"string","format":"date"},"date_to":{"type":"string","format":"date"},"group_by":{"type":"string","enum":["none","year","sport","feature"]},"order_by":{"type":"string","enum":["elevation","date","name"]}},"additionalProperties":false}`)
}

func executeQueryGeoFeatures(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args queryGeoFeaturesArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode query_geo_features args: %w", err)
	}
	countSQL, dataSQL, params, _, featureType, err := buildQueryGeoFeaturesSQL(userID, args, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	groupSQL, groupParams, err := buildQueryGeoFeatureGroupsSQL(userID, args, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	var totalCount int64
	var rows []map[string]any
	var grouped []map[string]any
	if deps == nil || deps.DB == nil {
		return nil, fmt.Errorf("database is required")
	}
	if err := deps.DB.WithUserTx(ctx, userID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, countSQL, params...).Scan(&totalCount); err != nil {
			return err
		}
		queryResult, err := tx.Query(ctx, dataSQL, params...)
		if err != nil {
			return err
		}
		defer queryResult.Close()
		rows, err = collectToolRows(queryResult)
		if err != nil {
			return err
		}
		if groupSQL != "" {
			groupResult, err := tx.Query(ctx, groupSQL, groupParams...)
			if err != nil {
				return err
			}
			defer groupResult.Close()
			grouped, err = collectToolRows(groupResult)
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if grouped == nil {
		grouped = groupGeoFeatureRows(rows, args.groupBy())
	}
	return map[string]any{
		"feature_type": featureType,
		"total_count":  totalCount,
		"features":     rows,
		"group_by":     args.groupBy(),
		"grouped":      grouped,
		"note":         fmt.Sprintf("Showing %d of %d total matches", len(rows), totalCount),
	}, nil
}

func executeSearchPOIsNearby(ctx context.Context, deps *Deps, _ string, raw json.RawMessage) (any, error) {
	var args searchPOIsNearbyArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode search_pois_nearby args: %w", err)
	}
	sql, params, err := buildSearchPOIsNearbySQL(args)
	if err != nil {
		return nil, err
	}
	rows, err := queryRows(ctx, deps, sql, params...)
	if err != nil {
		return nil, err
	}
	return map[string]any{"pois": rows, "count": len(rows)}, nil
}

func executeSearchTrailsByDifficulty(ctx context.Context, deps *Deps, _ string, raw json.RawMessage) (any, error) {
	var args searchTrailsByDifficultyArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode search_trails_by_difficulty args: %w", err)
	}
	sql, params, err := buildSearchTrailsByDifficultySQL(args)
	if err != nil {
		return nil, err
	}
	rows, err := queryRows(ctx, deps, sql, params...)
	if err != nil {
		return nil, err
	}
	return map[string]any{"trails": rows, "count": len(rows)}, nil
}

func buildQueryGeoFeaturesSQL(userID string, args queryGeoFeaturesArgs, now time.Time) (string, string, []any, int, string, error) {
	featureType := mappedGeoFeatureType(args.FeatureType)
	since, err := geoSinceDate(args, now)
	if err != nil {
		return "", "", nil, 0, "", err
	}
	limit := clampIntPtr(args.Limit, 20, 1, 100)
	criteria, err := normalizeActivitySelectionCriteria(activitySelectionCriteria{
		SportQuery: args.SportQuery,
		SportType:  args.SportType,
		SubType:    args.SubType,
		IsElectric: args.IsElectric,
		IsIndoor:   args.IsIndoor,
		IsVirtual:  args.IsVirtual,
		IsCommute:  args.IsCommute,
		DateFrom:   args.DateFrom,
		DateTo:     args.DateTo,
	})
	if err != nil {
		return "", "", nil, 0, "", err
	}

	if featureType == "gastronomy" {
		conditions := []string{"a.user_id = $1", "a.started_at >= $2", "s.poi_type IN ('restaurant','alpine_hut','cafe','biergarten','fast_food','pub','bar')"}
		params := []any{userID, since}
		conditions, params, err = applyGeoDateToFilter(conditions, params, criteria.DateTo)
		if err != nil {
			return "", "", nil, 0, "", err
		}
		conditions, params = applyActivityFilters(conditions, params, ActivityFilters{
			TableAlias: "a",
			SportType:  criteria.SportType,
			SubType:    criteria.SubType,
			IsElectric: criteria.IsElectric,
			IsIndoor:   criteria.IsIndoor,
			IsVirtual:  criteria.IsVirtual,
			IsCommute:  criteria.IsCommute,
		})
		where := strings.Join(conditions, " AND ")
		join := "FROM activity_stops s JOIN activities a ON a.id = s.activity_id WHERE " + where
		selectSQL := "SELECT s.poi_name AS name, s.poi_type AS subtype, ST_Y(s.location) AS lat, ST_X(s.location) AS lng, s.duration_s, a.id::text AS activity_id, a.started_at::date AS activity_date, a.title AS activity_title, COALESCE(a.sport_type, '') AS sport_type, COALESCE(a.sub_type, '') AS sub_type "
		return "SELECT COUNT(*) " + join, selectSQL + join + fmt.Sprintf(" ORDER BY %s LIMIT %d", geoFeatureOrderBy(args.OrderBy, "s", "a"), limit), params, limit, featureType, nil
	}

	conditions := []string{"a.user_id = $1", "a.started_at >= $2"}
	params := []any{userID, since}
	conditions, params, err = applyGeoDateToFilter(conditions, params, criteria.DateTo)
	if err != nil {
		return "", "", nil, 0, "", err
	}
	if featureType != "" {
		params = append(params, featureType)
		conditions = append(conditions, fmt.Sprintf("gt.category = $%d", len(params)))
	}
	if args.MinElevation != nil {
		params = append(params, *args.MinElevation)
		conditions = append(conditions, fmt.Sprintf("COALESCE(gt.ele, gt.ele_m) >= $%d", len(params)))
	}
	conditions, params = applyActivityFilters(conditions, params, ActivityFilters{
		TableAlias: "a",
		SportType:  criteria.SportType,
		SubType:    criteria.SubType,
		IsElectric: criteria.IsElectric,
		IsIndoor:   criteria.IsIndoor,
		IsVirtual:  criteria.IsVirtual,
		IsCommute:  criteria.IsCommute,
	})
	if len(args.ActivityIDs) > 0 {
		ids := make([]uuid.UUID, 0, len(args.ActivityIDs))
		for _, id := range args.ActivityIDs {
			parsed, err := uuid.Parse(id)
			if err != nil {
				return "", "", nil, 0, "", fmt.Errorf("invalid activity_id %q: %w", id, err)
			}
			ids = append(ids, parsed)
		}
		params = append(params, ids)
		conditions = append(conditions, fmt.Sprintf("a.id = ANY($%d::uuid[])", len(params)))
	}
	where := strings.Join(conditions, " AND ")
	join := "FROM activity_geo_tags gt JOIN activities a ON a.id = gt.activity_id WHERE " + where
	selectSQL := "SELECT gt.name, gt.category, COALESCE(gt.ele, gt.ele_m) AS ele, gt.lat, gt.lng, a.id::text AS activity_id, a.started_at::date AS activity_date, a.title AS activity_title, COALESCE(a.sport_type, '') AS sport_type, COALESCE(a.sub_type, '') AS sub_type "
	return "SELECT COUNT(*) " + join, selectSQL + join + fmt.Sprintf(" ORDER BY %s LIMIT %d", geoFeatureOrderBy(args.OrderBy, "gt", "a"), limit), params, limit, featureType, nil
}

func buildQueryGeoFeatureGroupsSQL(userID string, args queryGeoFeaturesArgs, now time.Time) (string, []any, error) {
	if args.groupBy() == "none" {
		return "", nil, nil
	}
	featureType := mappedGeoFeatureType(args.FeatureType)
	since, err := geoSinceDate(args, now)
	if err != nil {
		return "", nil, err
	}
	limit := clampIntPtr(args.Limit, 20, 1, 100)
	criteria, err := normalizeActivitySelectionCriteria(activitySelectionCriteria{
		SportQuery: args.SportQuery,
		SportType:  args.SportType,
		SubType:    args.SubType,
		IsElectric: args.IsElectric,
		IsIndoor:   args.IsIndoor,
		IsVirtual:  args.IsVirtual,
		IsCommute:  args.IsCommute,
		DateFrom:   args.DateFrom,
		DateTo:     args.DateTo,
	})
	if err != nil {
		return "", nil, err
	}
	nameExpr := "gt.name"
	categoryExpr := "gt.category"
	eleExpr := "COALESCE(gt.ele, gt.ele_m)"
	latExpr := "gt.lat"
	lngExpr := "gt.lng"
	join := "FROM activity_geo_tags gt JOIN activities a ON a.id = gt.activity_id"
	conditions := []string{"a.user_id = $1", "a.started_at >= $2"}
	params := []any{userID, since}
	if featureType == "gastronomy" {
		nameExpr = "s.poi_name"
		categoryExpr = "s.poi_type"
		eleExpr = "NULL::double precision"
		latExpr = "ST_Y(s.location)"
		lngExpr = "ST_X(s.location)"
		join = "FROM activity_stops s JOIN activities a ON a.id = s.activity_id"
		conditions = append(conditions, "s.poi_type IN ('restaurant','alpine_hut','cafe','biergarten','fast_food','pub','bar')")
	} else if featureType != "" {
		params = append(params, featureType)
		conditions = append(conditions, fmt.Sprintf("gt.category = $%d", len(params)))
	}
	conditions, params, err = applyGeoDateToFilter(conditions, params, criteria.DateTo)
	if err != nil {
		return "", nil, err
	}
	if featureType != "gastronomy" && args.MinElevation != nil {
		params = append(params, *args.MinElevation)
		conditions = append(conditions, fmt.Sprintf("COALESCE(gt.ele, gt.ele_m) >= $%d", len(params)))
	}
	conditions, params = applyActivityFilters(conditions, params, ActivityFilters{
		TableAlias: "a",
		SportType:  criteria.SportType,
		SubType:    criteria.SubType,
		IsElectric: criteria.IsElectric,
		IsIndoor:   criteria.IsIndoor,
		IsVirtual:  criteria.IsVirtual,
		IsCommute:  criteria.IsCommute,
	})
	if len(args.ActivityIDs) > 0 {
		ids := make([]uuid.UUID, 0, len(args.ActivityIDs))
		for _, id := range args.ActivityIDs {
			parsed, err := uuid.Parse(id)
			if err != nil {
				return "", nil, fmt.Errorf("invalid activity_id %q: %w", id, err)
			}
			ids = append(ids, parsed)
		}
		params = append(params, ids)
		conditions = append(conditions, fmt.Sprintf("a.id = ANY($%d::uuid[])", len(params)))
	}
	groupExpr := geoSQLGroupExpr(args.groupBy(), nameExpr, categoryExpr)
	sql := fmt.Sprintf(`
		WITH base AS (
			SELECT %s AS name, %s AS category, %s AS ele, %s AS lat, %s AS lng,
			       a.id::text AS activity_id, a.started_at::date AS activity_date,
			       a.title AS activity_title, COALESCE(a.sport_type, '') AS sport_type,
			       COALESCE(a.sub_type, '') AS sub_type, %s AS group_key
			  %s
			 WHERE %s
		), ranked AS (
			SELECT *, COUNT(*) OVER (PARTITION BY group_key) AS group_count,
			       ROW_NUMBER() OVER (PARTITION BY group_key ORDER BY %s) AS rn
			  FROM base
		)
		SELECT group_key AS "group", group_count AS count, name, category, ele, lat, lng,
		       activity_id, activity_date, activity_title, sport_type, sub_type
		  FROM ranked
		 WHERE rn = 1
		 ORDER BY group_key
		 LIMIT %d`, nameExpr, categoryExpr, eleExpr, latExpr, lngExpr, groupExpr, join, strings.Join(conditions, " AND "), geoRankOrder(args.OrderBy), limit)
	return sql, params, nil
}

func geoSQLGroupExpr(groupBy, nameExpr, categoryExpr string) string {
	switch groupBy {
	case "year":
		return "EXTRACT(YEAR FROM a.started_at)::text"
	case "sport":
		return "CASE WHEN COALESCE(a.sub_type, '') = '' THEN COALESCE(a.sport_type, '') ELSE COALESCE(a.sport_type, '') || '/' || COALESCE(a.sub_type, '') END"
	case "feature":
		return "COALESCE(NULLIF(" + nameExpr + ", ''), " + categoryExpr + ", 'unknown')"
	default:
		return "'all'"
	}
}

func geoRankOrder(orderBy string) string {
	switch orderBy {
	case "date":
		return "activity_date DESC NULLS LAST"
	case "name":
		return "name ASC NULLS LAST"
	default:
		return "ele DESC NULLS LAST, activity_date DESC NULLS LAST"
	}
}

func buildSearchPOIsNearbySQL(args searchPOIsNearbyArgs) (string, []any, error) {
	if !validLatLng(args.Lat, args.Lng) {
		return "", nil, fmt.Errorf("coordinates out of range")
	}
	radius := clampIntPtr(args.RadiusM, 5000, 1, 100000)
	limit := clampIntPtr(args.Limit, 20, 1, 100)
	params := []any{args.Lng, args.Lat, radius}
	sql := `
SELECT name, category, subtype, ele,
       ST_Y(ST_Centroid(geom)) AS lat, ST_X(ST_Centroid(geom)) AS lng,
       ST_Distance(geom::geography, ST_Point($1,$2)::geography) AS dist_m
FROM osm_pois
WHERE ST_DWithin(geom::geography, ST_Point($1,$2)::geography, $3)`
	if args.Category != "" {
		params = append(params, args.Category)
		sql += fmt.Sprintf(" AND category = $%d", len(params))
	}
	params = append(params, limit)
	sql += fmt.Sprintf(" ORDER BY dist_m LIMIT $%d", len(params))
	return sql, params, nil
}

func buildSearchTrailsByDifficultySQL(args searchTrailsByDifficultyArgs) (string, []any, error) {
	if !validLatLng(args.Lat, args.Lng) {
		return "", nil, fmt.Errorf("coordinates out of range")
	}
	radius := clampIntPtr(args.RadiusM, 10000, 1, 100000)
	limit := clampIntPtr(args.Limit, 20, 1, 100)
	params := []any{args.Lng, args.Lat, radius}
	sql := `
SELECT name, highway, sac_scale, mtb_scale, surface, tracktype,
       length_m, avg_slope,
       ST_Distance(geom::geography, ST_Point($1,$2)::geography) AS dist_m
FROM osm_ways
WHERE ST_DWithin(geom::geography, ST_Point($1,$2)::geography, $3)
  AND highway IN ('path', 'footway', 'track', 'bridleway', 'cycleway', 'steps')`
	if args.MaxSacScale != "" {
		maxLevel := sacScaleOrder[args.MaxSacScale]
		if maxLevel == 0 {
			return "", nil, fmt.Errorf("invalid max_sac_scale %q", args.MaxSacScale)
		}
		allowed := make([]string, 0, len(sacScaleOrder))
		for scale, level := range sacScaleOrder {
			if level <= maxLevel {
				allowed = append(allowed, scale)
			}
		}
		params = append(params, allowed)
		sql += fmt.Sprintf(" AND (sac_scale IS NULL OR sac_scale = ANY($%d))", len(params))
	}
	if args.MaxMTBScale != "" {
		maxLevel, err := parseMTBScale(args.MaxMTBScale)
		if err != nil {
			return "", nil, err
		}
		allowed := make([]string, 0, maxLevel+1)
		for i := 0; i <= maxLevel; i++ {
			allowed = append(allowed, strconv.Itoa(i))
		}
		params = append(params, allowed)
		sql += fmt.Sprintf(" AND (mtb_scale IS NULL OR mtb_scale = ANY($%d))", len(params))
	}
	if args.Surface != "" {
		params = append(params, args.Surface)
		sql += fmt.Sprintf(" AND surface = $%d", len(params))
	}
	params = append(params, limit)
	sql += fmt.Sprintf(" ORDER BY dist_m LIMIT $%d", len(params))
	return sql, params, nil
}

func mappedGeoFeatureType(featureType string) string {
	if mapped := geoFeatureCategoryMap[featureType]; mapped != "" {
		return mapped
	}
	return featureType
}

func geoSinceDate(args queryGeoFeaturesArgs, now time.Time) (time.Time, error) {
	if args.DateFrom != "" {
		return parseDateOnly(args.DateFrom)
	}
	return sinceDate(now, clampIntPtr(args.Days, 365, 1, 36500)), nil
}

func applyGeoDateToFilter(conditions []string, params []any, dateTo string) ([]string, []any, error) {
	if dateTo == "" {
		return conditions, params, nil
	}
	parsed, err := parseDateOnly(dateTo)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid date_to %q: %w", dateTo, err)
	}
	params = append(params, parsed.AddDate(0, 0, 1))
	conditions = append(conditions, fmt.Sprintf("a.started_at < $%d", len(params)))
	return conditions, params, nil
}

func geoFeatureOrderBy(orderBy, featureAlias, activityAlias string) string {
	switch orderBy {
	case "date":
		return activityAlias + ".started_at DESC NULLS LAST"
	case "name":
		if featureAlias == "s" {
			return "s.poi_name ASC NULLS LAST"
		}
		return featureAlias + ".name ASC NULLS LAST"
	default:
		if featureAlias == "s" {
			return "s.duration_s DESC NULLS LAST"
		}
		return "COALESCE(" + featureAlias + ".ele, " + featureAlias + ".ele_m) DESC NULLS LAST"
	}
}

func (a queryGeoFeaturesArgs) groupBy() string {
	switch a.GroupBy {
	case "year", "sport", "feature":
		return a.GroupBy
	default:
		return "none"
	}
}

func groupGeoFeatureRows(rows []map[string]any, groupBy string) []map[string]any {
	if groupBy == "" || groupBy == "none" {
		return nil
	}
	type aggregate struct {
		Key          string
		Count        int
		MaxElevation float64
		Top          map[string]any
	}
	groups := map[string]*aggregate{}
	order := []string{}
	for _, row := range rows {
		key := geoGroupKey(row, groupBy)
		if key == "" {
			key = "unknown"
		}
		group := groups[key]
		if group == nil {
			group = &aggregate{Key: key, Top: row}
			groups[key] = group
			order = append(order, key)
		}
		group.Count++
		ele := numberValue(row["ele"])
		if group.Top == nil || ele > group.MaxElevation {
			group.Top = row
			group.MaxElevation = ele
		}
	}
	out := make([]map[string]any, 0, len(order))
	for _, key := range order {
		group := groups[key]
		out = append(out, map[string]any{
			"group":         group.Key,
			"count":         group.Count,
			"max_elevation": group.MaxElevation,
			"top_feature":   group.Top,
		})
	}
	return out
}

func geoGroupKey(row map[string]any, groupBy string) string {
	switch groupBy {
	case "year":
		date := dateString(row["activity_date"])
		if len(date) >= 4 {
			return date[:4]
		}
	case "sport":
		sport := stringValue(row["sport_type"])
		if sub := stringValue(row["sub_type"]); sub != "" {
			return sport + "/" + sub
		}
		return sport
	case "feature":
		if name := stringValue(row["name"]); name != "" {
			return name
		}
		return stringValue(row["category"])
	}
	return ""
}

func parseMTBScale(value string) (int, error) {
	matches := mtbScaleDigits.FindString(value)
	if matches == "" {
		return 0, fmt.Errorf("invalid max_mtb_scale %q", value)
	}
	parsed, err := strconv.Atoi(matches)
	if err != nil || parsed < 0 || parsed > 5 {
		return 0, fmt.Errorf("invalid max_mtb_scale %q", value)
	}
	return parsed, nil
}

func clampIntPtr(value *int, fallback, minValue, maxValue int) int {
	if value == nil {
		return fallback
	}
	return min(max(*value, minValue), maxValue)
}
