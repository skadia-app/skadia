package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var getGearStatsInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "gear_id": {
      "type": "string",
      "description": "UUID of specific gear item (optional; omit for all active gear)"
    },
    "gear_type": {
      "type": "string",
      "description": "Filter by gear type key (optional)"
    }
  },
  "additionalProperties": false
}`)

var getGearForActivityInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "activity_id": {
      "type": "string",
      "description": "UUID of the activity"
    }
  },
  "required": ["activity_id"],
  "additionalProperties": false
}`)

var getGearDocumentsInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "gear_id": {
      "type": "string",
      "description": "UUID of the gear item"
    }
  },
  "required": ["gear_id"],
  "additionalProperties": false
}`)

var gearListInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "status": {
      "type": "string",
      "enum": ["active", "stored", "retired"],
      "description": "Filter by gear status. When set, overrides include_retired."
    },
    "type": {
      "type": "string",
      "description": "Filter by literal gear type prefix, e.g. bike or shoes.running"
    },
    "scope": {
      "type": "string",
      "description": "Filter by scope category, e.g. bike, shoes, other"
    },
    "include_retired": {
      "type": "boolean",
      "description": "Set to true to include retired gear in the results"
    }
  },
  "additionalProperties": false
}`)

var gearComponentDetailInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "gear_id": {
      "type": "string",
      "description": "UUID of the gear item whose components to list"
    },
    "component_type": {
      "type": "string",
      "description": "Optional component type filter"
    },
    "include_retired": {
      "type": "boolean",
      "description": "Include retired/replaced components"
    }
  },
  "required": ["gear_id"],
  "additionalProperties": false
}`)

var gearServiceEventsInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "gear_id": {
      "type": "string",
      "description": "UUID of the gear item; returns events for all its components"
    },
    "component_id": {
      "type": "string",
      "description": "UUID of a specific component"
    },
    "order": {
      "type": "string",
      "enum": ["asc", "desc"],
      "description": "Sort order by performed_at"
    },
    "limit": {
      "type": "integer",
      "description": "Max events to return"
    }
  },
  "additionalProperties": false
}`)

var gearSummaryInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "scope": {
      "type": "string",
      "description": "Filter by gear scope"
    },
    "type_key": {
      "type": "string",
      "description": "Filter by gear type key"
    },
    "since": {
      "type": "string",
      "description": "Start date ISO 8601 (YYYY-MM-DD, inclusive)"
    },
    "until": {
      "type": "string",
      "description": "End date ISO 8601 (YYYY-MM-DD, exclusive)"
    },
    "include_retired": {
      "type": "boolean",
      "description": "Include activities on retired gear"
    }
  },
  "additionalProperties": false
}`)

func GearTools() []*Tool {
	tools := []*Tool{
		GetGearStatsTool(),
		GetGearForActivityTool(),
		GetGearDocumentsTool(),
		GearListTool(),
		GearComponentDetailTool(),
		GearServiceEventsTool(),
		GearSummaryTool(),
	}
	return append(tools, WriteGearTools()...)
}

func RegisterGearTools(reg *Registry) {
	for _, tool := range GearTools() {
		reg.Register(tool)
	}
}

func GetGearStatsTool() *Tool {
	return &Tool{
		Name:        "get_gear_stats",
		Description: "Retrieve gear/equipment statistics: name, type, brand, model, total distance, total activities, and status.",
		InputSchema: getGearStatsInputSchema,
		Permission:  PermissionRead,
		AuditMode:   AuditNone,
		IsUserOwned: true,
		ExecuteRead: executeGetGearStats,
	}
}

func GetGearForActivityTool() *Tool {
	return &Tool{
		Name:        "get_gear_for_activity",
		Description: "Retrieve the primary gear/equipment used in a specific activity.",
		InputSchema: getGearForActivityInputSchema,
		Permission:  PermissionRead,
		AuditMode:   AuditNone,
		IsUserOwned: true,
		ExecuteRead: executeGetGearForActivity,
	}
}

func GetGearDocumentsTool() *Tool {
	return &Tool{
		Name:        "get_gear_documents",
		Description: "Retrieve documents for a specific gear item, including parsed extracted_data when available.",
		InputSchema: getGearDocumentsInputSchema,
		Permission:  PermissionRead,
		AuditMode:   AuditNone,
		IsUserOwned: true,
		ExecuteRead: executeGetGearDocuments,
	}
}

func GearListTool() *Tool {
	return &Tool{
		Name:        "gear_list",
		Description: "List the user's gear items with usage totals, optionally filtered by status, type prefix, or scope.",
		InputSchema: gearListInputSchema,
		Permission:  PermissionRead,
		AuditMode:   AuditNone,
		IsUserOwned: true,
		ExecuteRead: executeGearList,
	}
}

func GearComponentDetailTool() *Tool {
	return &Tool{
		Name:        "gear_component_detail",
		Description: "List components attached to a gear item with their usage totals.",
		InputSchema: gearComponentDetailInputSchema,
		Permission:  PermissionRead,
		AuditMode:   AuditNone,
		IsUserOwned: true,
		ExecuteRead: executeGearComponentDetail,
	}
}

func GearServiceEventsTool() *Tool {
	return &Tool{
		Name:        "gear_service_events",
		Description: "List service and maintenance events for a gear item's components.",
		InputSchema: gearServiceEventsInputSchema,
		Permission:  PermissionRead,
		AuditMode:   AuditNone,
		IsUserOwned: true,
		ExecuteRead: executeGearServiceEvents,
	}
}

func GearSummaryTool() *Tool {
	return &Tool{
		Name:        "gear_summary",
		Description: "Aggregate total kilometers, elevation, and activity count across gear items.",
		InputSchema: gearSummaryInputSchema,
		Permission:  PermissionRead,
		AuditMode:   AuditNone,
		IsUserOwned: true,
		ExecuteRead: executeGearSummary,
	}
}

type getGearStatsArgs struct {
	GearID   string `json:"gear_id"`
	GearType string `json:"gear_type"`
}

func executeGetGearStats(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (any, error) {
	var req getGearStatsArgs
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("parse get_gear_stats args: %w", err)
	}
	pool, err := requireReadPool(deps, "get_gear_stats")
	if err != nil {
		return nil, err
	}

	conditions := []string{"user_id = $1"}
	params := []any{userID}
	if req.GearID != "" {
		params = append(params, req.GearID)
		conditions = append(conditions, fmt.Sprintf("id = $%d", len(params)))
	} else {
		conditions = append(conditions, "is_retired = false")
	}
	if req.GearType != "" {
		params = append(params, req.GearType)
		conditions = append(conditions, fmt.Sprintf("type = $%d", len(params)))
	}

	rows, err := pool.Query(ctx, `
		SELECT id::text, name, type, brand, model,
		       ROUND((total_distance_m / 1000.0)::numeric, 0)::double precision AS total_distance_km,
		       total_activities, status
		  FROM gear
		 WHERE `+strings.Join(conditions, " AND ")+`
		 ORDER BY total_distance_m DESC`, params...)
	if err != nil {
		return nil, fmt.Errorf("get gear stats: %w", err)
	}
	defer rows.Close()

	gear, err := scanGearStatRows(rows)
	if err != nil {
		return nil, err
	}
	return map[string]any{"gear": gear}, nil
}

type getGearForActivityArgs struct {
	ActivityID string `json:"activity_id"`
}

func executeGetGearForActivity(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (any, error) {
	var req getGearForActivityArgs
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("parse get_gear_for_activity args: %w", err)
	}
	if strings.TrimSpace(req.ActivityID) == "" {
		return nil, fmt.Errorf("get_gear_for_activity requires activity_id")
	}
	pool, err := requireReadPool(deps, "get_gear_for_activity")
	if err != nil {
		return nil, err
	}

	var row gearStatRow
	err = pool.QueryRow(ctx, `
		SELECT g.id::text, g.name, g.type, g.brand, g.model,
		       ROUND((g.total_distance_m / 1000.0)::numeric, 0)::double precision AS total_distance_km,
		       g.total_activities, g.status
		  FROM gear g
		  JOIN activity_gear_assignments aga
		    ON aga.gear_id = g.id
		   AND aga.role_key = 'primary'
		   AND aga.user_id = $2
		  JOIN activities a
		    ON a.id = aga.activity_id
		   AND a.user_id = aga.user_id
		 WHERE a.id = $1
		   AND a.user_id = $2`,
		req.ActivityID, userID,
	).Scan(&row.ID, &row.Name, &row.Type, &row.Brand, &row.Model, &row.TotalDistanceKM, &row.TotalActivities, &row.Status)
	if err == nil {
		return map[string]any{"gear": row.mapValue()}, nil
	}
	if err == pgx.ErrNoRows {
		return map[string]any{"gear": nil, "note": "No gear assigned to this activity"}, nil
	}
	return nil, fmt.Errorf("get gear for activity: %w", err)
}

type getGearDocumentsArgs struct {
	GearID string `json:"gear_id"`
}

func executeGetGearDocuments(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (any, error) {
	var req getGearDocumentsArgs
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("parse get_gear_documents args: %w", err)
	}
	if strings.TrimSpace(req.GearID) == "" {
		return nil, fmt.Errorf("get_gear_documents requires gear_id")
	}
	pool, err := requireReadPool(deps, "get_gear_documents")
	if err != nil {
		return nil, err
	}

	docs, err := listGearDocumentsForTool(ctx, pool, userID, req.GearID)
	if err != nil {
		return nil, err
	}
	exposeExtracted := false
	if deps != nil && deps.DB != nil {
		prefs, err := deps.DB.GetChatPrefs(ctx, userID)
		exposeExtracted = err == nil && prefs != nil && prefs.ExposeDocumentExtractedData
	}
	if !exposeExtracted {
		for i := range docs {
			docs[i]["extracted_data"] = nil
		}
	}
	totalCost, currency := totalDocumentCost(docs)
	result := map[string]any{
		"documents": docs,
		"count":     len(docs),
	}
	if totalCost != 0 {
		result["total_cost"] = math.Round(totalCost*100) / 100
		if currency != "" {
			result["total_cost_currency"] = currency
		}
	}
	return result, nil
}

type gearListArgs struct {
	Status         string `json:"status"`
	Type           string `json:"type"`
	Scope          string `json:"scope"`
	IncludeRetired bool   `json:"include_retired"`
}

func executeGearList(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (any, error) {
	var req gearListArgs
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("parse gear_list args: %w", err)
	}
	pool, err := requireReadPool(deps, "gear_list")
	if err != nil {
		return nil, err
	}

	conditions := []string{"user_id = $1"}
	params := []any{userID}
	if req.Status != "" {
		params = append(params, req.Status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(params)))
	} else if !req.IncludeRetired {
		conditions = append(conditions, "is_retired = false")
	}
	if req.Scope != "" {
		params = append(params, req.Scope)
		conditions = append(conditions, fmt.Sprintf("scope = $%d", len(params)))
	}
	if req.Type != "" {
		params = append(params, escapeLikePrefix(req.Type))
		conditions = append(conditions, fmt.Sprintf("type LIKE $%d ESCAPE '\\'", len(params)))
	}

	rows, err := pool.Query(ctx, `
		SELECT id::text, name, type, scope, brand, model, status, is_retired,
		       ROUND((total_distance_m / 1000.0)::numeric, 1)::double precision AS total_distance_km,
		       total_activities, total_time_s
		  FROM gear
		 WHERE `+strings.Join(conditions, " AND ")+`
		 ORDER BY total_distance_m DESC`, params...)
	if err != nil {
		return nil, fmt.Errorf("gear list: %w", err)
	}
	defer rows.Close()

	gear, err := scanGearListRows(rows)
	if err != nil {
		return nil, err
	}
	return map[string]any{"gear": gear, "count": len(gear)}, nil
}

type gearComponentDetailArgs struct {
	GearID         string `json:"gear_id"`
	ComponentType  string `json:"component_type"`
	IncludeRetired bool   `json:"include_retired"`
}

func executeGearComponentDetail(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (any, error) {
	var req gearComponentDetailArgs
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("parse gear_component_detail args: %w", err)
	}
	if strings.TrimSpace(req.GearID) == "" {
		return nil, fmt.Errorf("gear_component_detail requires gear_id")
	}
	pool, err := requireReadPool(deps, "gear_component_detail")
	if err != nil {
		return nil, err
	}

	conditions := []string{"gc.gear_id = $1", "g.user_id = $2"}
	params := []any{req.GearID, userID}
	if !req.IncludeRetired {
		conditions = append(conditions, "gc.is_retired = false")
	}
	if req.ComponentType != "" {
		params = append(params, req.ComponentType)
		conditions = append(conditions, fmt.Sprintf("gc.type = $%d", len(params)))
	}

	rows, err := pool.Query(ctx, `
		SELECT gc.id::text, gc.name, gc.type, gc.brand, gc.model, gc.is_retired, gc.status,
		       ROUND((gc.total_distance_m / 1000.0)::numeric, 1)::double precision AS total_distance_km,
		       gc.total_elevation_gain_m, gc.total_time_s,
		       gc.installed_at::text, gc.removed_at::text
		  FROM gear_components gc
		  JOIN gear g ON g.id = gc.gear_id
		 WHERE `+strings.Join(conditions, " AND ")+`
		 ORDER BY gc.name`, params...)
	if err != nil {
		return nil, fmt.Errorf("gear component detail: %w", err)
	}
	defer rows.Close()

	components, err := scanGearComponentRows(rows)
	if err != nil {
		return nil, err
	}
	return map[string]any{"components": components, "count": len(components)}, nil
}

type gearServiceEventsArgs struct {
	GearID      string `json:"gear_id"`
	ComponentID string `json:"component_id"`
	Order       string `json:"order"`
	Limit       int    `json:"limit"`
}

func executeGearServiceEvents(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (any, error) {
	var req gearServiceEventsArgs
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("parse gear_service_events args: %w", err)
	}
	if strings.TrimSpace(req.GearID) == "" && strings.TrimSpace(req.ComponentID) == "" {
		return map[string]any{"error": "Either gear_id or component_id is required", "events": []map[string]any{}}, nil
	}
	pool, err := requireReadPool(deps, "gear_service_events")
	if err != nil {
		return nil, err
	}

	orderSQL := "DESC"
	if strings.EqualFold(req.Order, "asc") {
		orderSQL = "ASC"
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	var rows pgx.Rows
	if req.ComponentID != "" {
		rows, err = pool.Query(ctx, `
			SELECT gse.id::text, gse.performed_at::text, gse.description, gse.notes,
			       gc.name AS component_name, gc.type AS component_type
			  FROM gear_service_events gse
			  JOIN gear_components gc ON gc.id = gse.component_id
			  JOIN gear g ON g.id = gc.gear_id
			 WHERE gse.component_id = $1
			   AND g.user_id = $2
			 ORDER BY gse.performed_at `+orderSQL+`
			 LIMIT $3`, req.ComponentID, userID, limit)
	} else {
		rows, err = pool.Query(ctx, `
			SELECT gse.id::text, gse.performed_at::text, gse.description, gse.notes,
			       gc.name AS component_name, gc.type AS component_type
			  FROM gear_service_events gse
			  JOIN gear_components gc ON gc.id = gse.component_id
			  JOIN gear g ON g.id = gc.gear_id
			 WHERE gc.gear_id = $1
			   AND g.user_id = $2
			 ORDER BY gse.performed_at `+orderSQL+`
			 LIMIT $3`, req.GearID, userID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("gear service events: %w", err)
	}
	defer rows.Close()

	events, err := scanGearServiceEventRows(rows)
	if err != nil {
		return nil, err
	}
	return map[string]any{"events": events, "count": len(events)}, nil
}

type gearSummaryArgs struct {
	Scope          *string `json:"scope"`
	TypeKey        *string `json:"type_key"`
	Since          *string `json:"since"`
	Until          *string `json:"until"`
	IncludeRetired bool    `json:"include_retired"`
}

func executeGearSummary(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (any, error) {
	var req gearSummaryArgs
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("parse gear_summary args: %w", err)
	}
	if deps == nil || deps.DB == nil {
		return nil, fmt.Errorf("gear_summary requires DB")
	}
	summary, err := deps.DB.GetGearSummary(ctx, userID, database.GearSummaryFilter{
		Scope:          req.Scope,
		TypeKey:        req.TypeKey,
		Since:          req.Since,
		Until:          req.Until,
		IncludeRetired: req.IncludeRetired,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"total_distance_km":      math.Round((summary.TotalDistanceM/1000.0)*10) / 10,
		"total_elevation_gain_m": math.Round(summary.TotalElevationGainM),
		"total_elevation_loss_m": math.Round(summary.TotalElevationLossM),
		"total_time_s":           summary.TotalTimeS,
		"total_moving_time_s":    summary.TotalMovingTimeS,
		"activity_count":         summary.ActivityCount,
		"gear_count":             summary.GearCount,
	}, nil
}

func requireReadPool(deps *Deps, toolName string) (*pgxpool.Pool, error) {
	pool := readPool(deps)
	if pool == nil {
		return nil, fmt.Errorf("%s requires DB", toolName)
	}
	return pool, nil
}

func readPool(deps *Deps) *pgxpool.Pool {
	if deps != nil {
		if deps.Readonly != nil {
			return deps.Readonly
		}
		if deps.DB != nil {
			if deps.DB.ReadonlyPool != nil {
				return deps.DB.ReadonlyPool
			}
			return deps.DB.Pool
		}
	}
	return nil
}

func escapeLikePrefix(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `%`, `\%`)
	escaped = strings.ReplaceAll(escaped, `_`, `\_`)
	return escaped + "%"
}

type gearStatRow struct {
	ID              string
	Name            string
	Type            string
	Brand           *string
	Model           *string
	TotalDistanceKM float64
	TotalActivities int
	Status          string
}

func (r gearStatRow) mapValue() map[string]any {
	return map[string]any{
		"id":                r.ID,
		"name":              r.Name,
		"type":              r.Type,
		"brand":             nullableString(r.Brand),
		"model":             nullableString(r.Model),
		"total_distance_km": r.TotalDistanceKM,
		"total_activities":  r.TotalActivities,
		"status":            r.Status,
	}
}

func scanGearStatRows(rows pgx.Rows) ([]map[string]any, error) {
	out := []map[string]any{}
	for rows.Next() {
		var row gearStatRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Type, &row.Brand, &row.Model, &row.TotalDistanceKM, &row.TotalActivities, &row.Status); err != nil {
			return nil, fmt.Errorf("scan gear stats: %w", err)
		}
		out = append(out, row.mapValue())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan gear stats: %w", err)
	}
	return out, nil
}

func scanGearListRows(rows pgx.Rows) ([]map[string]any, error) {
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, name, typeKey, scope, status string
			brand, model                     *string
			isRetired                        bool
			totalDistanceKM                  float64
			totalActivities                  int
			totalTimeS                       int64
		)
		if err := rows.Scan(&id, &name, &typeKey, &scope, &brand, &model, &status, &isRetired, &totalDistanceKM, &totalActivities, &totalTimeS); err != nil {
			return nil, fmt.Errorf("scan gear list: %w", err)
		}
		out = append(out, map[string]any{
			"id":                id,
			"name":              name,
			"type":              typeKey,
			"scope":             scope,
			"brand":             nullableString(brand),
			"model":             nullableString(model),
			"status":            status,
			"is_retired":        isRetired,
			"total_distance_km": totalDistanceKM,
			"total_activities":  totalActivities,
			"total_time_s":      totalTimeS,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan gear list: %w", err)
	}
	return out, nil
}

func listGearDocumentsForTool(ctx context.Context, pool *pgxpool.Pool, userID, gearID string) ([]map[string]any, error) {
	rows, err := pool.Query(ctx, `
		SELECT doc_type, original_filename, document_date::text, document_amount::double precision,
		       document_currency, extracted_data, processing_status, created_at::date::text AS date
		  FROM gear_documents
		 WHERE gear_id = $1
		   AND user_id = $2
		 ORDER BY created_at`, gearID, userID)
	if err != nil {
		return nil, fmt.Errorf("get gear documents: %w", err)
	}
	defer rows.Close()

	docs := []map[string]any{}
	for rows.Next() {
		var (
			docType, originalFilename, processingStatus, createdDate string
			documentDate, documentCurrency                           *string
			documentAmount                                           *float64
			extractedRaw                                             []byte
		)
		if err := rows.Scan(&docType, &originalFilename, &documentDate, &documentAmount, &documentCurrency, &extractedRaw, &processingStatus, &createdDate); err != nil {
			return nil, fmt.Errorf("scan gear document: %w", err)
		}
		doc := map[string]any{
			"doc_type":          docType,
			"original_filename": originalFilename,
			"document_date":     nullableString(documentDate),
			"document_amount":   nullableFloat(documentAmount),
			"document_currency": nullableString(documentCurrency),
			"processing_status": processingStatus,
			"date":              createdDate,
		}
		if len(extractedRaw) > 0 {
			var extracted any
			if err := json.Unmarshal(extractedRaw, &extracted); err != nil {
				return nil, fmt.Errorf("parse gear document extracted_data: %w", err)
			}
			doc["extracted_data"] = extracted
		} else {
			doc["extracted_data"] = nil
		}
		docs = append(docs, doc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get gear documents: %w", err)
	}
	return docs, nil
}

func totalDocumentCost(docs []map[string]any) (float64, string) {
	var total float64
	var currency string
	for _, doc := range docs {
		amount := numberValue(doc["document_amount"])
		docCurrency := stringValue(doc["document_currency"])
		if extracted, ok := doc["extracted_data"].(map[string]any); ok {
			if amount == 0 {
				amount = numberValue(extracted["total_amount"])
			}
			if amount == 0 {
				amount = numberValue(extracted["amount"])
			}
			if docCurrency == "" {
				docCurrency = stringValue(extracted["currency"])
			}
		}
		if amount != 0 {
			total += amount
			if currency == "" {
				currency = docCurrency
			}
		}
	}
	return total, currency
}

func scanGearComponentRows(rows pgx.Rows) ([]map[string]any, error) {
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, name, typeKey, status            string
			brand, model, installedAt, removedAt *string
			isRetired                            bool
			totalDistanceKM, totalElevationGainM float64
			totalTimeS                           int64
		)
		if err := rows.Scan(&id, &name, &typeKey, &brand, &model, &isRetired, &status, &totalDistanceKM, &totalElevationGainM, &totalTimeS, &installedAt, &removedAt); err != nil {
			return nil, fmt.Errorf("scan gear components: %w", err)
		}
		out = append(out, map[string]any{
			"id":                     id,
			"name":                   name,
			"type":                   typeKey,
			"brand":                  nullableString(brand),
			"model":                  nullableString(model),
			"is_retired":             isRetired,
			"status":                 status,
			"total_distance_km":      totalDistanceKM,
			"total_elevation_gain_m": totalElevationGainM,
			"total_time_s":           totalTimeS,
			"installed_at":           nullableString(installedAt),
			"removed_at":             nullableString(removedAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan gear components: %w", err)
	}
	return out, nil
}

func scanGearServiceEventRows(rows pgx.Rows) ([]map[string]any, error) {
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, performedAt, componentName, componentType string
			description, notes                            *string
		)
		if err := rows.Scan(&id, &performedAt, &description, &notes, &componentName, &componentType); err != nil {
			return nil, fmt.Errorf("scan gear service events: %w", err)
		}
		out = append(out, map[string]any{
			"id":             id,
			"performed_at":   performedAt,
			"description":    nullableString(description),
			"notes":          nullableString(notes),
			"component_name": componentName,
			"component_type": componentType,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan gear service events: %w", err)
	}
	return out, nil
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}
