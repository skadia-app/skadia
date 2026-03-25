package main

import (
	"context"
	"encoding/hex"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/skadia-app/skadia/services/api/internal/auth"
	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/config"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/handler"
	analyticshandler "github.com/skadia-app/skadia/services/api/internal/handler/analytics"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/migrate"
	mlpkg "github.com/skadia-app/skadia/services/api/internal/ml"
	"github.com/skadia-app/skadia/services/api/internal/osm/coverage"
	"github.com/skadia-app/skadia/services/api/internal/pmc"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
	"github.com/skadia-app/skadia/services/api/internal/storage"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// pprof server on :6060 (default http mux)
	go func() {
		slog.Info("pprof server listening", "addr", "127.0.0.1:6060")
		if err := http.ListenAndServe("127.0.0.1:6060", nil); err != nil {
			slog.Error("pprof server failed", "error", err)
		}
	}()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	auth.ValidateConfig(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	// Clean up stale processing entries from previous runs (crash recovery).
	if cleaned, cleanErr := db.CleanupStaleProcessing(ctx, 10*time.Minute); cleanErr == nil && cleaned > 0 {
		slog.Info("import_log: cleaned stale processing entries", "count", cleaned)
	}

	// Periodic cleanup of stale import_log 'processing' entries (every 10 min).
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if n, err := db.CleanupStaleProcessing(ctx, 10*time.Minute); err != nil {
					slog.Warn("import_log: periodic cleanup stale processing", "error", err)
				} else if n > 0 {
					slog.Info("import_log: periodic cleanup stale processing", "count", n)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

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
	if cfg.KomootEncryptionKey == "" {
		slog.Warn("KOMOOT_ENCRYPTION_KEY is not set — Komoot import will be unavailable until configured")
	}

	// Build the ImportService which holds the shared post-processing pipeline.
	// The ShadowWriter and SegmentMatcher callbacks are wired after the Handler
	// is created (they reference handler methods), so we set them below.
	importSvc := importer.NewService(db, s3Client, pub, nil, nil)

	h := &handler.Handler{
		DB:            db,
		Storage:       s3Client,
		Events:        pub,
		ShadowFS: shadowFSCfg,
		ImportService: importSvc,
		Config: &handler.HandlerConfig{
			LiteLLMConfigPath:   cfg.LiteLLMConfigPath,
			LiteLLMInternalURL:  cfg.LiteLLMInternalURL,
			LLMEncryptionKey:    cfg.LLMEncryptionKey,
			MLServiceURL:        cfg.MLServiceURL,
			MLCallbackSecret:    mlCallbackSecret,
			BRouterURL:          cfg.BRouterURL,
			KomootEncryptionKey: cfg.KomootEncryptionKey,
			RoutingGraphPath:    cfg.RoutingGraphPath,
			RoutingTimeout:      parseDuration(os.Getenv("ROUTING_TIMEOUT"), 30*time.Second),
			MaxUploadSize:       cfg.MaxUploadSize,
		},
	}

	// Routing graph manager — initialize tiled routing (UnifiedRouter).
	graphMgr := &routing.GraphManager{}
	h.GraphManager = graphMgr
	tileDir := filepath.Join(filepath.Dir(cfg.RoutingGraphPath), "tiles")
	tileCacheBytes := int64(4_000_000_000) // 4 GB default
	if v := os.Getenv("TILE_CACHE_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			tileCacheBytes = n
		}
	}
	if unified, err := graphMgr.InitTiledReader(tileDir, tileCacheBytes); err != nil {
		slog.Warn("unified tiled reader init failed", "error", err)
	} else if unified {
		slog.Info("routing: unified router active")
	}

	// Wire GraphReader into the import service so surface enrichment can use
	// the tiled graph during initial import (graph path preferred over SQL).
	if gr := graphMgr.Reader(); gr != nil {
		importSvc.GraphReader = gr
	}

	// Coverage cache — on-demand tile loading for coverage query endpoint.
	h.CoverageCache = coverage.NewTileGridCache(db.Pool, coverage.DefaultNoiseSettings())
	// Wire Shadow FS callback: delegates to the handler method which needs
	// both h.DB and h.ShadowFS — set after h is constructed.
	if shadowFSCfg != nil {
		importSvc.ShadowWriter = h.WriteShadowForActivity
	}

	// Wire SegmentMatcher callback: runs segment matching + lap detection in
	// the background goroutine, reusing the existing handler-package functions.
	importSvc.SegmentMatcher = func(activityID, userID, sportType, subType string, gearID *string, activityDate time.Time) {
		bgCtx := context.Background()
		handler.MatchActivitySegments(bgCtx, db, activityID, userID, sportType, subType, gearID, activityDate)
		if db.IsLapDetectionEnabled(bgCtx, userID) {
			gpsStream, err := db.GetActivityGPSStream(bgCtx, activityID)
			if err == nil && len(gpsStream) >= 20 {
				if laps := handler.DetectLaps(activityID, gpsStream); len(laps) > 0 {
					if err := db.InsertDetectedLaps(bgCtx, activityID, laps); err != nil {
						slog.Error("import: insert detected laps failed", "id", activityID, "error", err)
					} else {
						slog.Info("import: detected laps", "id", activityID, "count", len(laps))
					}
				}
			}
		}
	}
	mlClient := mlpkg.NewClient(cfg.MLServiceURL)
	analyticsHandler := analyticshandler.NewHandlerWithML(db, pub, mlClient)

	// Auth handler (Go-native auth)
	authMailer := auth.NewMailer(cfg)
	authRL := middleware.NewRateLimiter(ctx)
	authHandler := auth.NewHandler(db, cfg, authMailer, authRL, ctx)

	// Token cleanup (daily, graceful shutdown)
	shutdown.Go("token-cleanup", func(ctx context.Context) {
		// Run once immediately at startup
		if err := db.CleanupExpiredTokens(ctx); err != nil {
			slog.Error("initial token cleanup failed", "error", err)
		} else {
			slog.Info("token cleanup completed at startup")
		}

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := db.CleanupExpiredTokens(ctx); err != nil {
					slog.Error("token cleanup failed", "error", err)
				} else {
					slog.Info("periodic token cleanup completed")
				}
				if err := db.PurgeDeletedUsers(ctx, cfg.ShadowFSBasePath); err != nil {
					slog.Error("GDPR purge failed", "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	})

	// Router
	r := chi.NewRouter()
	r.Use(middleware.TrustedProxyRealIP(cfg.TrustedProxyCIDRs))
	r.Use(middleware.SecurityHeaders(strings.HasPrefix(cfg.AppURL, "https://")))

	// CSRF protection via Fetch Metadata (Go 1.26 net/http)
	csrf := http.NewCrossOriginProtection()
	r.Use(func(next http.Handler) http.Handler { return csrf.Handler(next) })

	r.Use(middleware.Logger)
	r.Use(chimiddleware.Recoverer)

	// Public routes (no authentication required)
	r.Get("/health", h.Health)
	r.Get("/api/v1/sports/taxonomy", h.GetSportsTaxonomy)
	r.Route("/api/v1/auth", func(r chi.Router) {
		// Go-native auth endpoints
		r.With(middleware.RateLimitGeneric(authRL)).Get("/config", authHandler.AuthConfig)

		r.With(middleware.RateLimitRegister(authRL)).
			Post("/register", authHandler.Register)
		r.With(middleware.RateLimitGeneric(authRL)).
			Post("/verify-email", authHandler.VerifyEmail)
		r.With(middleware.RateLimitForgotPW(authRL)).
			Post("/forgot-password", authHandler.ForgotPassword)
		r.With(middleware.RateLimitGeneric(authRL)).
			Post("/reset-password", authHandler.ResetPassword)
		r.With(middleware.RateLimitGeneric(authRL)).
			Post("/confirm-email-change", authHandler.ConfirmEmailChange)
		r.With(middleware.RateLimitGeneric(authRL)).
			Post("/cancel-deletion", authHandler.CancelDeletion)
		r.With(middleware.RateLimitGeneric(authRL)).
			Post("/exchange-code", authHandler.ExchangeCode)
		r.With(middleware.RateLimitLoginByIP(authRL)).
			Post("/login", authHandler.Login) // POST coexists with old GET

		r.With(middleware.RateLimitGeneric(authRL)).
			Get("/social/{provider}", authHandler.SocialLogin)
		r.With(middleware.RateLimitGeneric(authRL)).
			Get("/social/{provider}/callback", authHandler.SocialCallback)

		// Web session management (cookie-based)
		r.With(middleware.RateLimitGeneric(authRL)).Post("/refresh", authHandler.Refresh)
		r.With(middleware.RateLimitGeneric(authRL)).Post("/logout", authHandler.Logout)

		// Mobile (Go-native)
		r.With(middleware.RateLimitLoginByIP(authRL)).
			Post("/mobile/login", authHandler.MobileLogin)
		r.With(middleware.RateLimitGeneric(authRL)).
			Post("/mobile/refresh", authHandler.MobileRefresh)
		r.With(middleware.RateLimitGeneric(authRL)).
			Post("/mobile/exchange", authHandler.MobileExchange)
		r.With(middleware.RateLimitGeneric(authRL)).Post("/mobile/logout", authHandler.MobileLogout)
	})

	// Internal routes (no JWT auth — service-to-service only, network-level access control)
	r.Get("/internal/coverage/query", h.QueryCoverage)

	// Protected routes
	r.Group(func(r chi.Router) {
		if cfg.DevAuth {
			slog.Warn("DEV_AUTH enabled – using fixed dev user ID, NOT for production")
			r.Use(middleware.DevAuthMiddleware)
		} else {
			var jwtSecret []byte
			if cfg.JWTSecret != "" {
				jwtSecret, _ = hex.DecodeString(cfg.JWTSecret) // already validated by auth.ValidateConfig
			}
			jwtAuth := middleware.NewJWTAuth(jwtSecret)
			r.Use(jwtAuth.Middleware)
			slog.Info("JWT auth enabled", "hs256", cfg.JWTSecret != "")
		}

		r.Use(middleware.RequirePasswordChanged())

		r.Route("/api/v1", func(r chi.Router) {
			// ── Admin routes (RequireAdmin) ──
			r.Route("/admin", func(r chi.Router) {
				r.Use(middleware.RequireAdmin())

				// Backfill / batch operations
				r.Post("/backfill-heatmap", h.BackfillHeatmap)
				r.Post("/backfill-heatmap-points", h.BackfillHeatmapPoints)
				r.Post("/backfill-weather", h.BackfillWeather)
				r.Post("/backfill-enrichment", h.BackfillEnrichment)
				r.Post("/backfill-sunrise-sunset", h.BackfillSunriseSunset)
				r.Post("/phash-backfill", h.StartPhashBackfill)
				r.Post("/detect-max-hr", h.DetectMaxHR)
				r.Post("/activities/recompute-all", h.RecomputeAllActivities)
				r.Get("/activities/recompute-status", h.RecomputeStatus)

				// OSM
				r.Get("/osm-status", h.GetOSMStatus)
				r.Get("/osm-regions/status", h.GetOSMRegionStatus)
				r.Get("/osm-disk-usage", h.GetOSMDiskUsage)
				r.Delete("/osm-regions/{regionID}", h.DeleteOSMRegion)
				r.Get("/osm-regions", h.ListOSMRegions)
				r.Post("/osm-import/start", h.StartOSMImport)
				r.Post("/osm-import/cancel/{jobID}", h.CancelOSMImport)
				r.Post("/osm-import/retry-enrichment", h.RetryOSMEnrichment)
				r.Get("/osm-schedule", h.GetOSMSchedule)
				r.Post("/osm-schedule", h.SetOSMSchedule)

				// DEM
				r.Get("/dem-status", h.GetDEMStatus)

				// Routing infrastructure
				r.Post("/routing/rebuild-tiles", h.RebuildRoutingTiles)
				r.Post("/routing/rebuild-landmarks", h.RebuildLandmarks)
				r.Post("/routing/build-brouter-segments", h.BuildBRouterSegments)
				r.Post("/routing/download-brouter-segments", h.DownloadBRouterSegments)
				r.Put("/routing/cache-budget", h.SetCacheBudget)

				// LLM config (system-level)
				r.Get("/llm-config", h.GetLLMConfig)
				r.Put("/llm-config/{role}", h.PutLLMConfigRole)
				r.Get("/llm-config/models", h.GetLLMModels)
				r.Post("/llm-config/test", h.TestLLMConnection)

				// ML System Settings (nightly job toggles etc.)
				r.Get("/settings/ml", h.GetMLSystemSettings)
				r.Put("/settings/ml/{key}", h.PutMLSystemSetting)

				// Photo batch operations
				r.Post("/photos/recompute-previews", h.RecomputePhotoPreviews)
				r.Post("/photos/recompute-thumbnails", h.RecomputePhotoThumbnails)
				r.Post("/photos/recompute-embeddings", h.RecomputePhotoEmbeddings)

				// User management
				r.Get("/users", authHandler.AdminListUsers)
				r.Post("/users", authHandler.AdminCreateUser)
				r.Post("/users/{id}/activate", authHandler.AdminActivateUser)
				r.Post("/users/{id}/deactivate", authHandler.AdminDeactivateUser)
				r.Post("/users/{id}/reset-password", authHandler.AdminResetPassword)
				r.Get("/users/{id}/sessions", authHandler.AdminListUserSessions)
				r.Post("/users/{id}/revoke-sessions", authHandler.AdminRevokeUserSessions)
				r.Get("/audit-log", authHandler.AdminAuditLog)
				r.Post("/users/{id}/restore", authHandler.AdminRestoreUser)
			})

			// ── Regular authenticated routes ──
			r.With(middleware.RateLimitChangePassword(authRL)).
				Post("/auth/change-password", authHandler.ChangePassword)
			r.With(middleware.RateLimitGeneric(authRL)).
				Get("/auth/sessions", authHandler.ListMySessions)
			r.With(middleware.RateLimitGeneric(authRL)).
				Post("/auth/sessions/{familyId}/revoke", authHandler.RevokeSession)
			r.Post("/auth/link/{provider}", authHandler.LinkSocialAccount)
			r.Delete("/auth/link/{provider}", authHandler.UnlinkSocialAccount)
			r.With(middleware.RateLimitPerUser(authRL, "resend:", 3.0/3600, 3)).
				Post("/auth/resend-verification", authHandler.ResendVerification)
			r.With(middleware.RateLimitGeneric(authRL)).
				With(middleware.RateLimitPerUser(authRL, "change-email:", 3.0/3600, 3)).
				Post("/auth/change-email", authHandler.ChangeEmail)
			r.With(middleware.RateLimitGeneric(authRL)).
				Delete("/auth/account", authHandler.DeleteAccount)
			r.With(middleware.RateLimitPerUser(authRL, "export:", 2.0/3600, 2)).
				Get("/auth/data-export", authHandler.DataExport)

			r.Post("/activities/upload", h.Upload)
			r.Post("/activities/manual", h.CreateManualActivity)
			r.Post("/activities/group", h.GroupActivities)
			r.Post("/activities/re-enrich", h.ReEnrichActivities)
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
			r.Patch("/activities/{id}/resolve-sport-conflict", h.ResolveSportConflict)
			r.Post("/activities/{id}/correct-elevation", h.CorrectActivityElevation)
			r.Post("/activities/{id}/reset-elevation", h.ResetActivityElevation)
			r.Post("/activities/{id}/correct-distance", h.CorrectActivityDistance)
			r.Post("/activities/{id}/reset-distance", h.ResetActivityDistance)
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
			r.Get("/activities/{id}/events", h.HandleGetActivityEvents)
			r.Get("/activities/{id}/sets", h.HandleGetActivitySets)
			r.Get("/activities/{id}/jumps", h.HandleGetActivityJumps)
			r.Get("/activities/{id}/hr-data", h.HandleGetActivityHrData)
			r.Get("/activities/{id}/segment-laps", h.HandleGetActivitySegmentLaps)
			r.Get("/workouts", h.HandleListWorkouts)
			r.Get("/workouts/{id}", h.HandleGetWorkout)
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
			r.Get("/imports", h.GetImports)

			r.Get("/activities/{id}/geo-tags", h.GetGeoTags)
			r.Get("/activities/{id}/enrichment-status", h.GetEnrichmentStatus)

			r.Get("/tags", h.ListTags)
			r.Post("/tags", h.CreateTag)
			r.Patch("/tags/{id}", h.UpdateTag)
			r.Delete("/tags/{id}", h.DeleteTag)
			r.Post("/tags/seed-defaults", h.SeedDefaultTags)
			r.Delete("/tags/orphan", h.DeleteOrphanTag)
			r.Get("/tags/{id}/activities", h.GetTagActivities)
			r.Post("/tags/{id}/assign-activities", h.AssignActivitiesToTag)
			r.Delete("/tags/{id}/activities", h.UnassignActivitiesFromTag)
			r.Post("/tags/{id}/merge", h.MergeTag)
			r.Post("/tags/{id}/sync-location", h.SyncTagLocation)
			r.Post("/tags/{id}/cover-image", h.UploadTagCoverImage)
			r.Delete("/tags/{id}/cover-image", h.ClearTagCoverImage)

			r.Get("/me", h.GetMe)
			r.Patch("/me", h.UpdateMe)

			r.Get("/settings/profile", h.GetProfile)
			r.Patch("/settings/profile", h.PatchProfile)

			r.Get("/settings/heatmap", h.GetHeatmapSettings)
			r.Put("/settings/heatmap", h.UpdateHeatmapSettings)
			r.Get("/settings/enrichment", h.GetEnrichmentSettings)
			r.Put("/settings/enrichment", h.UpdateEnrichmentSettings)
			r.Get("/settings/import", h.GetImportSettings)
			r.Put("/settings/import", h.UpdateImportSettings)
			r.Get("/settings/weather", h.GetWeatherSettings)
			r.Patch("/settings/weather", h.UpdateWeatherSettings)

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
			r.Get("/map/photos", h.ListMapPhotos)
			r.Get("/photos", h.ListPhotoFeed)
			r.Get("/photos/search", h.SearchPhotos)
			r.Get("/media/{bucket}/*", h.ServeMedia)

			// Geo
			r.Get("/geo/visited-pois", h.GetVisitedPOIs)
			r.Get("/geo/search", h.SearchAddress)

			// DEM Elevation
			r.Post("/elevation/query", h.QueryElevation)
			r.Post("/elevation/profile", h.ElevationProfile)
			r.Post("/dem/import", h.ImportDEM)

			// Entdeckungskarte
			r.Get("/terra-incognita/coverage", h.GetTerraIncognitaCoverage)

			// Summits Collection
			r.Get("/summits/collection", h.GetSummitCollection)
			r.Get("/summits/by-name/activities", h.GetSummitActivitiesByName)
			r.Get("/summits/{osm_id}/activities", h.GetSummitActivities)

			// Isochrone
			r.Post("/isochrone", h.Isochrone)

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
			r.Get("/import-queue/{id}/thumb", h.GetImportQueueItemThumb)

			// Tag Zones
			r.Get("/tag-zones/{tagId}", h.GetTagZone)
			r.Put("/tag-zones/{tagId}", h.UpsertTagZone)
			r.Delete("/tag-zones/{tagId}", h.DeleteTagZone)

			// ML User Settings (per-user preferences)
			r.Get("/settings/ml/user", h.GetMLUserSettings)
			r.Put("/settings/ml/user/{key}", h.PutMLUserSetting)

			// LLM User Settings
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

			// Komoot Import
			r.Post("/import/komoot/login", h.KomootLogin)
			r.Post("/import/komoot/scan", h.KomootScan)
			r.Post("/import/komoot/start", h.KomootStartImport)
			r.Get("/import/komoot/jobs/{id}", h.GetKomootImportJob)
			r.Get("/import/komoot/credentials", h.KomootCredentialsStatus)
			r.Delete("/import/komoot/credentials", h.KomootDeleteCredentials)

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

			// Hardware (Devices & Sensors)
			r.Get("/hardware", h.ListHardware)
			r.Post("/hardware/merge", h.MergeHardware)
			r.Get("/hardware/{id}", h.GetHardware)
			r.Patch("/hardware/{id}", h.PatchHardware)
			r.Delete("/hardware/{id}", h.DeleteHardware)
			r.Get("/hardware/{id}/activities", h.ListHardwareActivities)

			// Mobile: Health Data Ingest
			r.Post("/import/health-data", h.IngestHealthData)

			// Notifications
			r.Get("/notifications", h.ListNotifications)
			r.Get("/notifications/unread-count", h.GetUnreadCount)
			r.Post("/notifications/read-all", h.MarkAllNotificationsRead)
			r.Post("/notifications/{id}/read", h.MarkNotificationRead)
			r.Delete("/notifications", h.DeleteAllNotifications)
			r.Delete("/notifications/{id}", h.DeleteNotification)

			// Routing (BRouter proxy + auto-route suggestion)
			r.Post("/routing/route", h.RouteViabrouter)
			r.Post("/routes/suggest", h.SuggestRoutes)
			r.Get("/routing/status", h.RoutingGraphStatus)
			r.Post("/routing/route-internal", h.RouteCalculate)

			// Planned Routes (CRUD)
			r.Get("/routes", h.ListRoutes)
			r.Post("/routes", h.CreateRoute)
			r.Get("/routes/{id}", h.GetRoute)
			r.Patch("/routes/{id}", h.UpdateRoute)
			r.Delete("/routes/{id}", h.DeleteRoute)
			r.Post("/routes/{id}/duplicate", h.DuplicateRoute)
			r.Get("/routes/{id}/gpx", h.ExportRouteGPX)
			r.Put("/routes/{id}/tags", h.SetRouteTags)
			r.Post("/routes/{id}/tags/{tagId}", h.AddRouteTag)
			r.Delete("/routes/{id}/tags/{tagId}", h.RemoveRouteTag)
			r.Post("/routes/{id}/photos", h.UploadRoutePhoto)
			r.Delete("/routes/{id}/photos/{photoId}", h.DeleteRoutePhoto)
			r.Put("/routes/{id}/photos/reorder", h.ReorderRoutePhotos)
			r.Post("/routes/from-activity/{activityId}", h.RouteFromActivity)
			r.Get("/routes/from-activity/{activityId}/waypoints", h.RouteFromActivityWaypoints)

			// POI search
			r.Get("/search/pois", h.SearchPOIs)
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

	// Start ML Service health probe (updates cached availability every 30s).
	capabilities.StartMLProbe(ctx, cfg.MLServiceURL)

	// Start OSM update scheduler (checks every hour whether any region update is due).
	h.StartOSMUpdateScheduler(ctx)

	// Start weekly cleanup of hardware devices no longer linked to any activity.
	h.StartDeviceOrphanCleanupScheduler(ctx)

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

	// Start per-user hotfolder watchers if the hotfolder base path is configured.
	// Each user gets an isolated subdirectory: {HotfolderPath}/{userID}/.
	// HOTFOLDER_USER_ID is no longer used; each user controls their own watcher
	// via the hotfolder_enabled toggle in their settings.
	if shadowFSCfg != nil && shadowFSCfg.HotfolderPath != "" {
		if cfg.HotfolderUserID != "" {
			slog.Warn("HOTFOLDER_USER_ID env var is deprecated and ignored; " +
				"users control their hotfolder via the settings toggle")
		}
		serverCtx, serverCancel := context.WithCancel(context.Background())
		defer serverCancel()
		wm := shadowfs.NewWatcherManager(shadowFSCfg.HotfolderPath, func(userID, filePath string) error {
			return h.UploadFromFile(serverCtx, userID, filePath)
		})
		h.WatcherManager = wm
		defer wm.StopAll()

		// Start watchers for all users that have hotfolder enabled in DB.
		enabledUsers, err := db.GetUsersWithHotfolderEnabled(ctx)
		if err != nil {
			slog.Warn("hotfolder: failed to load enabled users", "error", err)
		} else {
			for _, userID := range enabledUsers {
				if err := wm.EnableUser(userID); err != nil {
					slog.Warn("hotfolder: failed to start watcher for user",
						"user_id", userID, "error", err)
				}
			}
			slog.Info("hotfolder watchers started", "users", len(enabledUsers),
				"base_path", shadowFSCfg.HotfolderPath)
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

	// Phase 1: Cancel all background jobs and wait for graceful completion.
	// 20s timeout — docker-compose stop_grace_period is 30s, leaves 10s buffer.
	if !shutdown.Shutdown(20 * time.Second) {
		slog.Warn("shutdown: background jobs did not finish, killing stale DB queries")
		// Active DB queries from orphaned goroutines would block locks indefinitely.
		// Kill them so PostgreSQL can release row-locks before SIGKILL arrives.
		db.Pool.Exec(context.Background(), `
			SELECT pg_cancel_backend(pid)
			FROM pg_stat_activity
			WHERE usename = current_user
			  AND pid != pg_backend_pid()
			  AND state = 'active'
			  AND query_start < now() - interval '5 seconds'`)
	}

	// Phase 2: Cancel main context (tickers, probes, scheduler)
	cancel()

	// Phase 3: Stop accepting new HTTP requests, finish in-flight requests.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
	}

	slog.Info("server stopped")
}

func parseDuration(s string, defaultVal time.Duration) time.Duration {
	if s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		slog.Warn("invalid duration, using default", "value", s, "default", defaultVal, "error", err)
		return defaultVal
	}
	return d
}
