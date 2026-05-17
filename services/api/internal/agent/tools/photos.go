package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
)

var searchPhotosInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {"type": "string", "description": "Visual content search query"},
    "limit": {"type": "integer", "minimum": 1, "maximum": 50, "default": 10}
  },
  "required": ["query"],
  "additionalProperties": false
}`)

func SearchPhotosByContentTool() *Tool {
	return &Tool{
		Name:        "search_photos_by_content",
		Description: "Search the user's activity photos by visual content using text embeddings.",
		InputSchema: searchPhotosInputSchema,
		Permission:  PermissionRead,
		AuditMode:   AuditNone,
		IsUserOwned: true,
		ExecuteRead: executeSearchPhotosByContent,
		RequiredCapabilities: []string{
			capabilities.KeyPhotoEmbeddings,
		},
	}
}

func executeSearchPhotosByContent(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (any, error) {
	if deps == nil || deps.DB == nil {
		return nil, fmt.Errorf("search_photos_by_content requires DB")
	}
	var req struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("parse search_photos_by_content args: %w", err)
	}
	if req.Query == "" {
		return nil, fmt.Errorf("search_photos_by_content requires query")
	}
	if req.Limit <= 0 {
		req.Limit = 10
	}
	if req.Limit > 50 {
		req.Limit = 50
	}

	var indexedCount int
	if err := deps.DB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM activity_photos
		WHERE user_id = $1 AND clip_embedding IS NOT NULL
	`, userID).Scan(&indexedCount); err != nil {
		return nil, err
	}
	if indexedCount == 0 {
		return map[string]any{"query": req.Query, "photos": []map[string]any{}, "count": 0, "reason": "not_indexed"}, nil
	}

	embedding, err := embedText(ctx, deps.MLBaseURL, req.Query)
	if err != nil {
		return map[string]any{"query": req.Query, "photos": []map[string]any{}, "count": 0, "error": err.Error()}, nil
	}
	vector, err := pgVectorLiteral(embedding)
	if err != nil {
		return nil, err
	}

	rows, err := deps.DB.Pool.Query(ctx, `
		SELECT ap.id::text,
		       COALESCE(ap.activity_id::text, ''),
		       COALESCE(ap.file_id::text, ''),
		       ap.lat,
		       ap.lng,
		       ap.taken_at::date,
		       1 - (ap.clip_embedding <=> $2::vector) AS similarity,
		       COALESCE(a.title, '') AS activity_title
		FROM activity_photos ap
		LEFT JOIN activities a ON a.id = ap.activity_id AND a.user_id = ap.user_id
		WHERE ap.user_id = $1 AND ap.clip_embedding IS NOT NULL
		ORDER BY ap.clip_embedding <=> $2::vector
		LIMIT $3
	`, userID, vector, req.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	photos := make([]map[string]any, 0, req.Limit)
	for rows.Next() {
		var id, activityID, fileID, activityTitle string
		var lat, lng *float64
		var takenAt *time.Time
		var similarity float64
		if err := rows.Scan(&id, &activityID, &fileID, &lat, &lng, &takenAt, &similarity, &activityTitle); err != nil {
			return nil, err
		}
		photo := map[string]any{
			"id":             id,
			"activity_id":    emptyStringNil(activityID),
			"file_id":        emptyStringNil(fileID),
			"lat":            lat,
			"lng":            lng,
			"date":           isoDate(takenAt),
			"similarity":     math.Round(similarity*1000) / 1000,
			"activity_title": activityTitle,
		}
		photos = append(photos, photo)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"query": req.Query, "photos": photos, "count": len(photos)}, nil
}

func isoDate(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

func emptyStringNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}
