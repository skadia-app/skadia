package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/filestore"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// CreateManualActivityRequest is the JSON body for POST /api/v1/activities/manual.
type CreateManualActivityRequest struct {
	SportType      string   `json:"sport_type"`
	SubType        string   `json:"sub_type,omitempty"`
	Title          string   `json:"title,omitempty"`
	Description    string   `json:"description,omitempty"`
	StartedAt      string   `json:"started_at"`
	ElapsedTimeS   float64  `json:"elapsed_time_s"`
	DistanceM      float64  `json:"distance_m,omitempty"`
	ElevationGainM float64  `json:"elevation_gain_m,omitempty"`
	ElevationLossM float64  `json:"elevation_loss_m,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	GearID         string   `json:"gear_id,omitempty"`
	IsEbike        bool     `json:"is_ebike,omitempty"`
	IsVirtual      bool     `json:"is_virtual,omitempty"`
	WorkoutFeel    *int     `json:"workout_feel,omitempty"`
	WorkoutRpe     *int     `json:"workout_rpe,omitempty"`
}

// CreateManualActivity handles POST /api/v1/activities/manual.
func (h *Handler) CreateManualActivity(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	req, ok := httputil.DecodeJSON[CreateManualActivityRequest](w, r)
	if !ok {
		return
	}

	// Validate sport_type against taxonomy.
	if req.SportType == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "sport_type is required", r.URL.Path)
		return
	}
	if _, valid := sport.GetSportDef(req.SportType); !valid {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "unknown sport_type: "+req.SportType, r.URL.Path)
		return
	}

	if req.ElapsedTimeS <= 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "elapsed_time_s must be positive", r.URL.Path)
		return
	}
	if req.WorkoutFeel != nil && (*req.WorkoutFeel < 0 || *req.WorkoutFeel > 4) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "workout_feel must be 0-4", r.URL.Path)
		return
	}
	if req.WorkoutRpe != nil && (*req.WorkoutRpe < 1 || *req.WorkoutRpe > 10) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "workout_rpe must be 1-10", r.URL.Path)
		return
	}

	startedAt, err := parseFlexibleTime(req.StartedAt)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid started_at: "+err.Error(), r.URL.Path)
		return
	}

	// Build MetaFile (= the JSON source file stored in S3).
	maf := &sidecar.MetaFile{
		SchemaVersion:  3,
		FileFormat:     "manual",
		SportType:      req.SportType,
		SubType:        req.SubType,
		Title:          req.Title,
		Description:    req.Description,
		StartedAt:      startedAt,
		DistanceM:      req.DistanceM,
		ElevationGainM: req.ElevationGainM,
		Tags:           req.Tags,
		GearID:         req.GearID,
		IsEbike:        req.IsEbike,
		IsVirtual:      req.IsVirtual,
		IsManualEntry:  true,
		Source:         "manual",
		WorkoutFeel:    req.WorkoutFeel,
		WorkoutRpe:     req.WorkoutRpe,
		CreatedAt:      time.Now().UTC(),
	}

	jsonBytes, err := maf.Marshal()
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to marshal activity", r.URL.Path)
		return
	}

	// Hash includes CreatedAt so each manual creation is unique (no file_hash dedup).
	hash := sha256.Sum256(jsonBytes)
	hashStr := hex.EncodeToString(hash[:])
	s3Key := fmt.Sprintf("%s/%s.json", userID, hashStr)

	// Upload JSON via FileStore (preferred) or legacy S3 (fallback).
	var manualFileID string
	if h.FileStore != nil {
		rec, putErr := h.FileStore.Put(r.Context(), filestore.FileEntry{
			UserID:      userID,
			FileType:    "activity",
			ContentType: "application/json",
			FileName:    "manual.json",
			SizeBytes:   int64(len(jsonBytes)),
			FileHash:    hashStr,
			Tags:        map[string]string{"format": "manual", "source": "manual"},
		}, bytes.NewReader(jsonBytes))
		if putErr != nil {
			slog.Error("manual activity: FileStore.Put failed", "error", putErr)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to store activity file", r.URL.Path)
			return
		}
		manualFileID = rec.ID
	} else {
		if err := h.Storage.Upload(r.Context(), s3Key, jsonBytes, "application/json"); err != nil {
			slog.Error("manual activity: S3 upload failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to store activity file", r.URL.Path)
			return
		}
	}

	// Build ActivityData for InsertActivity.
	endedAt := startedAt.Add(time.Duration(req.ElapsedTimeS) * time.Second)
	data := &model.ActivityData{
		SportType:      req.SportType,
		SubType:        req.SubType,
		StartedAt:      startedAt,
		EndedAt:        endedAt,
		ElapsedTimeS:   int(req.ElapsedTimeS),
		MovingTimeS:    int(req.ElapsedTimeS),
		DistanceM:      req.DistanceM,
		ElevationGainM: req.ElevationGainM,
		ElevationLossM: req.ElevationLossM,
		IsEbike:        req.IsEbike,
		IsVirtual:      req.IsVirtual,
		IsManualEntry:  true,
		FileHash:       hashStr,
		FileFormat:     "manual",
		OriginalName:   "manual.json",
		Source:         "manual",
	}
	if req.WorkoutFeel != nil {
		data.WorkoutFeel = *req.WorkoutFeel
	}
	if req.WorkoutRpe != nil {
		data.WorkoutRpe = *req.WorkoutRpe
	}

	activity, err := h.DB.InsertActivity(r.Context(), userID, data)
	if err != nil {
		slog.Error("manual activity: insert failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create activity", r.URL.Path)
		return
	}

	// Set title, description via enrichment fields (not in ActivityData/InsertActivity).
	enrichFields := make(map[string]any)
	if req.Title != "" {
		enrichFields["title"] = req.Title
		enrichFields["title_is_manual"] = true
	}
	if req.Description != "" {
		enrichFields["description"] = req.Description
	}
	if len(enrichFields) > 0 {
		if err := h.DB.UpdateEnrichmentFields(r.Context(), activity.ID, enrichFields); err != nil {
			slog.Warn("manual activity: set title/description failed", "id", activity.ID, "error", err)
		}
	}

	// Set gear with ownership check.
	if req.GearID != "" {
		var gearExists bool
		_ = h.DB.Pool.QueryRow(r.Context(),
			"SELECT EXISTS(SELECT 1 FROM gear WHERE id = $1 AND user_id = $2)",
			req.GearID, userID).Scan(&gearExists)
		if gearExists {
			if _, err := h.DB.Pool.Exec(r.Context(),
				"UPDATE activities SET gear_id = $2 WHERE id = $1",
				activity.ID, req.GearID); err != nil {
				slog.Warn("manual activity: set gear failed", "id", activity.ID, "error", err)
			}
		}
	}

	// Resolve tag IDs to names and link via junction table.
	if len(req.Tags) > 0 {
		var tagNames []string
		for _, tagID := range req.Tags {
			var name string
			err := h.DB.Pool.QueryRow(r.Context(),
				"SELECT name FROM tags WHERE id = $1 AND user_id = $2", tagID, userID).Scan(&name)
			if err == nil {
				tagNames = append(tagNames, name)
				_ = h.DB.LinkActivityTag(r.Context(), activity.ID, tagID, "manual")
			}
		}
		if len(tagNames) > 0 {
			if err := h.DB.UpdateEnrichmentFields(r.Context(), activity.ID, map[string]any{"tags": tagNames}); err != nil {
				slog.Warn("manual activity: set tag names failed", "id", activity.ID, "error", err)
			}
		}
	}

	// Set file_id when FileStore was used; otherwise fall back to legacy S3 file info.
	metaKey := s3Key + ".meta"
	if manualFileID != "" {
		if _, execErr := h.DB.Pool.Exec(r.Context(),
			`UPDATE activities SET file_id = $1 WHERE id = $2 AND file_id IS NULL`,
			manualFileID, activity.ID,
		); execErr != nil {
			slog.Warn("manual activity: set file_id failed", "id", activity.ID, "error", execErr)
		}
	} else {
		if err := h.DB.UpdateActivityFileInfo(r.Context(), activity.ID, "skadia-originals", s3Key, "manual", "manual.json", metaKey); err != nil {
			slog.Warn("manual activity: update file info failed", "id", activity.ID, "error", err)
		}
	}

	// Stage 1 enrichment: asyncWeather=true, skipStreamEnrichment=true, skipGeo=true (no GPS).
	enrichment.RunStage1(r.Context(), h.DB, userID, activity.ID, data, h.Events, true, true, nil, true)

	if h.Events != nil {
		h.Events.PublishAnalyticsInvalidate(userID)
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]string{
		"id":     activity.ID,
		"status": "created",
	})
}

// parseFlexibleTime parses RFC3339 or "2006-01-02T15:04" format.
func parseFlexibleTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02T15:04", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("expected RFC3339 or 2006-01-02T15:04")
}
