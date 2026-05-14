package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/jackc/pgx/v5"
)

var searchDocumentsInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "minLength": 1,
      "description": "Text to search in extracted OCR text, markdown, structured extraction, filenames, and document titles"
    },
    "gear_id": {
      "type": "string",
      "format": "uuid",
      "description": "Optional gear UUID to restrict the search"
    },
    "component_id": {
      "type": "string",
      "format": "uuid",
      "description": "Optional component UUID to restrict the search"
    },
    "service_event_id": {
      "type": "string",
      "format": "uuid",
      "description": "Optional service event UUID to restrict the search"
    },
    "limit": {
      "type": "integer",
      "minimum": 1,
      "maximum": 20
    }
  },
  "required": ["query"],
  "additionalProperties": false
}`)

type searchDocumentsArgs struct {
	Query          string `json:"query"`
	GearID         string `json:"gear_id,omitempty"`
	ComponentID    string `json:"component_id,omitempty"`
	ServiceEventID string `json:"service_event_id,omitempty"`
	Limit          *int   `json:"limit,omitempty"`
}

func RegisterDocumentTools(reg *Registry) {
	for _, tool := range DocumentTools() {
		reg.Register(tool)
	}
}

func DocumentTools() []*Tool {
	return []*Tool{
		{
			Name:        "search_documents",
			Description: "Search user-owned gear documents by extracted OCR text, markdown, structured fields, filenames, or titles. Returns snippets only.",
			InputSchema: searchDocumentsInputSchema,
			Permission:  PermissionRead,
			AuditMode:   AuditRead,
			IsUserOwned: true,
			ExecuteRead: executeSearchDocuments,
			RequiredCapabilities: []string{
				capabilities.KeyDocumentML,
			},
		},
	}
}

func executeSearchDocuments(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args searchDocumentsArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("parse search_documents args: %w", err)
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return nil, fmt.Errorf("search_documents requires query")
	}
	if !chatDocumentTextEnabled(ctx, deps, userID) {
		return map[string]any{
			"documents": []map[string]any{},
			"count":     0,
			"reason":    "document_text_not_enabled",
		}, nil
	}

	limit := boundedLimit(args.Limit, 10, 20)
	rows, err := queryDocumentSearchRows(ctx, deps, userID, args, query, limit)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"documents": rows,
		"count":     len(rows),
	}, nil
}

func chatDocumentTextEnabled(ctx context.Context, deps *Deps, userID string) bool {
	if deps == nil || deps.DB == nil {
		return false
	}
	prefs, err := deps.DB.GetChatPrefs(ctx, userID)
	return err == nil && prefs != nil && prefs.ExposeDocumentExtractedData
}

func queryDocumentSearchRows(ctx context.Context, deps *Deps, userID string, args searchDocumentsArgs, query string, limit int) ([]map[string]any, error) {
	pool, err := readonlyPool(deps)
	if err != nil {
		return nil, err
	}

	params := []any{userID}
	conditions := []string{"user_id = $1"}
	if strings.TrimSpace(args.GearID) != "" {
		params = append(params, strings.TrimSpace(args.GearID))
		conditions = append(conditions, fmt.Sprintf("gear_id = $%d::uuid", len(params)))
	}
	if strings.TrimSpace(args.ComponentID) != "" {
		params = append(params, strings.TrimSpace(args.ComponentID))
		conditions = append(conditions, fmt.Sprintf("component_id = $%d::uuid", len(params)))
	}
	if strings.TrimSpace(args.ServiceEventID) != "" {
		params = append(params, strings.TrimSpace(args.ServiceEventID))
		conditions = append(conditions, fmt.Sprintf("service_event_id = $%d::uuid", len(params)))
	}
	params = append(params, "%"+escapeLikeContains(query)+"%")
	queryParam := len(params)
	conditions = append(conditions, fmt.Sprintf(`(
		COALESCE(extracted_markdown, '') ILIKE $%d ESCAPE '\'
		OR COALESCE(extracted_text, '') ILIKE $%d ESCAPE '\'
		OR COALESCE(extracted_data::text, '') ILIKE $%d ESCAPE '\'
		OR COALESCE(title, '') ILIKE $%d ESCAPE '\'
		OR original_filename ILIKE $%d ESCAPE '\'
	)`, queryParam, queryParam, queryParam, queryParam, queryParam))
	params = append(params, limit)
	limitParam := len(params)

	sql := fmt.Sprintf(`
		SELECT id::text, COALESCE(gear_id::text, '') AS gear_id,
		       COALESCE(component_id::text, '') AS component_id,
		       COALESCE(service_event_id::text, '') AS service_event_id,
		       doc_type, COALESCE(title, '') AS title, original_filename,
		       COALESCE(document_date::text, '') AS document_date,
		       processing_status, created_at::date::text AS date,
		       COALESCE(extracted_markdown, '') AS extracted_markdown,
		       COALESCE(extracted_text, '') AS extracted_text,
		       COALESCE(extracted_data::text, '') AS extracted_data
		  FROM gear_documents
		 WHERE %s
		 ORDER BY created_at DESC
		 LIMIT $%d`, strings.Join(conditions, " AND "), limitParam)

	rows, err := pool.Query(ctx, sql, params...)
	if err != nil {
		return nil, fmt.Errorf("search documents: %w", err)
	}
	defer rows.Close()
	return scanDocumentSearchRows(rows, query)
}

func scanDocumentSearchRows(rows pgx.Rows, query string) ([]map[string]any, error) {
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, gearID, componentID, serviceEventID         string
			docType, title, filename, documentDate          string
			processingStatus, createdDate                   string
			extractedMarkdown, extractedText, extractedData string
		)
		if err := rows.Scan(
			&id, &gearID, &componentID, &serviceEventID,
			&docType, &title, &filename, &documentDate,
			&processingStatus, &createdDate,
			&extractedMarkdown, &extractedText, &extractedData,
		); err != nil {
			return nil, fmt.Errorf("scan document search: %w", err)
		}
		doc := map[string]any{
			"id":                id,
			"doc_type":          docType,
			"title":             emptyStringToNil(title),
			"original_filename": filename,
			"document_date":     emptyStringToNil(documentDate),
			"processing_status": processingStatus,
			"date":              createdDate,
			"snippet":           documentSearchSnippet(query, extractedMarkdown, extractedText, extractedData, title, filename),
		}
		if gearID != "" {
			doc["gear_id"] = gearID
		}
		if componentID != "" {
			doc["component_id"] = componentID
		}
		if serviceEventID != "" {
			doc["service_event_id"] = serviceEventID
		}
		out = append(out, doc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan document search: %w", err)
	}
	return out, nil
}

func escapeLikeContains(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `%`, `\%`)
	escaped = strings.ReplaceAll(escaped, `_`, `\_`)
	return escaped
}

func documentSearchSnippet(query string, sources ...string) string {
	queryLower := strings.ToLower(strings.TrimSpace(query))
	for _, source := range sources {
		normalized := strings.Join(strings.Fields(source), " ")
		if normalized == "" {
			continue
		}
		if queryLower == "" || strings.Contains(strings.ToLower(normalized), queryLower) {
			return truncateRunes(normalized, 600)
		}
	}
	for _, source := range sources {
		normalized := strings.Join(strings.Fields(source), " ")
		if normalized != "" {
			return truncateRunes(normalized, 600)
		}
	}
	return ""
}

func truncateRunes(value string, maxLen int) string {
	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	return string(runes[:maxLen]) + "..."
}

func emptyStringToNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}
