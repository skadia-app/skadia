package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/komoot"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// --- Request/Response types ---

type KomootLoginRequest struct {
	Email           string `json:"email"`
	Password        string `json:"password"`
	SaveCredentials bool   `json:"save_credentials"` // false = only use for this session, don't store
}

type KomootScanResult struct {
	TourCount      int  `json:"tour_count"`
	RecordedCount  int  `json:"recorded_count"`
	PlannedCount   int  `json:"planned_count"`
	NewCount       int  `json:"new_count"` // tours not yet imported
	HasCredentials bool `json:"has_credentials"`
}

type KomootStartRequest struct {
	ImportRecorded         bool                   `json:"import_recorded"`
	ImportPlanned          bool                   `json:"import_planned"`
	ImportPhotos           bool                   `json:"import_photos"`
	TourIDs                []int64                `json:"tour_ids,omitempty"` // empty = all new
	Options                importer.ImportOptions `json:"options"`
	DeleteCredentialsAfter bool                   `json:"delete_credentials_after"` // delete stored credentials after import completes
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

	// Validate encryption key is configured
	if h.Config.KomootEncryptionKey == "" {
		writeError(w, http.StatusInternalServerError, "Internal Error", "Komoot encryption key not configured", r.URL.Path)
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

	// Always save credentials (import goroutine needs them to re-login).
	// If user chose not to persist, delete_credentials_after will clean up post-import.
	if err := h.DB.SaveKomootCredentials(r.Context(), userID, database.KomootCredentials{
		Email:             req.Email,
		EncryptedPassword: encPassword,
		KomootUserID:      client.UserID(),
		SyncEnabled:       req.SaveCredentials,
		ImportPlanned:     true,
		ImportRecorded:    true,
		ImportPhotos:      true,
	}); err != nil {
		slog.Error("komoot: save credentials failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to save credentials", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":            "ok",
		"komoot_user_id":    client.UserID(),
		"credentials_saved": req.SaveCredentials,
		"scan": KomootScanResult{
			TourCount:      len(tours),
			RecordedCount:  recordedCount,
			PlannedCount:   plannedCount,
			NewCount:       newCount,
			HasCredentials: req.SaveCredentials,
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

	if h.Config.KomootEncryptionKey == "" {
		writeError(w, http.StatusInternalServerError, "Internal Error", "Komoot encryption key not configured", r.URL.Path)
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

	// Estimate new_count: count tours whose komoot_tour_id is not yet in DB
	// This is a fast EXISTS check, not a full scan
	newCount := 0
	for _, t := range tours {
		if t.Type == "tour_recorded" {
			exists, _ := h.DB.ActivityExistsByKomootTourID(r.Context(), userID, t.ID)
			if !exists {
				newCount++
			}
		}
	}

	writeJSON(w, http.StatusOK, KomootScanResult{
		TourCount:      len(tours),
		RecordedCount:  recordedCount,
		PlannedCount:   plannedCount,
		NewCount:       newCount,
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
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load credentials", r.URL.Path)
		return
	}
	if creds == nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "no Komoot credentials stored — login first", r.URL.Path)
		return
	}

	if h.Config.KomootEncryptionKey == "" {
		writeError(w, http.StatusInternalServerError, "Internal Error", "Komoot encryption key not configured", r.URL.Path)
		return
	}

	password, err := decryptAPIKey(creds.EncryptedPassword, h.Config.KomootEncryptionKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to decrypt credentials", r.URL.Path)
		return
	}

	// Check for already-running import
	alreadyRunning, err := h.DB.HasActiveKomootImportJob(r.Context(), userID)
	if err != nil {
		slog.Error("komoot: check active job failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to check active jobs", r.URL.Path)
		return
	}
	if alreadyRunning {
		writeJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
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
		defer func() {
			if r := recover(); r != nil {
				slog.Error("komoot import: panic", "job_id", jobID, "panic", r)
				_ = h.DB.FailBackgroundJob(context.Background(), jobID)
			}
		}()
		bgCtx := context.Background()
		client, err := komoot.Login(bgCtx, creds.Email, password)
		if err != nil {
			slog.Error("komoot import: login failed", "job_id", jobID, "error", err)
			_ = h.DB.FailBackgroundJob(bgCtx, jobID)
			return
		}

		_ = h.DB.UpdateBackgroundJobPhase(bgCtx, jobID, "import")

		// Get tour count to set progress_total
		allTours, listErr := client.ListTours(bgCtx, nil)
		if listErr == nil {
			_ = h.DB.UpdateBackgroundJobProgress(bgCtx, jobID, 0)
			_, _ = h.DB.Pool.Exec(bgCtx,
				`UPDATE background_jobs SET progress_total = $1 WHERE id = $2`,
				int64(len(allTours)), jobID,
			)
		}

		params := komoot.KomootImportParams{
			DB:      h.DB,
			Storage: h.Storage,
			UserID:  userID,
			TourIDs: req.TourIDs,
			Options: req.Options,
			OnProgress: func(current, total int, msg string) {
				_ = h.DB.UpdateBackgroundJobProgress(bgCtx, jobID, int64(current))
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

		// Import planned routes
		if req.ImportPlanned {
			routeImporter := &komoot.RoutesImporter{Client: client}
			routeResult, err := routeImporter.Import(bgCtx, params)
			if err != nil {
				slog.Error("komoot import: routes failed", "job_id", jobID, "error", err)
				importErrors = append(importErrors, fmt.Sprintf("routes: %s", err))
			}
			if routeResult != nil {
				importErrors = append(importErrors, routeResult.Errors...)
			}
		}

		// Import photos (after activities — needs tourActivityMap)
		if req.ImportPhotos && len(tourActivityMap) > 0 {
			photoImporter := &komoot.PhotosImporter{
				Client:          client,
				TourActivityMap: tourActivityMap,
			}
			photoResult, err := photoImporter.Import(bgCtx, params)
			if err != nil {
				slog.Error("komoot import: photos failed", "job_id", jobID, "error", err)
				importErrors = append(importErrors, fmt.Sprintf("photos: %s", err))
			}
			if photoResult != nil {
				importErrors = append(importErrors, photoResult.Errors...)
			}
		}

		// Update last sync
		_ = h.DB.UpdateKomootLastSync(bgCtx, userID, client.UserID())

		// Delete credentials after import if requested
		if req.DeleteCredentialsAfter {
			if delErr := h.DB.DeleteKomootCredentials(bgCtx, userID); delErr != nil {
				slog.Error("komoot import: delete credentials after import failed", "job_id", jobID, "error", delErr)
			} else {
				slog.Info("komoot import: credentials deleted after import", "job_id", jobID)
			}
		}

		failed := len(importErrors) > 0
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
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Not Found", "job not found", r.URL.Path)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get job", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// KomootDeleteCredentials removes stored Komoot credentials.
//
// DELETE /api/v1/import/komoot/credentials
func (h *Handler) KomootDeleteCredentials(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	if err := h.DB.DeleteKomootCredentials(r.Context(), userID); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete credentials", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
