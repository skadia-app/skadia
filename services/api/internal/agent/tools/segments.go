package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type querySegmentsArgs struct {
	Query          string  `json:"query,omitempty"`
	IncludeEfforts bool    `json:"include_efforts,omitempty"`
	Days           int     `json:"days,omitempty"`
	DateFrom       string  `json:"date_from,omitempty"`
	DateTo         string  `json:"date_to,omitempty"`
	Limit          int     `json:"limit,omitempty"`
	SportQuery     string  `json:"sport_query,omitempty"`
	SportType      string  `json:"sport_type,omitempty"`
	SubType        *string `json:"sub_type,omitempty"`
	IsElectric     *bool   `json:"is_electric,omitempty"`
	IsIndoor       *bool   `json:"is_indoor,omitempty"`
	IsVirtual      *bool   `json:"is_virtual,omitempty"`
	IsCommute      *bool   `json:"is_commute,omitempty"`
	InLeaderboard  *bool   `json:"in_leaderboard,omitempty"`
}

func RegisterSegmentTools(reg *Registry) {
	for _, tool := range SegmentTools() {
		reg.Register(tool)
	}
}

func SegmentTools() []*Tool {
	return []*Tool{
		{
			Name:        "query_segments",
			Description: "Search segments and user-owned effort history, including personal record and recent attempts.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"include_efforts":{"type":"boolean","default":true},"days":{"type":"integer","minimum":1,"default":365},"date_from":{"type":"string","format":"date"},"date_to":{"type":"string","format":"date"},"limit":{"type":"integer","minimum":1,"maximum":20,"default":5},"sport_query":{"type":"string","description":"Natural sport phrase to resolve via Skadia taxonomy"},"sport_type":{"type":"string"},"sub_type":{"type":"string"},"is_electric":{"type":"boolean"},"is_indoor":{"type":"boolean"},"is_virtual":{"type":"boolean"},"is_commute":{"type":"boolean"},"in_leaderboard":{"type":"boolean"}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: true,
			ExecuteRead: executeQuerySegments,
		},
	}
}

func executeQuerySegments(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	args := querySegmentsArgs{IncludeEfforts: true}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode query_segments args: %w", err)
	}
	pool, err := readonlyPool(deps)
	if err != nil {
		return nil, err
	}
	return querySegments(ctx, pool, userID, args)
}

func querySegments(ctx context.Context, pool *pgxpool.Pool, userID string, args querySegmentsArgs) (map[string]any, error) {
	sql, params, err := buildQuerySegmentsSQL(userID, args)
	if err != nil {
		return nil, err
	}
	rows, err := queryRowsFromPool(ctx, pool, sql, params...)
	if err != nil {
		return nil, err
	}
	segments := make([]map[string]any, 0, len(rows))
	segmentIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		row["allowed_predicates"] = decodeJSONValue(row["allowed_predicates"])
		row["last_attempt"] = dateString(row["last_attempt"])
		row["pr_date"] = dateString(row["pr_date"])
		if truthyNumber(row["pr_seconds"]) {
			row["pr_formatted"] = formatDuration(row["pr_seconds"])
		}
		segments = append(segments, row)
		segmentIDs = append(segmentIDs, stringValue(row["id"]))
	}
	if args.includeEfforts() && len(segmentIDs) > 0 {
		effortsBySegment, err := queryRecentSegmentEfforts(ctx, pool, userID, segmentIDs, args)
		if err != nil {
			return nil, err
		}
		for _, segment := range segments {
			segment["recent_efforts"] = effortsBySegment[stringValue(segment["id"])]
		}
	}
	return map[string]any{"segments": segments}, nil
}

func buildQuerySegmentsSQL(userID string, args querySegmentsArgs) (string, []any, error) {
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
	conditions := []string{"se.user_id = $1::uuid"}
	params := []any{userID}
	if strings.TrimSpace(args.Query) != "" {
		params = append(params, "%"+args.Query+"%")
		conditions = append(conditions, fmt.Sprintf("s.name ILIKE $%d", len(params)))
	}
	if criteria.DateFrom == "" {
		params = append(params, sinceDate(time.Now().UTC(), args.days()))
		conditions = append(conditions, fmt.Sprintf("se.started_at >= $%d", len(params)))
	}
	conditions, params, err = applySegmentDateFilters(conditions, params, criteria.DateFrom, criteria.DateTo)
	if err != nil {
		return "", nil, err
	}
	conditions, params = applySegmentEffortFilters(conditions, params, SegmentEffortFilters{
		SportType:     criteria.SportType,
		SubType:       criteria.SubType,
		IsElectric:    criteria.IsElectric,
		IsIndoor:      criteria.IsIndoor,
		IsVirtual:     criteria.IsVirtual,
		IsCommute:     criteria.IsCommute,
		InLeaderboard: args.inLeaderboard(),
	})
	sql := `SELECT s.id::text AS id, s.name, s.sport_group, s.allowed_predicates,
	       ROUND((s.distance_m / 1000.0)::numeric, 2)::float8 AS distance_km,
	       s.elevation_gain_m,
	       COUNT(se.id) AS effort_count,
	       MIN(se.elapsed_time_s) AS pr_seconds,
	       (array_agg(se.started_at::date ORDER BY se.elapsed_time_s ASC NULLS LAST, se.started_at ASC))[1] AS pr_date,
	       (array_agg(a.title ORDER BY se.elapsed_time_s ASC NULLS LAST, se.started_at ASC))[1] AS pr_activity_title,
	       (array_agg(a.id::text ORDER BY se.elapsed_time_s ASC NULLS LAST, se.started_at ASC))[1] AS pr_activity_id,
	       MAX(se.started_at)::date AS last_attempt
		FROM segments s
		JOIN segment_efforts se ON se.segment_id = s.id
		JOIN activities a ON a.id = se.activity_id AND a.user_id = se.user_id
		WHERE ` + strings.Join(conditions, " AND ") + `
		GROUP BY s.id, s.name, s.sport_group, s.allowed_predicates, s.distance_m, s.elevation_gain_m
		ORDER BY last_attempt DESC
		LIMIT ` + fmt.Sprint(args.limit())
	return sql, params, nil
}

func queryRecentSegmentEfforts(ctx context.Context, pool *pgxpool.Pool, userID string, segmentIDs []string, args querySegmentsArgs) (map[string][]map[string]any, error) {
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
		return nil, err
	}
	conditions := []string{"se.user_id = $1::uuid", "se.segment_id = ANY($2::uuid[])"}
	params := []any{userID, segmentIDs}
	if criteria.DateFrom == "" {
		params = append(params, sinceDate(time.Now().UTC(), args.days()))
		conditions = append(conditions, fmt.Sprintf("se.started_at >= $%d", len(params)))
	}
	conditions, params, err = applySegmentDateFilters(conditions, params, criteria.DateFrom, criteria.DateTo)
	if err != nil {
		return nil, err
	}
	conditions, params = applySegmentEffortFilters(conditions, params, SegmentEffortFilters{
		SportType:     criteria.SportType,
		SubType:       criteria.SubType,
		IsElectric:    criteria.IsElectric,
		IsIndoor:      criteria.IsIndoor,
		IsVirtual:     criteria.IsVirtual,
		IsCommute:     criteria.IsCommute,
		InLeaderboard: args.inLeaderboard(),
	})
	rows, err := queryRowsFromPool(ctx, pool, `SELECT se.segment_id::text AS segment_id, se.started_at::date AS date, se.elapsed_time_s, se.avg_heart_rate
		FROM segment_efforts se
		JOIN activities a ON a.id = se.activity_id AND a.user_id = se.user_id
		WHERE `+strings.Join(conditions, " AND ")+`
		ORDER BY se.started_at DESC`, params...)
	if err != nil {
		return nil, err
	}
	effortsBySegment := make(map[string][]map[string]any)
	for _, row := range rows {
		segmentID := stringValue(row["segment_id"])
		if len(effortsBySegment[segmentID]) >= 3 {
			continue
		}
		effortsBySegment[segmentID] = append(effortsBySegment[segmentID], map[string]any{
			"date": dateString(row["date"]),
			"time": formatDuration(row["elapsed_time_s"]),
			"hr":   row["avg_heart_rate"],
		})
	}
	return effortsBySegment, nil
}

func (a querySegmentsArgs) inLeaderboard() *bool {
	if a.InLeaderboard != nil {
		return a.InLeaderboard
	}
	v := true
	return &v
}

func applySegmentDateFilters(conditions []string, params []any, dateFrom, dateTo string) ([]string, []any, error) {
	if strings.TrimSpace(dateFrom) != "" {
		from, err := parseDateOnly(dateFrom)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid date_from %q: %w", dateFrom, err)
		}
		params = append(params, from)
		conditions = append(conditions, fmt.Sprintf("se.started_at >= $%d", len(params)))
	}
	if strings.TrimSpace(dateTo) != "" {
		to, err := parseDateOnly(dateTo)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid date_to %q: %w", dateTo, err)
		}
		params = append(params, to.AddDate(0, 0, 1))
		conditions = append(conditions, fmt.Sprintf("se.started_at < $%d", len(params)))
	}
	return conditions, params, nil
}

func (a querySegmentsArgs) days() int {
	if a.Days <= 0 {
		return 365
	}
	return a.Days
}

func (a querySegmentsArgs) limit() int {
	if a.Limit <= 0 {
		return 5
	}
	return min(max(a.Limit, 1), 20)
}

func (a querySegmentsArgs) includeEfforts() bool {
	return a.IncludeEfforts
}
