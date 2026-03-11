package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/komoot"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// --- Request/Response types ---

type KomootLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type KomootScanResult struct {
	TourCount      int  `json:"tour_count"`
	RecordedCount  int  `json:"recorded_count"`
	PlannedCount   int  `json:"planned_count"`
	NewCount       int  `json:"new_count"` // tours not yet imported
	HasCredentials bool `json:"has_credentials"`
}

type KomootStartRequest struct {
	ImportRecorded bool                   `json:"import_recorded"`
	ImportPlanned  bool                   `json:"import_planned"`
	ImportPhotos   bool                   `json:"import_photos"`
	TourIDs        []int64                `json:"tour_ids,omitempty"` // empty = all new
	Options        importer.ImportOptions `json:"options"`
}

// --- Handlers ---

// KomootLogin validates Komoot credentials against the API and saves them encrypted.
//
// POST /api/v1/import/komoot/login
func (h *Handler) KomootLogin(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	var req KomootLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "email and password are required", r.URL.Path)
		return
	}

	// Validate credentials against Komoot API
	client, err := komoot.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		slog.Warn("komoot login failed", "user_id", userID, "error", err)
		writeJSON(w, http.StatusOK, map[string]string{"status": "invalid_credentials", "error": err.Error()})
		return
	}

	// Encrypt password for storage
	encPassword, err := encryptAPIKey(req.Password, h.Config.KomootEncryptionKey)
	if err != nil {
		slog.Error("komoot: encrypt password failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to encrypt password", r.URL.Path)
		return
	}

	// Quick scan to get tour counts
	tours, err := client.ListTours(r.Context(), nil)
	if err != nil {
		slog.Warn("komoot: list tours after login failed", "error", err)
		tours = nil
	}

	var recordedCount, plannedCount int
	for _, t := range tours {
		switch t.Type {
		case "tour_recorded":
			recordedCount++
		case "tour_planned":
			plannedCount++
		}
	}
	newCount := len(tours)

	// Save credentials
	if err := h.DB.SaveKomootCredentials(r.Context(), userID, database.KomootCredentials{
		Email:             req.Email,
		EncryptedPassword: encPassword,
		KomootUserID:      client.UserID(),
		SyncEnabled:       true,
		ImportPlanned:     true,
		ImportRecorded:    true,
		ImportPhotos:      true,
	}); err != nil {
		slog.Error("komoot: save credentials failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to save credentials", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"komoot_user_id": client.UserID(),
		"scan": KomootScanResult{
			TourCount:      len(tours),
			RecordedCount:  recordedCount,
			PlannedCount:   plannedCount,
			NewCount:       newCount,
			HasCredentials: true,
		},
	})
}

// KomootScan re-scans Komoot tours using stored credentials.
//
// POST /api/v1/import/komoot/scan
func (h *Handler) KomootScan(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	creds, err := h.DB.GetKomootCredentials(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load credentials", r.URL.Path)
		return
	}
	if creds == nil {
		writeJSON(w, http.StatusOK, KomootScanResult{HasCredentials: false})
		return
	}

	password, err := decryptAPIKey(creds.EncryptedPassword, h.Config.KomootEncryptionKey)
	if err != nil {
		slog.Error("komoot: decrypt password failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to decrypt credentials", r.URL.Path)
		return
	}

	client, err := komoot.Login(r.Context(), creds.Email, password)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "invalid_credentials", "error": err.Error()})
		return
	}

	tours, err := client.ListTours(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list tours", r.URL.Path)
		return
	}

	var recordedCount, plannedCount int
	for _, t := range tours {
		switch t.Type {
		case "tour_recorded":
			recordedCount++
		case "tour_planned":
			plannedCount++
		}
	}

	writeJSON(w, http.StatusOK, KomootScanResult{
		TourCount:      len(tours),
		RecordedCount:  recordedCount,
		PlannedCount:   plannedCount,
		NewCount:       recordedCount + plannedCount,
		HasCredentials: true,
	})
}

// KomootStartImport starts an async Komoot import job.
//
// POST /api/v1/import/komoot/start
func (h *Handler) KomootStartImport(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	var req KomootStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	creds, err := h.DB.GetKomootCredentials(r.Context(), userID)
	if err != nil || creds == nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "no Komoot credentials stored — login first", r.URL.Path)
		return
	}

	password, err := decryptAPIKey(creds.EncryptedPassword, h.Config.KomootEncryptionKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to decrypt credentials", r.URL.Path)
		return
	}

	// Create background job
	entityName := "Komoot Import"
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "komoot_import", nil, &entityName, 0)
	if err != nil {
		slog.Error("komoot: create background job failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create import job", r.URL.Path)
		return
	}

	// Run async
	go func() {
		bgCtx := context.Background()
		client, err := komoot.Login(bgCtx, creds.Email, password)
		if err != nil {
			slog.Error("komoot import: login failed", "job_id", jobID, "error", err)
			_ = h.DB.FailBackgroundJob(bgCtx, jobID)
			return
		}

		_ = h.DB.UpdateBackgroundJobPhase(bgCtx, jobID, "import")

		params := komoot.KomootImportParams{
			DB:      h.DB,
			Storage: h.Storage,
			UserID:  userID,
			TourIDs: req.TourIDs,
			Options: req.Options,
			OnProgress: func(current, total int, msg string) {
				if total > 0 {
					_ = h.DB.UpdateBackgroundJobProgress(bgCtx, jobID, int64(current))
				}
				slog.Debug("komoot import progress", "job_id", jobID, "current", current, "total", total, "msg", msg)
			},
		}

		var importErrors []string
		tourActivityMap := make(map[int64]string)

		// Import recorded activities
		if req.ImportRecorded {
			actImporter := &komoot.ActivitiesImporter{Client: client}
			actResult, err := actImporter.Import(bgCtx, params)
			if err != nil {
				slog.Error("komoot import: activities failed", "job_id", jobID, "error", err)
				importErrors = append(importErrors, fmt.Sprintf("activities: %s", err))
			}
			if actResult != nil {
				importErrors = append(importErrors, actResult.Errors...)
				for k, v := range actResult.TourActivityMap {
					tourActivityMap[k] = v
				}
			}
		}

		// Update last sync
		_ = h.DB.UpdateKomootLastSync(bgCtx, userID, client.UserID())

		failed := len(importErrors) > 0 && !req.ImportRecorded
		_ = h.DB.CompleteBackgroundJob(bgCtx, jobID, failed)

		slog.Info("komoot import completed",
			"job_id", jobID,
			"errors", len(importErrors),
			"tour_activity_map_size", len(tourActivityMap),
		)
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// GetKomootImportJob returns the status of a Komoot import job.
//
// GET /api/v1/import/komoot/jobs/{id}
func (h *Handler) GetKomootImportJob(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	jobID := chi.URLParam(r, "id")

	job, err := h.DB.GetKomootImportJob(r.Context(), userID, jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Not Found", "job not found", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, job)
}
