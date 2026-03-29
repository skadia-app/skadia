package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

const photoFeedPageSize = 20

// ListPhotoFeed — GET /api/v1/photos?sort=date|added&page=1
func (h *Handler) ListPhotoFeed(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	sort := r.URL.Query().Get("sort")
	if sort != "added" {
		sort = "date"
	}
	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	offset := (page - 1) * photoFeedPageSize

	rows, total, err := h.DB.ListPhotoFeed(r.Context(), userID, sort, photoFeedPageSize+1, offset)
	if err != nil {
		slog.Error("list photo feed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "db error", r.URL.Path)
		return
	}

	hasNext := len(rows) > photoFeedPageSize
	if hasNext {
		rows = rows[:photoFeedPageSize]
	}

	// Batch-fetch thumbnail file_ids from user_files in a single query.
	// photoIDs come from a user_id-scoped DB query above — safe to use as batch parameter.
	photoIDs := make([]string, len(rows))
	for i, row := range rows {
		photoIDs[i] = row.Photo.ID
	}
	thumbs := h.batchFetchThumbs(r.Context(), photoIDs, userID)

	for i := range rows {
		fillFeedPhotoURLs(&rows[i].Photo)
		if thumbs != nil {
			rows[i].Photo.ThumbFileID = thumbs[rows[i].Photo.ID]
		}
	}

	groups := groupPhotoFeedRows(rows, sort)

	var nextPage *int
	if hasNext {
		np := page + 1
		nextPage = &np
	}

	httputil.WriteJSON(w, http.StatusOK, model.PhotoFeedPage{
		Groups:   groups,
		NextPage: nextPage,
		Total:    total,
	})
}

// SearchPhotos — GET /api/v1/photos/search?q=text&limit=20
func (h *Handler) SearchPhotos(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httputil.WriteJSON(w, http.StatusOK, []model.PhotoSearchResult{})
		return
	}
	limit := 20
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 50 {
		limit = l
	}

	var clipVecLiteral string
	if h.Config != nil && h.Config.MLServiceURL != "" {
		vec, err := fetchPhotoTextEmbedding(r.Context(), h.Config.MLServiceURL, q)
		if err != nil {
			slog.Warn("photo search: text embedding failed, falling back to title-only", "error", err)
		} else {
			clipVecLiteral = pgvecLiteral(vec)
		}
	}

	results, err := h.DB.SearchPhotos(r.Context(), userID, clipVecLiteral, q, limit)
	if err != nil {
		slog.Error("search photos", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "db error", r.URL.Path)
		return
	}

	// Batch-fetch thumbnail file_ids from user_files in a single query.
	// photoIDs come from a user_id-scoped DB query above — safe to use as batch parameter.
	photoIDs := make([]string, len(results))
	for i, r := range results {
		photoIDs[i] = r.Photo.ID
	}
	thumbs := h.batchFetchThumbs(r.Context(), photoIDs, userID)

	for i := range results {
		fillFeedPhotoURLs(&results[i].Photo)
		if thumbs != nil {
			results[i].Photo.ThumbFileID = thumbs[results[i].Photo.ID]
		}
	}

	httputil.WriteJSON(w, http.StatusOK, results)
}

// fetchPhotoTextEmbedding ruft POST /embed/text beim ML-Service auf.
func fetchPhotoTextEmbedding(ctx context.Context, mlURL, text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]string{"text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mlURL+"/embed/text", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ml service returned %d", resp.StatusCode)
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
		Dim       int       `json:"dim"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Dim != 1152 {
		return nil, fmt.Errorf("unexpected embedding dim %d", result.Dim)
	}
	return result.Embedding, nil
}

// fillFeedPhotoURLs setzt ThumbURL/PreviewURL/FullURL anhand der Photo-ID.
// Kein /api/v1/-Präfix — wird vom Frontend-API-Client hinzugefügt.
func fillFeedPhotoURLs(p *model.ActivityPhoto) {
	base := "/photos/" + p.ID
	p.ThumbURL = base + "/thumb"
	p.FullURL = base + "/full"
	if p.S3KeyPreview != "" {
		p.PreviewURL = base + "/preview"
	} else {
		p.PreviewURL = base + "/full"
	}
}

// pgvecLiteral wandelt einen float32-Slice in ein pgvector-Literal um.
// Sicher: nur float32-Werte, keine User-Strings.
func pgvecLiteral(vec []float32) string {
	b := strings.Builder{}
	b.WriteString("'[")
	for i, v := range vec {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(v), 'f', 6, 32))
	}
	b.WriteString("]'::vector")
	return b.String()
}

// groupPhotoFeedRows gruppiert DB-Zeilen in PhotoFeedGroup-Slices.
func groupPhotoFeedRows(rows []database.PhotoFeedRow, sort string) []model.PhotoFeedGroup {
	groups := make([]model.PhotoFeedGroup, 0)
	var current *model.PhotoFeedGroup

	for _, row := range rows {
		var key string
		if sort == "added" {
			key = row.Photo.CreatedAt.Format("2006-01-02")
		} else {
			if row.ActID != nil {
				key = *row.ActID
			} else {
				key = row.Photo.CreatedAt.Format("2006-01-02")
			}
		}

		if current == nil || current.GroupKey != key {
			if current != nil {
				groups = append(groups, *current)
			}
			current = &model.PhotoFeedGroup{
				GroupKey:  key,
				GroupType: "day",
			}
			if sort == "date" && row.ActID != nil {
				current.GroupType = "activity"
				current.Activity = buildPhotoFeedActivity(row)
			}
		}
		current.Photos = append(current.Photos, row.Photo)
	}
	if current != nil {
		groups = append(groups, *current)
	}
	return groups
}

func buildPhotoFeedActivity(row database.PhotoFeedRow) *model.PhotoFeedActivitySummary {
	if row.ActID == nil {
		return nil
	}
	s := &model.PhotoFeedActivitySummary{
		ID:             *row.ActID,
		TrackGeoJSON:   row.ActTrackGeoJSON,
		Lat:            row.ActLat,
		Lng:            row.ActLng,
		DistanceM:      row.ActDistanceM,
		MovingTimeS:    row.ActMovingTimeS,
		ElevationGainM: row.ActElevationGainM,
		Timezone:       row.ActTimezone,
	}
	if row.ActTitle != nil {
		s.Title = *row.ActTitle
	}
	if row.ActSportType != nil {
		s.SportType = *row.ActSportType
	}
	if row.ActStartedAt != nil {
		t := row.ActStartedAt.Format(time.RFC3339)
		s.StartedAt = &t
	}
	return s
}
