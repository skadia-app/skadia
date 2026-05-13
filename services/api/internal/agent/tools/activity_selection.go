package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultActivitySelectionDays = 30
	defaultActivitySampleLimit   = 10
	maxActivitySampleLimit       = 50
	maxBulkTagActivityIDs        = 5000
)

var previewActivitySelectionInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "activity_ids": {"type": "array", "items": {"type": "string", "format": "uuid"}, "maxItems": 5000},
    "sport_query": {"type": "string", "description": "Natural sport phrase from the user, resolved by the backend taxonomy, e.g. swim, Freiwasser, Gravel, MTB"},
    "sport_type": {"type": "string"},
    "sub_type": {"type": "string"},
    "days": {"type": "integer", "minimum": 1},
    "date_from": {"type": "string", "format": "date"},
    "date_to": {"type": "string", "format": "date"},
    "min_distance_km": {"type": "number", "minimum": 0},
    "max_distance_km": {"type": "number", "minimum": 0},
    "title_search": {"type": "string"},
    "gear_id": {"type": "string", "format": "uuid"},
    "tag_name": {"type": "string"},
    "is_electric": {"type": "boolean"},
    "is_indoor": {"type": "boolean"},
    "is_virtual": {"type": "boolean"},
    "is_commute": {"type": "boolean"},
    "all_history": {"type": "boolean"},
    "sample_size": {"type": "integer", "minimum": 1, "maximum": 50}
  },
  "additionalProperties": false
}`)

type activitySelectionCriteria struct {
	ActivityIDs   []string `json:"activity_ids,omitempty"`
	SportQuery    string   `json:"sport_query,omitempty"`
	SportType     string   `json:"sport_type,omitempty"`
	SubType       *string  `json:"sub_type,omitempty"`
	Days          *int     `json:"days,omitempty"`
	DateFrom      string   `json:"date_from,omitempty"`
	DateTo        string   `json:"date_to,omitempty"`
	MinDistanceKM *float64 `json:"min_distance_km,omitempty"`
	MaxDistanceKM *float64 `json:"max_distance_km,omitempty"`
	TitleSearch   string   `json:"title_search,omitempty"`
	GearID        string   `json:"gear_id,omitempty"`
	TagName       string   `json:"tag_name,omitempty"`
	IsElectric    *bool    `json:"is_electric,omitempty"`
	IsIndoor      *bool    `json:"is_indoor,omitempty"`
	IsVirtual     *bool    `json:"is_virtual,omitempty"`
	IsCommute     *bool    `json:"is_commute,omitempty"`
	AllHistory    bool     `json:"all_history,omitempty"`
}

type previewActivitySelectionArgs struct {
	activitySelectionCriteria
	SampleSize *int `json:"sample_size,omitempty"`
}

type activitySelectionPlan struct {
	Criteria            activitySelectionCriteria
	CriteriaForResponse map[string]any
	Conditions          []string
	Params              []any
	DefaultDaysApplied  bool
}

type activitySelectionResult struct {
	Count      int
	IDs        []string
	Sample     []map[string]any
	Truncated  bool
	Cap        int
	SampleSize int
	Criteria   map[string]any
}

func PreviewActivitySelectionTool() *Tool {
	return &Tool{
		Name:           "preview_activity_selection",
		Description:    "Preview owned activities matching reusable selection criteria without mutating data.",
		InputSchema:    previewActivitySelectionInputSchema,
		Permission:     PermissionRead,
		AuditMode:      AuditPreview,
		IsUserOwned:    true,
		ExecutePreview: executePreviewActivitySelection,
	}
}

func executePreviewActivitySelection(ctx context.Context, deps *PreviewExecutionDeps, userID string, raw json.RawMessage) (any, error) {
	var args previewActivitySelectionArgs
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode preview_activity_selection args: %w", err)
	}
	if deps == nil || deps.Readonly == nil {
		return nil, fmt.Errorf("readonly pool is required")
	}
	sampleSize := defaultActivitySampleLimit
	if args.SampleSize != nil {
		sampleSize = min(max(*args.SampleSize, 1), maxActivitySampleLimit)
	}
	result, err := previewActivitySelection(ctx, deps.Readonly, userID, args.activitySelectionCriteria, time.Now().UTC(), sampleSize)
	if err != nil {
		return nil, err
	}
	return activitySelectionPreviewResponse(result), nil
}

func previewActivitySelection(ctx context.Context, pool *pgxpool.Pool, userID string, criteria activitySelectionCriteria, now time.Time, sampleSize int) (*activitySelectionResult, error) {
	plan, err := buildActivitySelectionPlan(userID, criteria, now)
	if err != nil {
		return nil, err
	}
	countSQL := "SELECT COUNT(*) FROM activities a WHERE " + strings.Join(plan.Conditions, " AND ")
	var count int
	if err := pool.QueryRow(ctx, countSQL, plan.Params...).Scan(&count); err != nil {
		return nil, fmt.Errorf("count activity selection: %w", err)
	}
	sampleSQL := activitySelectionSelectSQL(plan.Conditions, sampleSize)
	rows, err := pool.Query(ctx, sampleSQL, plan.Params...)
	if err != nil {
		return nil, fmt.Errorf("sample activity selection: %w", err)
	}
	defer rows.Close()
	rawSample, err := collectToolRows(rows)
	if err != nil {
		return nil, err
	}
	sample := make([]map[string]any, 0, len(rawSample))
	for _, row := range rawSample {
		sample = append(sample, formatActivityCompact(row))
	}
	return &activitySelectionResult{
		Count:      count,
		Sample:     sample,
		Truncated:  count > sampleSize,
		Cap:        sampleSize,
		SampleSize: len(sample),
		Criteria:   plan.CriteriaForResponse,
	}, nil
}

func resolveActivitySelectionIDs(ctx context.Context, pool *pgxpool.Pool, userID string, criteria activitySelectionCriteria, now time.Time, cap int) (*activitySelectionResult, error) {
	plan, err := buildActivitySelectionPlan(userID, criteria, now)
	if err != nil {
		return nil, err
	}
	countSQL := "SELECT COUNT(*) FROM activities a WHERE " + strings.Join(plan.Conditions, " AND ")
	var count int
	if err := pool.QueryRow(ctx, countSQL, plan.Params...).Scan(&count); err != nil {
		return nil, fmt.Errorf("count activity selection: %w", err)
	}
	idSQL := "SELECT a.id::text FROM activities a WHERE " + strings.Join(plan.Conditions, " AND ") +
		fmt.Sprintf(" ORDER BY a.started_at DESC NULLS LAST, a.id LIMIT %d", cap+1)
	rows, err := pool.Query(ctx, idSQL, plan.Params...)
	if err != nil {
		return nil, fmt.Errorf("resolve activity selection: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0, min(count, cap))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan activity selection id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	truncated := len(ids) > cap
	if truncated {
		ids = ids[:cap]
	}
	return &activitySelectionResult{
		Count:     count,
		IDs:       ids,
		Truncated: truncated || count > cap,
		Cap:       cap,
		Criteria:  plan.CriteriaForResponse,
	}, nil
}

func buildActivitySelectionPlan(userID string, criteria activitySelectionCriteria, now time.Time) (*activitySelectionPlan, error) {
	normalized, err := normalizeActivitySelectionCriteria(criteria)
	if err != nil {
		return nil, err
	}
	conditions := []string{"a.user_id = $1"}
	params := []any{userID}

	if len(normalized.ActivityIDs) > 0 {
		params = append(params, normalized.ActivityIDs)
		conditions = append(conditions, fmt.Sprintf("a.id::text = ANY($%d)", len(params)))
	}

	defaultDaysApplied := false
	hasExplicitTimeBound := normalized.Days != nil || normalized.DateFrom != "" || normalized.DateTo != ""
	if len(normalized.ActivityIDs) == 0 && !hasExplicitTimeBound && !normalized.AllHistory {
		defaultDays := defaultActivitySelectionDays
		normalized.Days = &defaultDays
		defaultDaysApplied = true
	}
	if normalized.Days != nil {
		params = append(params, sinceDate(now, *normalized.Days))
		conditions = append(conditions, fmt.Sprintf("a.started_at >= $%d", len(params)))
	}
	if normalized.DateFrom != "" {
		from, err := parseDateOnly(normalized.DateFrom)
		if err != nil {
			return nil, fmt.Errorf("invalid date_from %q: %w", normalized.DateFrom, err)
		}
		params = append(params, from)
		conditions = append(conditions, fmt.Sprintf("a.started_at >= $%d", len(params)))
	}
	if normalized.DateTo != "" {
		to, err := parseDateOnly(normalized.DateTo)
		if err != nil {
			return nil, fmt.Errorf("invalid date_to %q: %w", normalized.DateTo, err)
		}
		params = append(params, to.AddDate(0, 0, 1))
		conditions = append(conditions, fmt.Sprintf("a.started_at < $%d", len(params)))
	}

	conditions, params = applyActivityFilters(conditions, params, ActivityFilters{
		TableAlias: "a",
		SportType:  normalized.SportType,
		SubType:    normalized.SubType,
		IsElectric: normalized.IsElectric,
		IsIndoor:   normalized.IsIndoor,
		IsVirtual:  normalized.IsVirtual,
		IsCommute:  normalized.IsCommute,
	})
	if normalized.MinDistanceKM != nil {
		params = append(params, *normalized.MinDistanceKM*1000)
		conditions = append(conditions, fmt.Sprintf("a.distance_m >= $%d", len(params)))
	}
	if normalized.MaxDistanceKM != nil {
		params = append(params, *normalized.MaxDistanceKM*1000)
		conditions = append(conditions, fmt.Sprintf("a.distance_m <= $%d", len(params)))
	}
	if normalized.TitleSearch != "" {
		params = append(params, "%"+normalized.TitleSearch+"%")
		conditions = append(conditions, fmt.Sprintf("a.title ILIKE $%d", len(params)))
	}
	if normalized.GearID != "" {
		gearID, err := uuid.Parse(normalized.GearID)
		if err != nil {
			return nil, fmt.Errorf("invalid gear_id: %w", err)
		}
		params = append(params, gearID)
		conditions = append(conditions, fmt.Sprintf("EXISTS (SELECT 1 FROM activity_gear_assignments aga WHERE aga.activity_id = a.id AND aga.user_id = a.user_id AND aga.role_key = 'primary' AND aga.gear_id = $%d)", len(params)))
	}
	if normalized.TagName != "" {
		params = append(params, normalized.TagName)
		conditions = append(conditions, fmt.Sprintf("EXISTS (SELECT 1 FROM activity_tags at JOIN tags t ON t.id = at.tag_id WHERE at.activity_id = a.id AND t.user_id = a.user_id AND lower(t.name) = lower($%d))", len(params)))
	}

	return &activitySelectionPlan{
		Criteria:            normalized,
		CriteriaForResponse: activitySelectionCriteriaResponse(normalized, defaultDaysApplied),
		Conditions:          conditions,
		Params:              params,
		DefaultDaysApplied:  defaultDaysApplied,
	}, nil
}

func normalizeActivitySelectionCriteria(criteria activitySelectionCriteria) (activitySelectionCriteria, error) {
	criteria.SportType = strings.TrimSpace(criteria.SportType)
	criteria.SportQuery = strings.TrimSpace(criteria.SportQuery)
	criteria.DateFrom = strings.TrimSpace(criteria.DateFrom)
	criteria.DateTo = strings.TrimSpace(criteria.DateTo)
	criteria.TitleSearch = strings.TrimSpace(criteria.TitleSearch)
	criteria.GearID = strings.TrimSpace(criteria.GearID)
	criteria.TagName = strings.TrimSpace(criteria.TagName)

	if criteria.Days != nil && *criteria.Days <= 0 {
		return criteria, fmt.Errorf("days must be positive")
	}
	if criteria.MinDistanceKM != nil && *criteria.MinDistanceKM < 0 {
		return criteria, fmt.Errorf("min_distance_km must be non-negative")
	}
	if criteria.MaxDistanceKM != nil && *criteria.MaxDistanceKM < 0 {
		return criteria, fmt.Errorf("max_distance_km must be non-negative")
	}
	if criteria.MinDistanceKM != nil && criteria.MaxDistanceKM != nil && *criteria.MinDistanceKM > *criteria.MaxDistanceKM {
		return criteria, fmt.Errorf("min_distance_km must be <= max_distance_km")
	}
	if criteria.DateFrom != "" && criteria.DateTo != "" {
		from, err := parseDateOnly(criteria.DateFrom)
		if err != nil {
			return criteria, fmt.Errorf("invalid date_from %q: %w", criteria.DateFrom, err)
		}
		to, err := parseDateOnly(criteria.DateTo)
		if err != nil {
			return criteria, fmt.Errorf("invalid date_to %q: %w", criteria.DateTo, err)
		}
		if from.After(to) {
			return criteria, fmt.Errorf("date_from must be <= date_to")
		}
	}
	if criteria.SportQuery != "" {
		match, ok := sport.ResolveNaturalSportQuery(criteria.SportQuery)
		if !ok {
			return criteria, fmt.Errorf("unknown sport_query %q", criteria.SportQuery)
		}
		if criteria.SportType != "" && criteria.SportType != match.Tuple.SportType {
			return criteria, fmt.Errorf("sport_query %q resolves to sport_type %q but sport_type %q was provided", criteria.SportQuery, match.Tuple.SportType, criteria.SportType)
		}
		criteria.SportType = match.Tuple.SportType
		if match.Tuple.SubType != "" && criteria.SubType != nil && *criteria.SubType != match.Tuple.SubType {
			return criteria, fmt.Errorf("sport_query %q resolves to sub_type %q but sub_type %q was provided", criteria.SportQuery, match.Tuple.SubType, *criteria.SubType)
		}
		if match.Tuple.SubType != "" && criteria.SubType == nil {
			criteria.SubType = &match.Tuple.SubType
		}
		applySportQueryModifierPatch(&criteria, match.Patch)
	}
	activityIDs := make([]string, 0, len(criteria.ActivityIDs))
	seen := make(map[string]bool, len(criteria.ActivityIDs))
	for _, raw := range criteria.ActivityIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		parsed, err := uuid.Parse(id)
		if err != nil {
			return criteria, fmt.Errorf("invalid activity_id %q: %w", raw, err)
		}
		canonical := parsed.String()
		if seen[canonical] {
			continue
		}
		seen[canonical] = true
		activityIDs = append(activityIDs, canonical)
	}
	criteria.ActivityIDs = activityIDs
	return criteria, nil
}

func activitySelectionSelectSQL(conditions []string, limit int) string {
	return "SELECT a.id, a.title, a.sport_type, a.sub_type, a.started_at, " +
		"a.distance_m, a.elapsed_time_s, a.avg_heart_rate, a.elevation_gain_m, " +
		"a.avg_speed_ms, a.is_electric, a.is_indoor, a.is_virtual, a.is_commute, a.start_city " +
		"FROM activities a WHERE " + strings.Join(conditions, " AND ") + " " +
		fmt.Sprintf("ORDER BY a.started_at DESC NULLS LAST, a.id LIMIT %d", limit)
}

func activitySelectionPreviewResponse(result *activitySelectionResult) map[string]any {
	return map[string]any{
		"count":             result.Count,
		"sample_activities": result.Sample,
		"criteria":          result.Criteria,
		"cap": map[string]any{
			"sample_limit": result.Cap,
			"write_limit":  maxBulkTagActivityIDs,
		},
		"truncated": result.Truncated,
	}
}

func activitySelectionCriteriaResponse(criteria activitySelectionCriteria, defaultDaysApplied bool) map[string]any {
	out := map[string]any{}
	if len(criteria.ActivityIDs) > 0 {
		out["activity_ids"] = criteria.ActivityIDs
	}
	if criteria.SportQuery != "" {
		out["sport_query"] = criteria.SportQuery
	}
	if criteria.SportType != "" {
		out["sport_type"] = criteria.SportType
	}
	if criteria.SubType != nil {
		out["sub_type"] = *criteria.SubType
	}
	if criteria.Days != nil {
		out["days"] = *criteria.Days
	}
	if criteria.DateFrom != "" {
		out["date_from"] = criteria.DateFrom
	}
	if criteria.DateTo != "" {
		out["date_to"] = criteria.DateTo
	}
	if criteria.MinDistanceKM != nil {
		out["min_distance_km"] = *criteria.MinDistanceKM
	}
	if criteria.MaxDistanceKM != nil {
		out["max_distance_km"] = *criteria.MaxDistanceKM
	}
	if criteria.TitleSearch != "" {
		out["title_search"] = criteria.TitleSearch
	}
	if criteria.GearID != "" {
		out["gear_id"] = criteria.GearID
	}
	if criteria.TagName != "" {
		out["tag_name"] = criteria.TagName
	}
	if criteria.IsElectric != nil {
		out["is_electric"] = *criteria.IsElectric
	}
	if criteria.IsIndoor != nil {
		out["is_indoor"] = *criteria.IsIndoor
	}
	if criteria.IsVirtual != nil {
		out["is_virtual"] = *criteria.IsVirtual
	}
	if criteria.IsCommute != nil {
		out["is_commute"] = *criteria.IsCommute
	}
	if criteria.AllHistory {
		out["all_history"] = true
	}
	if defaultDaysApplied {
		out["default_days_applied"] = true
	}
	return out
}

func applySportQueryModifierPatch(criteria *activitySelectionCriteria, patch sport.ModifierPatch) {
	if criteria == nil {
		return
	}
	if patch.IsElectric != nil && criteria.IsElectric == nil {
		criteria.IsElectric = patch.IsElectric
	}
	if patch.IsIndoor != nil && criteria.IsIndoor == nil {
		criteria.IsIndoor = patch.IsIndoor
	}
	if patch.IsVirtual != nil && criteria.IsVirtual == nil {
		criteria.IsVirtual = patch.IsVirtual
	}
	if patch.IsCommute != nil && criteria.IsCommute == nil {
		criteria.IsCommute = patch.IsCommute
	}
}
