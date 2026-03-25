package handler

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/crypto"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/komoot"
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
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	req, ok := httputil.DecodeJSON[KomootLoginRequest](w, r)
	if !ok {
		return
	}
	if req.Email == "" || req.Password == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "email and password are required", r.URL.Path)
		return
	}

	// Validate credentials against Komoot API
	client, err := komoot.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		slog.Warn("komoot login failed", "user_id", userID, "error", err)
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "invalid_credentials", "error": "Anmeldung bei Komoot fehlgeschlagen. Bitte E-Mail und Passwort prüfen."})
		return
	}

	// Validate encryption key is configured
	if h.Config.KomootEncryptionKey == "" {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "Komoot encryption key not configured", r.URL.Path)
		return
	}

	// Encrypt password for storage
	encPassword, err := crypto.Encrypt(req.Password, h.Config.KomootEncryptionKey)
	if err != nil {
		slog.Error("komoot: encrypt password failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to encrypt password", r.URL.Path)
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
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to save credentials", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
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
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	creds, err := h.DB.GetKomootCredentials(r.Context(), userID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load credentials", r.URL.Path)
		return
	}
	if creds == nil {
		httputil.WriteJSON(w, http.StatusOK, KomootScanResult{HasCredentials: false})
		return
	}

	if h.Config.KomootEncryptionKey == "" {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "Komoot encryption key not configured", r.URL.Path)
		return
	}

	password, err := crypto.Decrypt(creds.EncryptedPassword, h.Config.KomootEncryptionKey)
	if err != nil {
		slog.Error("komoot: decrypt password failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to decrypt credentials", r.URL.Path)
		return
	}

	client, err := komoot.Login(r.Context(), creds.Email, password)
	if err != nil {
		slog.Warn("komoot scan: login with stored credentials failed", "user_id", userID, "error", err)
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "invalid_credentials", "error": "Gespeicherte Komoot-Zugangsdaten ungültig. Bitte erneut anmelden."})
		return
	}

	tours, err := client.ListTours(r.Context(), nil)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list tours", r.URL.Path)
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

	httputil.WriteJSON(w, http.StatusOK, KomootScanResult{
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
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	req, ok := httputil.DecodeJSON[KomootStartRequest](w, r)
	if !ok {
		return
	}

	creds, err := h.DB.GetKomootCredentials(r.Context(), userID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load credentials", r.URL.Path)
		return
	}
	if creds == nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "no Komoot credentials stored — login first", r.URL.Path)
		return
	}

	if h.Config.KomootEncryptionKey == "" {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "Komoot encryption key not configured", r.URL.Path)
		return
	}

	password, err := crypto.Decrypt(creds.EncryptedPassword, h.Config.KomootEncryptionKey)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to decrypt credentials", r.URL.Path)
		return
	}

	// Check for already-running import
	alreadyRunning, err := h.DB.HasActiveKomootImportJob(r.Context(), userID)
	if err != nil {
		slog.Error("komoot: check active job failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to check active jobs", r.URL.Path)
		return
	}
	if alreadyRunning {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
		return
	}

	// Create background job
	entityName := "Komoot Import"
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "komoot_import", nil, &entityName, 0)
	if err != nil {
		slog.Error("komoot: create background job failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create import job", r.URL.Path)
		return
	}

	// Run async
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("komoot import: panic", "job_id", jobID, "panic", rec)
				_ = h.DB.FailBackgroundJob(context.Background(), jobID)
				// Ensure credentials are deleted even on panic
				if req.DeleteCredentialsAfter {
					if delErr := h.DB.DeleteKomootCredentials(context.Background(), userID); delErr != nil {
						slog.Error("komoot import: delete credentials on panic failed", "job_id", jobID, "error", delErr)
					}
				}
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
			DB:            h.DB,
			Storage:       h.Storage,
			FileStore:     h.FileStore,
			ImportService: h.ImportService,
			UserID:        userID,
			BatchID:       jobID,
			TourIDs:       req.TourIDs,
			Options:       req.Options,
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
				maps.Copy(tourActivityMap, actResult.TourActivityMap)
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
		_ = h.DB.CompleteBackgroundJob(bgCtx, jobID, failed, "")

		slog.Info("komoot import completed",
			"job_id", jobID,
			"errors", len(importErrors),
			"tour_activity_map_size", len(tourActivityMap),
		)
	}()

	httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// GetKomootImportJob returns the status of a Komoot import job.
//
// GET /api/v1/import/komoot/jobs/{id}
func (h *Handler) GetKomootImportJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	jobID := chi.URLParam(r, "id")

	job, err := h.DB.GetKomootImportJob(r.Context(), userID, jobID)
	if httputil.HandleDBErr(w, r, err, "job not found") {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, job)
}

// KomootCredentialsStatus checks if stored Komoot credentials exist for the user.
//
// GET /api/v1/import/komoot/credentials
func (h *Handler) KomootCredentialsStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	creds, err := h.DB.GetKomootCredentials(r.Context(), userID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to check credentials", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"has_credentials": creds != nil,
		"email": func() string {
			if creds != nil {
				return creds.Email
			}
			return ""
		}(),
	})
}

// KomootDeleteCredentials removes stored Komoot credentials.
//
// DELETE /api/v1/import/komoot/credentials
func (h *Handler) KomootDeleteCredentials(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	if err := h.DB.DeleteKomootCredentials(r.Context(), userID); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete credentials", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
