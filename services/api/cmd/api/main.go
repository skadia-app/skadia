package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/skadia-app/skadia/services/api/internal/config"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/handler"
	analyticshandler "github.com/skadia-app/skadia/services/api/internal/handler/analytics"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/migrate"
	mlpkg "github.com/skadia-app/skadia/services/api/internal/ml"
	"github.com/skadia-app/skadia/services/api/internal/pmc"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/storage"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// Database
	db, err := database.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Run database migrations
	if err := migrate.Run(ctx, db.Pool); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// NOTE: Stale running jobs are handled later via h.ResumeInterruptedJobs
	// after the Handler is fully initialised (needs DB + Handler for matching).

	// S3 Storage (Garage)
	s3Client, err := storage.NewS3Client(ctx, cfg.GarageEndpoint, cfg.GarageAccessKey, cfg.GarageSecretKey, cfg.GarageBucket)
	if err != nil {
		slog.Error("failed to create S3 client", "error", err)
		os.Exit(1)
	}

	// Ensure media bucket exists
	mediaBucket := os.Getenv("GARAGE_MEDIA_BUCKET")
	if mediaBucket == "" {
		mediaBucket = "skadia-media"
	}
	if err := s3Client.EnsureBucket(ctx, mediaBucket); err != nil {
		slog.Warn("could not ensure media bucket", "error", err)
	}

	// NATS (optional - log warning if unavailable)
	var pub *event.Publisher
	pub, err = event.NewPublisher(cfg.NatsURL)
	if err != nil {
		slog.Warn("NATS unavailable, events will not be published", "error", err)
		pub = nil
	} else {
		defer pub.Close()

		// Start PMC consumer
		pmcService := &pmc.Service{DB: db}
		consumer := event.NewConsumer(pub.JetStream(), pmcService)
		if err := consumer.Start(); err != nil {
			slog.Warn("PMC consumer failed to start", "error", err)
		} else {
			defer consumer.Stop()
		}
	}

	// Auth config (optional – only when OIDC is configured)
	var authCfg *handler.AuthConfig
	if cfg.OIDCClientID != "" && cfg.JWTSecret != "" {
		authCfg = &handler.AuthConfig{
			OIDCBaseURL:      cfg.OIDCBaseURL,
			OIDCExternalURL:  cfg.OIDCExternalURL,
			OIDCClientID:     cfg.OIDCClientID,
			OIDCClientSecret: cfg.OIDCClientSecret,
			OIDCAppSlug:      cfg.OIDCAppSlug,
			AppRedirectURI:        cfg.AppRedirectURI,
			FrontendURL:           cfg.FrontendURL,
			JWTSecret:             []byte(cfg.JWTSecret),
		}
		slog.Info("OIDC OIDC configured",
			"base_url", cfg.OIDCBaseURL,
			"external_url", cfg.OIDCExternalURL,
			"redirect_uri", cfg.AppRedirectURI,
		)
	} else {
		slog.Warn("OIDC OIDC not configured (OIDC_CLIENT_ID or JWT_SECRET missing)")
	}

	// Shadow File System (optional)
	var shadowFSCfg *shadowfs.ShadowFSConfig
	if cfg.ShadowFSBasePath != "" {
		shadowFSCfg = &shadowfs.ShadowFSConfig{
			BasePath:         cfg.ShadowFSBasePath,
			HotfolderPath:    cfg.HotfolderPath,
			HotfolderEnabled: cfg.HotfolderEnabled,
		}
		slog.Info("shadow file system configured", "base_path", cfg.ShadowFSBasePath)
	}

	// Handler
	mlCallbackSecret := os.Getenv("ML_CALLBACK_SECRET")
	if mlCallbackSecret == "" {
		// Log at Error level so it is clearly visible in logs; no os.Exit to allow dev startup.
		slog.Error("SECURITY: ML_CALLBACK_SECRET is not set — the /internal/ml-callback endpoint is unauthenticated and must be secured before production deployment")
	}

	h := &handler.Handler{
		DB:       db,
		Storage:  s3Client,
		Events:   pub,
		Auth:     authCfg,
		ShadowFS: shadowFSCfg,
		Config: &handler.HandlerConfig{
			LiteLLMConfigPath:  cfg.LiteLLMConfigPath,
			LiteLLMInternalURL: cfg.LiteLLMInternalURL,
			LLMEncryptionKey:   cfg.LLMEncryptionKey,
			MLServiceURL:       cfg.MLServiceURL,
			MLCallbackSecret:   mlCallbackSecret,
			BRouterURL:         cfg.BRouterURL,
		},
	}
	mlClient := mlpkg.NewClient(cfg.MLServiceURL)
	analyticsHandler := analyticshandler.NewHandlerWithML(db, pub, mlClient)

	// Router
	r := chi.NewRouter()
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(chimiddleware.Recoverer)

	// Public routes (no authentication required)
	r.Get("/health", h.Health)
	r.Get("/api/v1/sports/taxonomy", h.GetSportsTaxonomy)
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Get("/login", h.AuthLogin)
		r.Get("/callback", h.AuthCallback)
		r.Post("/refresh", h.AuthRefresh)
		r.Post("/logout", h.AuthLogout)
		// Mobile PKCE endpoints (no cookie, tokens in body)
		r.Post("/token", h.ExchangeToken)
		r.Post("/mobile-refresh", h.MobileRefreshToken)
	})

	// Protected routes
	r.Group(func(r chi.Router) {
		if cfg.DevAuth {
			slog.Warn("DEV_AUTH enabled – using fixed dev user ID, NOT for production")
			r.Use(middleware.DevAuthMiddleware)
		} else {
			var jwtSecret []byte
			if cfg.JWTSecret != "" {
				jwtSecret = []byte(cfg.JWTSecret)
			}
			jwtAuth := middleware.NewJWTAuth(cfg.OIDCJWKSURL, jwtSecret)
			r.Use(jwtAuth.Middleware)
			slog.Info("JWT auth enabled", "jwks_url", cfg.OIDCJWKSURL, "hs256", cfg.JWTSecret != "")
		}

		r.Route("/api/v1", func(r chi.Router) {
			r.Post("/activities/upload", h.Upload)
			r.Post("/activities/group", h.GroupActivities)
			r.Post("/activities/recompute-all", h.RecomputeAllActivities)
			r.Get("/activities/recompute-status", h.RecomputeStatus)
			r.Get("/jobs/active", h.ListActiveJobs)
			r.Get("/jobs/stream", h.StreamJobProgress)
			r.Get("/activities/sport-type-stats", h.GetSportTypeStats)
			r.Get("/activities", h.ListActivities)
			r.Post("/activities/export-zip", h.ExportActivitiesZip)
			r.Get("/activities/{id}", h.GetActivity)
			r.Patch("/activities/{id}", h.UpdateActivity)
			r.Delete("/activities/{id}", h.DeleteActivity)
			r.Delete("/activities", h.DeleteActivities)
			r.Post("/activities/{id}/recompute", h.RecomputeActivity)
			r.Get("/activities/{id}/streams", h.GetStreams)
			r.Get("/activities/{id}/sensors", h.GetSensors)
			r.Get("/activities/{id}/laps", h.GetLaps)
			r.Get("/activities/{id}/splits", h.GetSplits)
			r.Patch("/activities/{id}/splits/{splitId}", h.PatchSplit)
			r.Get("/activities/{id}/lengths", h.GetLengths)
			r.Get("/activities/{id}/climbs", h.GetActivityClimbs)
			r.Get("/activities/{id}/climb-pro", h.GetClimbProEvents)
			r.Get("/activities/{id}/intervals", h.GetActivityIntervals)
			r.Get("/activities/{id}/export", h.ExportActivity)
			r.Post("/activities/{id}/fusion", h.SetActivityFusion)
			r.Get("/activities/{id}/fusion-candidates", h.GetFusionCandidates)
			r.Get("/activities/{id}/zone-distribution", h.GetZoneDistribution)
			r.Get("/activities/{id}/children", h.GetChildActivities)
			r.Get("/activities/{id}/combined-track", h.GetCombinedTrack)
			r.Delete("/activities/{id}/group", h.UngroupActivities)
			r.Get("/stats/pmc", h.GetPMC)
			r.Get("/stats/power-curve", h.GetPowerCurve)

			r.Get("/privacy-zones", h.ListPrivacyZones)
			r.Post("/privacy-zones", h.CreatePrivacyZone)
			r.Put("/privacy-zones/{id}", h.UpdatePrivacyZone)
			r.Delete("/privacy-zones/{id}", h.DeletePrivacyZone)

			r.Get("/settings/shadow-fs", h.GetShadowFSSettings)
			r.Put("/settings/shadow-fs", h.UpdateShadowFSSettings)
			r.Post("/shadow-fs/preview-rename", h.PreviewRename)
			r.Post("/shadow-fs/execute-rename", h.ExecuteRename)

			r.Get("/activities/{id}/geo-tags", h.GetGeoTags)
			r.Get("/activities/{id}/enrichment-status", h.GetEnrichmentStatus)

			r.Get("/tags", h.ListTags)
			r.Post("/tags", h.CreateTag)
			r.Patch("/tags/{id}", h.UpdateTag)
			r.Delete("/tags/{id}", h.DeleteTag)
			r.Post("/tags/seed-defaults", h.SeedDefaultTags)
			r.Get("/tags/{id}/activities", h.GetTagActivities)
			r.Post("/tags/{id}/assign-activities", h.AssignActivitiesToTag)
			r.Delete("/tags/{id}/activities", h.UnassignActivitiesFromTag)
			r.Post("/tags/{id}/merge", h.MergeTag)
			r.Post("/tags/{id}/sync-location", h.SyncTagLocation)
			r.Post("/tags/{id}/cover-image", h.UploadTagCoverImage)

			// ML System Settings (nightly job toggles etc.)
			r.Get("/settings/ml", h.GetMLSystemSettings)
			r.Put("/settings/ml/{key}", h.PutMLSystemSetting)

			r.Post("/admin/backfill-heatmap", h.BackfillHeatmap)
			r.Post("/admin/backfill-heatmap-points", h.BackfillHeatmapPoints)
			r.Post("/admin/backfill-weather", h.BackfillWeather)
			r.Post("/admin/backfill-enrichment", h.BackfillEnrichment)
			r.Post("/admin/backfill-sunrise-sunset", h.BackfillSunriseSunset)
			r.Get("/admin/osm-status", h.GetOSMStatus)
			r.Get("/admin/osm-regions", h.ListOSMRegions)
			r.Post("/admin/osm-import/start", h.StartOSMImport)

			r.Get("/me", h.GetMe)

			r.Get("/settings/profile", h.GetProfile)
			r.Patch("/settings/profile", h.PatchProfile)
			r.Post("/admin/detect-max-hr", h.DetectMaxHR)

			r.Get("/settings/heatmap", h.GetHeatmapSettings)
			r.Put("/settings/heatmap", h.UpdateHeatmapSettings)
			r.Get("/settings/enrichment", h.GetEnrichmentSettings)
			r.Put("/settings/enrichment", h.UpdateEnrichmentSettings)
			r.Get("/settings/import", h.GetImportSettings)
			r.Put("/settings/import", h.UpdateImportSettings)

			r.Get("/capabilities", h.GetCapabilities)

			r.Get("/gear", h.ListGear)
			r.Post("/gear", h.CreateGear)
			r.Get("/gear/component-presets", h.ListComponentPresets)
			r.Get("/gear/maintenance-alerts", h.GetMaintenanceAlerts)
			r.Get("/gear/pending-review", h.ListPendingReviewGear)
			r.Get("/gear/duplicate-candidates", h.GetGearDuplicateCandidates)
			r.Patch("/gear/{id}/review", h.PatchGearReview)
			r.Get("/gear/{id}/costs", h.GetGearCosts)
			r.Get("/gear/{id}", h.GetGear)
			r.Put("/gear/{id}", h.UpdateGear)
			r.Delete("/gear/{id}", h.DeleteGear)
			r.Patch("/gear/{id}/status", h.UpdateGearStatus)
			r.Post("/gear/{id}/merge", h.MergeGear)
			r.Post("/gear/{id}/components", h.CreateComponent)
			r.Post("/gear/{id}/documents", h.UploadGearDocument)
			r.Get("/gear/{id}/documents", h.ListGearDocuments)
			r.Delete("/gear/documents/{docId}", h.DeleteGearDocument)
			r.Get("/gear/documents/{docId}/download", h.DownloadGearDocument)
			r.Get("/gear/{id}/setups", h.ListBikeSetups)
			r.Post("/gear/{id}/setups", h.CreateBikeSetup)
			r.Get("/gear/{id}/chain-rotation", h.GetChainRotation)
			r.Post("/gear/{id}/chain-rotation", h.CreateChainRotation)
			r.Post("/gear/{id}/chain-rotation/swap", h.SwapChain)
			r.Put("/gear/chain-rotation/{id}", h.UpdateChainRotation)
			r.Delete("/gear/chain-rotation/{id}", h.DeleteChainRotation)
			r.Put("/gear/setups/{id}", h.UpdateBikeSetup)
			r.Delete("/gear/setups/{id}", h.DeleteBikeSetup)
			r.Post("/gear/setups/{id}/activate", h.ActivateBikeSetup)
			r.Put("/gear/components/{id}", h.UpdateComponent)
			r.Delete("/gear/components/{id}", h.DeleteComponent)
			r.Post("/gear/components/{id}/transfer", h.TransferComponent)
			r.Get("/gear/components/{id}/history", h.GetComponentHistory)
			r.Get("/gear/components/{id}/services", h.ListServiceEvents)
			r.Post("/gear/components/{id}/services", h.CreateServiceEvent)
			r.Delete("/gear/services/{id}", h.DeleteServiceEvent)
			r.Post("/gear/components/{id}/documents", h.UploadComponentDocument)
			r.Get("/gear/components/{id}/documents", h.ListComponentDocuments)

			r.Get("/automation-rules", h.ListRules)
			r.Post("/automation-rules", h.CreateRule)
			r.Put("/automation-rules/{id}", h.UpdateRule)
			r.Delete("/automation-rules/{id}", h.DeleteRule)
			r.Post("/automation-rules/apply", h.ApplyRules)
			r.Post("/automation-rules/migrate-from-tags", h.MigrateTagConditions)

			// Custom Maps
			r.Get("/custom-maps", h.ListCustomMaps)
			r.Post("/custom-maps", h.CreateCustomMap)
			r.Put("/custom-maps/{mapID}", h.UpdateCustomMap)
			r.Delete("/custom-maps/{mapID}", h.DeleteCustomMap)
			r.Get("/custom-maps/{mapID}/markers", h.ListCustomMapMarkers)
			r.Post("/custom-maps/{mapID}/markers", h.CreateCustomMapMarker)
			r.Put("/custom-maps/{mapID}/markers/{markerID}", h.UpdateCustomMapMarker)
			r.Delete("/custom-maps/{mapID}/markers/{markerID}", h.DeleteCustomMapMarker)
			r.Post("/custom-maps/{mapID}/markers/{markerID}/image", h.UploadCustomMapMarkerImage)

			// Activity Photos
			r.Post("/activities/{id}/photos", h.UploadActivityPhoto)
			r.Get("/activities/{id}/photos", h.ListActivityPhotos)
			r.Patch("/activities/{id}/photos/{photoID}", h.PatchActivityPhotoLocation)
			r.Delete("/activities/{id}/photos/{photoID}", h.DeleteActivityPhoto)
			r.Get("/photos/{photoID}/thumb", h.StreamActivityPhotoThumb)
			r.Get("/photos/{photoID}/full", h.StreamActivityPhotoFull)
			r.Get("/photos/{photoID}/preview", h.StreamActivityPhotoPreview)
			r.Post("/photos/recompute-previews", h.RecomputePhotoPreviews)
			r.Get("/map/photos", h.ListMapPhotos)
			r.Get("/media/{bucket}/*", h.ServeMedia)

			// Geo
			r.Get("/geo/visited-pois", h.GetVisitedPOIs)

			// Entdeckungskarte
			r.Get("/terra-incognita/coverage", h.GetTerraIncognitaCoverage)

			// Peaks Collection
			r.Get("/peaks/collection", h.GetPeakCollection)

			// Segments
			r.Get("/segments", h.ListSegments)
			r.Post("/segments", h.CreateSegment)
			r.Get("/segments/{id}", h.GetSegment)
			r.Put("/segments/{id}", h.UpdateSegment)
			r.Delete("/segments/{id}", h.DeleteSegment)
			r.Get("/segments/{id}/leaderboard", h.GetSegmentLeaderboard)
			r.Post("/segments/{id}/recompute", h.RecomputeSegment)
			r.Get("/segments/{id}/favorite", h.GetSegmentFavoriteStatus)
			r.Post("/segments/{id}/favorite", h.AddSegmentFavorite)
			r.Delete("/segments/{id}/favorite", h.RemoveSegmentFavorite)
			r.Get("/segments/{id}/performances", h.GetSegmentPerformances)
			r.Post("/segments/import-lifts", h.ImportLiftsFromOSM)
			r.Get("/activities/{id}/segments", h.GetActivitySegmentEfforts)
			r.Get("/activities/{id}/detected-laps", h.GetActivityDetectedLaps)
			r.Get("/activities/{id}/stops", h.GetActivityStops)
			r.Get("/activities/{id}/surfaces", h.GetActivitySurfaces)
			r.Get("/activities/{id}/lift-candidates", h.GetLiftCandidates)
			r.Post("/activities/{id}/lift-candidates/{cid}/confirm", h.ConfirmLiftCandidate)
			r.Delete("/activities/{id}/lift-candidates/{cid}", h.RejectLiftCandidate)

			// Track Editor
			r.Get("/activities/{id}/edits", h.GetActivityEdits)
			r.Put("/activities/{id}/edits", h.PutActivityEdits)
			r.Delete("/activities/{id}/edits", h.DeleteActivityEdits)
			r.Post("/activities/{id}/edits/detect-hr-spikes", h.DetectHRSpikesHandler)
			r.Post("/activities/{id}/edits/detect-gps-outliers", h.DetectGPSOutliersHandler)
			r.Post("/activities/{id}/edits/apply", h.ApplyActivityEdits)

			// Non-destructive edit operations (Trim / Split / Merge)
			r.Post("/activities/{id}/trim", h.TrimActivity)
			r.Post("/activities/{id}/split", h.SplitActivity)
			r.Post("/activities/{id}/merge", h.MergeActivities)
			r.Post("/activities/{id}/fusion-merge", h.FusionMerge)
			r.Post("/activities/{id}/revert-fusion", h.RevertFusion)

			// Media Upload (multi-file, EXIF auto-assignment)
			r.Post("/upload/media", h.UploadMedia)

			// Import Queue
			r.Get("/import-queue", h.ListImportQueue)
			r.Get("/import-queue/count", h.CountImportQueue)
			r.Patch("/import-queue/{id}", h.PatchImportQueueItem)
			r.Delete("/import-queue/{id}", h.DeleteImportQueueItem)
			r.Post("/import-queue/{id}/assign", h.AssignImportQueueItem)

			// Tag Zones
			r.Get("/tag-zones/{tagId}", h.GetTagZone)
			r.Put("/tag-zones/{tagId}", h.UpsertTagZone)
			r.Delete("/tag-zones/{tagId}", h.DeleteTagZone)

			// ML User Settings (per-user preferences)
			r.Get("/settings/ml/user", h.GetMLUserSettings)
			r.Put("/settings/ml/user/{key}", h.PutMLUserSetting)

			// LLM Settings (Admin + User)
			r.Get("/admin/llm-config", h.GetLLMConfig)
			r.Put("/admin/llm-config/{role}", h.PutLLMConfigRole)
			r.Get("/admin/llm-config/models", h.GetLLMModels)
			r.Post("/admin/llm-config/test", h.TestLLMConnection)
			r.Get("/user/llm-config", h.GetLLMUserConfig)
			r.Patch("/user/llm-config", h.PatchLLMUserConfig)

			// Health data
			r.Route("/health", func(r chi.Router) {
				r.Get("/daily-summary", h.GetHealthDailySummaries)
				r.Get("/sleep", h.GetHealthSleep)
				r.Get("/hrv", h.GetHealthHRV)
				r.Get("/weight", h.GetHealthWeight)
				r.Post("/weight", h.CreateHealthWeightEntry)
				r.Get("/readiness", h.GetHealthReadiness)
				r.Get("/training-load", h.GetHealthTrainingLoad)
				r.Get("/stress", h.GetHealthStress)
				r.Get("/spo2", h.GetHealthSpO2)
				r.Get("/blood-pressure", h.GetHealthBloodPressure)
				r.Get("/fitness-age", h.GetHealthFitnessAge)
				r.Get("/overview", h.GetHealthOverview)
			})

			// Import from external URL (skadia:// deep-link handler)
			r.Post("/import/from-url", h.ImportFromURL)

			// Garmin Import
			r.Post("/import/garmin/detect", h.DetectGarminModules)
			r.Post("/import/garmin/scan", h.ScanGarminExport)
			r.Post("/import/garmin/start", h.StartGarminImport)
			r.Post("/import/garmin/upload-zip", h.UploadGarminZIP)
			r.Post("/import/garmin/upload-zip-scan", h.UploadGarminZIPScan)
			r.Get("/import/garmin/jobs/{id}", h.GetGarminImportJob)

			// Strava Import
			r.Post("/import/strava/detect", h.DetectStravaModules)
			r.Post("/import/strava/scan", h.ScanStravaExport)
			r.Post("/import/strava/start", h.StartStravaImport)
			r.Post("/import/strava/upload-zip", h.UploadStravaZIP)
			r.Post("/import/strava/upload-zip-scan", h.UploadStravaZIPScan)
			r.Get("/import/strava/jobs/{id}", h.GetStravaImportJob)

			// Fusion Queue (Tier-2 GPS-Similarity)
			r.Get("/fusion-queue", h.GetFusionQueue)

			// Chat
			r.Mount("/chat", h.ChatRoutes())

			// Analytics
			r.Mount("/analytics", analyticsHandler.Routes())

			// ML Status
			r.Get("/ml/status", analyticsHandler.GetMLStatus)

			// ML Callback (internal — called by remote ML service)
			r.Post("/internal/ml-callback", h.HandleMLCallback)

			// Mobile: Device Registration (FCM/APNs)
			r.Post("/devices/register", h.RegisterDevice)
			r.Get("/devices", h.ListDevices)
			r.Delete("/devices/{id}", h.DeleteDevice)

			// Mobile: Health Data Ingest
			r.Post("/import/health-data", h.IngestHealthData)

			// Notifications
			r.Get("/notifications", h.ListNotifications)
			r.Get("/notifications/unread-count", h.GetUnreadCount)
			r.Post("/notifications/read-all", h.MarkAllNotificationsRead)
			r.Post("/notifications/{id}/read", h.MarkNotificationRead)
		})
	})

	// Server with graceful shutdown
	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Resume interrupted background jobs from previous server process.
	go h.ResumeInterruptedJobs(context.Background())

	// Clean up orphaned ZIP-scan temp directories from previous server runs.
	// These are created by UploadGarminZIPScan / UploadStravaZIPScan and only
	// removed by the subsequent start-import call. If the user abandons the
	// wizard they linger until the next restart.
	go func() {
		tmpBase := os.TempDir()
		entries, err := os.ReadDir(tmpBase)
		if err != nil {
			return
		}
		threshold := time.Now().Add(-2 * time.Hour)
		for _, e := range entries {
			if !e.IsDir() || !strings.HasPrefix(e.Name(), "skadia-import-") {
				continue
			}
			info, err := e.Info()
			if err != nil || info.ModTime().After(threshold) {
				continue
			}
			path := filepath.Join(tmpBase, e.Name())
			if rmErr := os.RemoveAll(path); rmErr != nil {
				slog.Warn("startup cleanup: failed to remove orphaned temp dir", "path", path, "error", rmErr)
			} else {
				slog.Info("startup cleanup: removed orphaned temp dir", "path", path)
			}
		}
	}()

	// Background: recalculate PMC for all active users on startup.
	// Ensures historical data is populated after service changes or fresh deploys.
	go func() {
		time.Sleep(2 * time.Second)
		userIDs, err := db.ListActiveUserIDs(ctx)
		if err != nil {
			slog.Warn("startup PMC recalc: failed to list users", "error", err)
			return
		}
		pmcSvc := &pmc.Service{DB: db}
		for _, uid := range userIDs {
			if err := pmcSvc.RecalculatePMC(ctx, uid, time.Now()); err != nil {
				slog.Warn("startup PMC recalc failed", "user_id", uid, "error", err)
			}
		}
		slog.Info("startup PMC recalculation complete", "users", len(userIDs))
	}()

	// Start hotfolder watcher if configured
	var hotfolderWatcher *shadowfs.Watcher
	if shadowFSCfg != nil && shadowFSCfg.HotfolderEnabled && shadowFSCfg.HotfolderPath != "" {
		hotfolderWatcher = shadowfs.NewWatcher(shadowFSCfg.HotfolderPath, func(filePath string) error {
			userID := cfg.HotfolderUserID
			if userID == "" {
				slog.Warn("hotfolder: HOTFOLDER_USER_ID not configured, skipping file", "path", filePath)
				return nil
			}
			ctx := context.Background()
			return h.UploadFromFile(ctx, userID, filePath)
		})
		if err := hotfolderWatcher.Start(); err != nil {
			slog.Warn("hotfolder watcher failed to start", "error", err)
		} else {
			defer hotfolderWatcher.Stop()
		}
	}

	// Start server in goroutine
	go func() {
		slog.Info("server starting", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
	}

	slog.Info("server stopped")
}
