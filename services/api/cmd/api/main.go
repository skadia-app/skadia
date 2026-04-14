package main

import (
	"context"
	"flag"
	"fmt"
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

	"github.com/alexedwards/argon2id"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/skadia-app/skadia/services/api/internal/auth"
	"github.com/skadia-app/skadia/services/api/internal/bestefforts"
	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/config"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/dataimport"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/garmin"
	"github.com/skadia-app/skadia/services/api/internal/handler"
	analyticshandler "github.com/skadia-app/skadia/services/api/internal/handler/analytics"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/migrate"
	mlpkg "github.com/skadia-app/skadia/services/api/internal/ml"
	"github.com/skadia-app/skadia/services/api/internal/osm"
	"github.com/skadia-app/skadia/services/api/internal/osm/coverage"
	"github.com/skadia-app/skadia/services/api/internal/pmc"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

func main() {
	// Parse LOG_LEVEL before config.Load() so startup logs use the correct level.
	logLevel := slog.LevelInfo
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})))

	// ── migrate-storage subcommand ─────────────────────────────────────────────
	// Usage: api migrate-storage -target=s3 [-dry-run] [-batch-size=100]
	//                                        [-workers=1] [-cleanup-source]
	// Must run BEFORE the HTTP server starts (offline migration).
	if len(os.Args) > 1 && os.Args[1] == "migrate-storage" {
		fs := flag.NewFlagSet("migrate-storage", flag.ExitOnError)
		target := fs.String("target", "", "Target backend: 'local' or 's3' (required)")
		dryRun := fs.Bool("dry-run", false, "List files and check reachability without copying")
		batchSize := fs.Int("batch-size", 100, "Files per batch")
		workers := fs.Int("workers", 1, "Parallel workers (each buffers up to 500 MB for S3)")
		cleanupSource := fs.Bool("cleanup-source", false, "Verify source is empty after migration")
		if err := fs.Parse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-storage: flag parse error: %v\n", err)
			os.Exit(1)
		}
		if *workers > 16 {
			fmt.Fprintf(os.Stderr, "migrate-storage: max 16 workers supported, got %d\n", *workers)
			os.Exit(1)
		}
		if *target == "" {
			fmt.Fprintf(os.Stderr, "migrate-storage: -target is required (use 'local' or 's3')\n")
			fs.Usage()
			os.Exit(1)
		}
		if *target != "local" && *target != "s3" {
			fmt.Fprintf(os.Stderr, "migrate-storage: -target must be 'local' or 's3', got %q\n", *target)
			os.Exit(1)
		}

		// Load config + DB (migrations included).
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "migrate-storage: config load failed: %v\n", err)
			os.Exit(1)
		}
		ctx := context.Background()
		db, err := database.New(ctx, cfg.DatabaseURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "migrate-storage: database connect failed: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()
		if err := migrate.Run(ctx, db.Pool); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-storage: migrations failed: %v\n", err)
			os.Exit(1)
		}
		if err := shadowfs.CanonicalizeParentTypes(ctx, db.Pool); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-storage: canonicalize parent_type failed: %v\n", err)
			os.Exit(1)
		}

		// Build backends.
		var localBE, s3BE shadowfs.StorageBackend
		if cfg.LibraryBasePath != "" {
			localBE = shadowfs.NewLocalBackend(cfg.LibraryBasePath)
		}
		if cfg.S3Endpoint != "" {
			s3BE = shadowfs.NewS3Backend(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey,
				cfg.S3Bucket, cfg.S3Region, cfg.S3ForcePathStyle)
		}

		// Determine source/target backends.
		var srcBE, tgtBE shadowfs.StorageBackend
		switch *target {
		case "s3":
			srcBE, tgtBE = localBE, s3BE
		case "local":
			srcBE, tgtBE = s3BE, localBE
		}
		if srcBE == nil {
			fmt.Fprintf(os.Stderr, "migrate-storage: source backend not configured\n")
			os.Exit(1)
		}
		if tgtBE == nil {
			fmt.Fprintf(os.Stderr, "migrate-storage: target backend not configured\n")
			os.Exit(1)
		}

		store := shadowfs.NewUnifiedShadowFS(db.Pool, localBE, s3BE, *target,
			cfg.LibraryBasePath, shadowfs.NewSchemaRenderer(), cfg.LibraryKeepVersions,
			cfg.MaxUserStorageBytes, cfg.S3Bucket, cfg.S3BucketActivity, cfg.S3BucketMedia)

		if err := shadowfs.RunMigrateStorage(
			ctx, db.Pool,
			srcBE, tgtBE, *target,
			store.BucketFor,
			cfg.LibraryBasePath,
			shadowfs.NewSchemaRenderer(),
			*dryRun, *batchSize, *workers, *cleanupSource,
		); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-storage: FAILED: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// ── repair-registry subcommand ─────────────────────────────────────────────
	// Usage: api repair-registry
	// Fills size_bytes=0 and file_hash IS NULL entries in user_files by reading
	// each file from its registered backend. Rows for missing files are skipped.
	if len(os.Args) > 1 && os.Args[1] == "repair-registry" {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "repair-registry: config load failed: %v\n", err)
			os.Exit(1)
		}
		ctx := context.Background()
		db, err := database.New(ctx, cfg.DatabaseURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "repair-registry: database connect failed: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()
		if err := migrate.Run(ctx, db.Pool); err != nil {
			fmt.Fprintf(os.Stderr, "repair-registry: migrations failed: %v\n", err)
			os.Exit(1)
		}

		var localBE, s3BE shadowfs.StorageBackend
		if cfg.LibraryBasePath != "" {
			localBE = shadowfs.NewLocalBackend(cfg.LibraryBasePath)
		}
		if cfg.S3Endpoint != "" {
			s3BE = shadowfs.NewS3Backend(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey,
				cfg.S3Bucket, cfg.S3Region, cfg.S3ForcePathStyle)
		}

		store := shadowfs.NewUnifiedShadowFS(db.Pool, localBE, s3BE, "s3",
			cfg.LibraryBasePath, shadowfs.NewSchemaRenderer(), cfg.LibraryKeepVersions,
			cfg.MaxUserStorageBytes, cfg.S3Bucket, cfg.S3BucketActivity, cfg.S3BucketMedia)

		if err := shadowfs.RunRepairRegistry(ctx, db.Pool, localBE, s3BE, store.BucketFor); err != nil {
			fmt.Fprintf(os.Stderr, "repair-registry: FAILED: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// ── build-coverage subcommand ─────────────────────────────────────────────
	// Usage: api build-coverage [--layer=noise] [--region=austria] [--tile=47,8] [--workers=4]
	if len(os.Args) > 1 && os.Args[1] == "build-coverage" {
		fs := flag.NewFlagSet("build-coverage", flag.ExitOnError)
		layer := fs.String("layer", "", "Build only this layer")
		tile := fs.String("tile", "", "Build only this tile (lat,lon)")
		region := fs.String("region", "", "Region name (e.g. austria, germany, switzerland)")
		workers := fs.Int("workers", 4, "Parallel workers")
		if err := fs.Parse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "build-coverage: flag parse error: %v\n", err)
			os.Exit(1)
		}

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "build-coverage: config load failed: %v\n", err)
			os.Exit(1)
		}
		ctx := context.Background()
		db, err := database.New(ctx, cfg.DatabaseURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "build-coverage: database connect failed: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()
		if err := migrate.Run(ctx, db.Pool); err != nil {
			fmt.Fprintf(os.Stderr, "build-coverage: migrations failed: %v\n", err)
			os.Exit(1)
		}

		opts := coverage.BuildCoverageOptions{
			BaseDir: filepath.Join(filepath.Dir(cfg.RoutingGraphPath), "coverage"),
			Layer:   *layer,
			Region:  *region,
			Workers: *workers,
		}
		if *region != "" {
			reg, ok := osm.RegionByID(*region)
			if !ok {
				fmt.Fprintf(os.Stderr, "build-coverage: unknown region %q (use region name, e.g. austria, germany, switzerland)\n", *region)
				os.Exit(1)
			}
			opts.RegionBBox = [4]float64{reg.MinLat, reg.MinLon, reg.MaxLat, reg.MaxLon}
		}
		if *tile != "" {
			var lat, lon int
			if _, err := fmt.Sscanf(*tile, "%d,%d", &lat, &lon); err != nil {
				fmt.Fprintf(os.Stderr, "build-coverage: invalid --tile format, expected lat,lon: %v\n", err)
				os.Exit(1)
			}
			opts.TileLat = lat
			opts.TileLon = lon
			opts.SingleTile = true
		}

		if err := coverage.RunBuildCoverage(ctx, db.Pool, opts, func(current, total int64) {
			slog.Info("progress", "current", current, "total", total)
		}); err != nil {
			slog.Error("build-coverage failed", "error", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// ── import subcommand ──────────────────────────────────────────────────────
	// Usage: api import --archive <path> --target-user <uuid>
	// Imports a GDPR export ZIP or directory into an existing user account.
	// The target account must have no existing activities or uploaded files.
	if len(os.Args) > 1 && os.Args[1] == "import" {
		fs := flag.NewFlagSet("import", flag.ExitOnError)
		archivePath := fs.String("archive", "", "Path to export ZIP or directory (required)")
		targetUser := fs.String("target-user", "", "Target user UUID (required)")
		if err := fs.Parse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "import: flag parse error: %v\n", err)
			os.Exit(1)
		}

		if *archivePath == "" || *targetUser == "" {
			fmt.Fprintln(os.Stderr, "Usage: api import --archive <path> --target-user <uuid>")
			os.Exit(1)
		}

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "import: config load failed: %v\n", err)
			os.Exit(1)
		}
		ctx := context.Background()
		db, err := database.New(ctx, cfg.DatabaseURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "import: database connect failed: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()
		if err := migrate.Run(ctx, db.Pool); err != nil {
			fmt.Fprintf(os.Stderr, "import: migrations failed: %v\n", err)
			os.Exit(1)
		}

		var localBE, s3BE shadowfs.StorageBackend
		if cfg.LibraryBasePath != "" {
			localBE = shadowfs.NewLocalBackend(cfg.LibraryBasePath)
		}
		if cfg.S3Endpoint != "" {
			s3BE = shadowfs.NewS3Backend(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey,
				cfg.S3Bucket, cfg.S3Region, cfg.S3ForcePathStyle)
		}

		sfs := shadowfs.NewUnifiedShadowFS(db.Pool, localBE, s3BE, cfg.StorageBackend,
			cfg.LibraryBasePath, shadowfs.NewSchemaRenderer(), cfg.LibraryKeepVersions,
			cfg.MaxUserStorageBytes, cfg.S3Bucket, cfg.S3BucketActivity, cfg.S3BucketMedia)

		source, err := dataimport.OpenSource(*archivePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "import: open source failed: %v\n", err)
			os.Exit(1)
		}
		defer source.Close()

		engine := &dataimport.Engine{
			DB:       db.Pool,
			ShadowFS: sfs,
			Config: dataimport.EngineConfig{
				StorageBackend:  cfg.StorageBackend,
				LibraryBasePath: cfg.LibraryBasePath,
				MaxStorageBytes: cfg.MaxUserStorageBytes,
			},
		}

		vr, err := engine.ValidateAccount(ctx, *targetUser)
		if err != nil {
			fmt.Fprintf(os.Stderr, "import: ValidateAccount failed: %v\n", err)
			os.Exit(1)
		}
		if !vr.CanImport {
			fmt.Fprintf(os.Stderr, "import: %s\n", vr.Warnings[0])
			os.Exit(1)
		}

		result, err := engine.RunImport(ctx, dataimport.ImportInput{
			Source:       source,
			TargetUserID: *targetUser,
			OnProgress: func(ev dataimport.ProgressEvent) {
				fmt.Fprintf(os.Stderr, "\r[Phase %d/%d] %s", ev.Phase, ev.PhasesTotal, ev.Message)
			},
		})
		fmt.Fprintln(os.Stderr) // newline after progress

		if result != nil {
			fmt.Printf("Imported: %d entities, %d files (%d bytes)\n",
				result.EntitiesImported, result.FilesImported, result.BytesImported)
			fmt.Printf("Skipped: %d, Failed: %d\n", result.EntitiesSkipped, result.EntitiesFailed)
			for _, w := range result.Warnings {
				fmt.Fprintf(os.Stderr, "WARNING: %s\n", w)
			}
			for _, e := range result.Errors {
				fmt.Fprintf(os.Stderr, "ERROR [phase %d, table %s, row %d]: %s\n",
					e.Phase, e.Table, e.RowIndex, e.Error)
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Import failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// L69: pprof server gated behind ENABLE_PPROF=true to avoid unintentional exposure.
	if os.Getenv("ENABLE_PPROF") == "true" {
		go func() {
			slog.Info("pprof server listening", "addr", "127.0.0.1:6060")
			if err := http.ListenAndServe("127.0.0.1:6060", nil); err != nil {
				slog.Error("pprof server failed", "error", err)
			}
		}()
	}

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
	if err := shadowfs.CanonicalizeParentTypes(ctx, db.Pool); err != nil {
		slog.Error("failed to canonicalize parent_type values", "error", err)
		os.Exit(1)
	}

	// Ensure TimescaleDB Continuous Aggregates exist (cannot be created inside
	// golang-migrate transactions — see migration 000242 placeholder).
	if err := garmin.EnsureMonitoringDailyCA(ctx, db.Pool); err != nil {
		slog.Error("failed to ensure health_monitoring_daily CA", "error", err)
		os.Exit(1)
	}

	// Clean up expired system_flags
	db.Pool.Exec(ctx, `DELETE FROM system_flags WHERE expires_at < NOW()`)

	// Check maintenance mode
	var maintenanceValue string
	var maintenanceSetAt time.Time
	if err := db.Pool.QueryRow(ctx,
		`SELECT value, set_at FROM system_flags WHERE key = 'maintenance_mode' AND expires_at > NOW()`).Scan(&maintenanceValue, &maintenanceSetAt); err == nil {
		slog.Error("maintenance mode active — server cannot start",
			"reason", maintenanceValue,
			"set_at", maintenanceSetAt,
			"help", "Complete the storage migration or see docs for recovery steps")
		os.Exit(1)
	}

	// Bootstrap admin from env vars (for headless/CI setup).
	if cfg.BootstrapAdminUser != "" {
		if cfg.BootstrapAdminPassword == "" {
			slog.Error("BOOTSTRAP_ADMIN_USER is set but BOOTSTRAP_ADMIN_PASSWORD is empty — refusing to create admin with empty password")
		} else {
			var userCount int
			if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&userCount); err != nil {
				slog.Error("bootstrap admin: count users failed", "error", err)
			} else if userCount == 0 {
				hash, err := argon2id.CreateHash(cfg.BootstrapAdminPassword, auth.GetArgon2Params())
				if err != nil {
					slog.Error("bootstrap admin: hash password failed", "error", err)
				} else {
					var newUserID string
					err := db.Pool.QueryRow(ctx, `INSERT INTO users (id, username, password_hash, is_admin, is_active, email_verified, created_at)
						VALUES (gen_random_uuid(), $1, $2, true, true, true, now()) RETURNING id`,
						cfg.BootstrapAdminUser, hash).Scan(&newUserID)
					if err != nil {
						slog.Error("bootstrap admin: create user failed", "error", err)
					} else {
						slog.Info("bootstrap admin created", "username", cfg.BootstrapAdminUser, "user_id", newUserID)
						go db.LogAuthEvent(ctx, newUserID, "bootstrap_admin_created", "127.0.0.1", "setup.sh",
							map[string]string{"username": cfg.BootstrapAdminUser})
					}
				}
			} else {
				slog.Warn("BOOTSTRAP_ADMIN_USER is set but users already exist — remove BOOTSTRAP_ADMIN_PASSWORD from .env")
			}
		}
		slog.Warn("SECURITY: BOOTSTRAP_ADMIN_PASSWORD is still set in .env — remove it for security")
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

	// Mark all jobs that were still 'running' at startup as failed.
	// These are leftovers from the previous server process (crash or restart).
	// This must happen before InsertBackgroundJobGuarded calls so the partial-
	// unique index (user_id, job_type WHERE status='running') does not block
	// new jobs from being inserted.
	if _, cleanErr := db.Pool.Exec(context.Background(), `
		UPDATE background_jobs
		SET status = 'failed',
		    finished_at = NOW(),
		    error = 'Durch Server-Neustart unterbrochen'
		WHERE status = 'running'`); cleanErr != nil {
		slog.Warn("startup: failed to clean up stale running jobs", "error", cleanErr)
	} else {
		slog.Info("startup: stale running jobs marked as failed")
	}

	// Clean up garmin_import_jobs and strava_import_jobs the same way.
	// These use separate tables (not background_jobs) so the UPDATE above does not
	// reach them.  Without this, a job interrupted by a container restart stays
	// status='running' for up to 2 hours and the frontend reconnects in a loop,
	// showing "Verbinde mit laufendem Import…" indefinitely.
	for _, importTable := range [2]string{"garmin_import_jobs", "strava_import_jobs"} {
		var q string
		switch importTable {
		case "garmin_import_jobs":
			q = `UPDATE garmin_import_jobs
			     SET status = 'failed', completed_at = NOW(),
			         errors = jsonb_build_array('Import durch Server-Neustart unterbrochen. Bitte erneut starten.')
			     WHERE status = 'running'`
		case "strava_import_jobs":
			q = `UPDATE strava_import_jobs
			     SET status = 'failed', completed_at = NOW(),
			         errors = jsonb_build_array('Import durch Server-Neustart unterbrochen. Bitte erneut starten.')
			     WHERE status = 'running'`
		}
		if res, cleanErr := db.Pool.Exec(context.Background(), q); cleanErr != nil {
			slog.Warn("startup: failed to clean up stale "+importTable+" jobs", "error", cleanErr)
		} else if res.RowsAffected() > 0 {
			slog.Info("startup: stale "+importTable+" jobs marked as failed", "count", res.RowsAffected())
		}
	}

	// NOTE: Stale running jobs are handled later via h.ResumeInterruptedJobs
	// after the Handler is fully initialised (needs DB + Handler for matching).

	// ShadowFS (unified storage) — both backends always created when configured;
	// UnifiedShadowFS routes to the target backend and can read from either.
	var localBE, s3BE shadowfs.StorageBackend
	if cfg.LibraryBasePath != "" {
		localBE = shadowfs.NewLocalBackend(cfg.LibraryBasePath)
	}
	if cfg.S3Endpoint != "" {
		s3BE = shadowfs.NewS3Backend(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey,
			cfg.S3Bucket, cfg.S3Region, cfg.S3ForcePathStyle)
	}
	store := shadowfs.NewUnifiedShadowFS(db.Pool, localBE, s3BE, cfg.StorageBackend,
		cfg.LibraryBasePath, shadowfs.NewSchemaRenderer(), cfg.LibraryKeepVersions,
		cfg.MaxUserStorageBytes, cfg.S3Bucket, cfg.S3BucketActivity, cfg.S3BucketMedia)
	slog.Info("shadowfs: unified backend active", "target", cfg.StorageBackend)

	// S3 startup connectivity check — fail fast when STORAGE_BACKEND=s3 but S3 is unreachable.
	// Without this guard, imports silently lose media files (uploaded to non-existent S3,
	// then temp directory cleaned up → data permanently lost).
	if cfg.StorageBackend == "s3" && store.S3Configured() {
		pingCtx, pingCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := store.PingS3(pingCtx); err != nil {
			pingCancel()
			slog.Error("STORAGE_BACKEND=s3 but S3 is unreachable — refusing to start (would cause silent data loss on imports)", "error", err)
			os.Exit(1)
		}
		pingCancel()
		slog.Info("shadowfs: S3 connectivity verified", "endpoint", cfg.S3Endpoint, "bucket", cfg.S3Bucket)
	}

	// PMC service — always created; used by event consumer and recompute endpoint.
	pmcService := &pmc.Service{DB: db}

	// Best Efforts service — always created; used by recompute and recalc endpoints.
	bestEffortsService := &bestefforts.Service{Store: db}

	// Settings store — write-through cache over server_settings table.
	settingsStore := settings.NewSettingsStore(db, cfg.SettingsEncryptionKey, cfg.StorageBackend)

	// NATS (optional - log warning if unavailable)
	var pub *event.Publisher
	pub, err = event.NewPublisher(cfg.NatsURL)
	if err != nil {
		slog.Warn("NATS unavailable, events will not be published", "error", err)
		pub = nil
	} else {
		defer pub.Close()

		// Start PMC consumer
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
	// The SegmentMatcher callback is wired after the Handler is created
	// (it references handler methods), so we set it below.
	importSvc := importer.NewService(db, store, pub, nil)
	importSvc.PMC = pmcService
	importSvc.BestEfforts = bestEffortsService

	h := &handler.Handler{
		DB:             db,
		Events:         pub,
		PMC:            pmcService,
		BestEfforts:    bestEffortsService,
		ShadowFS:       store,
		ShadowFSConfig: shadowFSCfg,
		ImportService:  importSvc,
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
			ImportAllowedDirs:   cfg.ImportAllowedDirs,
			OSMDataDir:         cfg.OSMDataDir,
			DEMDataDir:         cfg.DEMDataDir,
			BRouterSegmentsDir: cfg.BRouterSegmentsDir,
			BRouterProfilesDir: cfg.BRouterProfilesDir,
			TilesDir:           cfg.TilesDir,
			LibraryBasePath:     cfg.LibraryBasePath,
			StorageBackend:      cfg.StorageBackend,
			KeepVersions:        cfg.LibraryKeepVersions,
			SyncEnabled:         cfg.LibrarySyncEnabled,
			LocalBE:             localBE,
			S3BE:                s3BE,
			BucketFn:            store.BucketFor,
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

	h.CoverageBaseDir = filepath.Join(filepath.Dir(cfg.RoutingGraphPath), "coverage")

	// DiskCoverageReader — mmap-based reader for FCOV coverage binaries
	coverageDir := filepath.Join(filepath.Dir(cfg.RoutingGraphPath), "coverage")
	if _, statErr := os.Stat(coverageDir); statErr == nil {
		diskReader, drErr := coverage.NewDiskCoverageReader(coverageDir, 256)
		if drErr != nil {
			slog.Warn("disk coverage reader init failed", "error", drErr)
		} else {
			defer diskReader.Close()
			h.CoverageReader = diskReader
			slog.Info("disk coverage reader initialized", "dir", coverageDir)
		}
	}
	if h.CoverageReader != nil {
		graphMgr.CoverageReader = h.CoverageReader
	}

	if h.CoverageReader != nil {
		ts := h.CoverageReader.BuildTimestamp()
		h.PNGCache = coverage.NewPNGCache(10_000, ts)
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
	// Wire PostImportHook: triggers fusion queue recompute after successful imports.
	importSvc.PostImportHook = func(ctx context.Context, userID string) {
		h.MaybeTriggerFusionQueueCompute(ctx, userID)
	}

	mlClient := mlpkg.NewClient(cfg.MLServiceURL)
	analyticsHandler := analyticshandler.NewHandlerWithML(db, pub, mlClient)

	// Auth handler (Go-native auth)
	authMailer := auth.NewMailer(settingsStore)
	authRL := middleware.NewRateLimiter(ctx)
	authHandler := auth.NewHandler(db, cfg, settingsStore, authMailer, authRL, ctx)
	authHandler.ShadowFS = store

	// Wire settings store and mailer into main handler (needed for admin settings endpoints).
	h.Settings = settingsStore
	h.Mailer = authMailer

	// Library watcher (polling, local backend only)
	if cfg.StorageBackend == "local" && cfg.LibrarySyncEnabled {
		watcher := shadowfs.NewLibraryWatcher(db.Pool, cfg.LibraryBasePath, cfg.LibrarySyncAutoImport)
		shutdown.Go("library-watcher", func(ctx context.Context) { watcher.Run(ctx) })
	}

	// Token cleanup (daily, graceful shutdown)
	shutdown.Go("token-cleanup", func(ctx context.Context) {
		// Run once immediately at startup
		if err := db.CleanupExpiredTokens(ctx); err != nil {
			slog.Error("initial token cleanup failed", "error", err)
		} else {
			slog.Info("token cleanup completed at startup")
		}

		// M17: Run GDPR purge once at startup before entering the 24h ticker loop.
		if err := db.PurgeDeletedUsers(ctx, func(ctx context.Context, userID string) error {
			return store.DeleteAllForUser(ctx, userID)
		}); err != nil {
			slog.Error("startup GDPR purge failed", "error", err)
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
				if err := db.PurgeDeletedUsers(ctx, func(ctx context.Context, userID string) error {
					return store.DeleteAllForUser(ctx, userID)
				}); err != nil {
					slog.Error("GDPR purge failed", "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	})

	// ShadowFS reconciliation (daily, graceful shutdown)
	if cfg.LibraryBasePath != "" {
		reconcileRenderer := shadowfs.NewSchemaRenderer()
		reconcileJob := shadowfs.NewReconcileJob(db.Pool, localBE, s3BE, cfg.LibraryBasePath, store.BucketFor, reconcileRenderer, cfg.LibraryKeepVersions)
		shutdown.Go("shadowfs-reconciliation", func(ctx context.Context) {
			doReconcile := func() {
				slog.Info("shadowfs reconcile: starting run")
				result, err := reconcileJob.Run(ctx, nil)
				if err != nil {
					slog.Error("shadowfs reconcile: run failed", "error", err)
				} else {
					slog.Info("shadowfs reconcile: run complete",
						"orphaned", result.OrphanedFiles,
						"dangling", result.DanglingRecords,
						"dangling_cleaned", result.DanglingCleaned,
						"size_fixes", result.SizeMismatches,
						"archived_cleaned", result.ArchivedCleaned,
						"temp_cleaned", result.TempCleaned,
						"duplicates", result.DuplicatePaths,
						"purging_cleaned", result.PurgingCleaned,
						"rename_fixed", result.RenameFixed,
						"orphan_warnings", result.OrphanWarnings,
						"archive_warnings", len(result.ArchiveWarnings),
					)
				}
			}

			// L27: Run one immediate reconciliation on first startup.
			doReconcile()

			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					doReconcile()
				case <-ctx.Done():
					return
				}
			}
		})
	}

	// Load built-in profiles from JSON files into memory.
	profileDir := os.Getenv("BADROUTER_PROFILE_DIR")
	if profileDir == "" {
		profileDir = "config/badrouter"
	}
	profileCount, profileErr := routing.LoadBuiltinProfiles(profileDir)
	if profileErr != nil {
		slog.Error("built-in profile load errors", "dir", profileDir, "error", profileErr)
	}
	if profileCount == 0 {
		slog.Error("no profiles found — check directory and JSON files", "dir", profileDir)
		os.Exit(1)
	}
	for _, name := range routing.ValidProfileNames {
		if _, err := routing.BuiltinProfile(name); err != nil {
			slog.Error("required profile missing after load", "profile", name, "dir", profileDir)
			os.Exit(1)
		}
	}
	slog.Info("built-in profiles loaded", "count", profileCount, "dir", profileDir)

	// Seed profiles into DB: config/badrouter/ as system, data/profiles/ as filesystem.
	// Order matters: filesystem overrides system on name collision (ON CONFLICT).
	if n, err := handler.SeedProfiles(ctx, profileDir, db, routing.SourceSystem); err != nil {
		slog.Warn("system profile seed failed", "error", err)
	} else if n > 0 {
		slog.Info("system profiles seeded", "count", n)
	}

	// Filesystem profile scan — import BadRouter JSON profiles from data/profiles/badrouter/
	if n, err := handler.SeedProfiles(ctx, "data/profiles/badrouter", db, routing.SourceFilesystem); err != nil {
		slog.Warn("filesystem profile scan failed", "error", err)
	} else if n > 0 {
		slog.Info("filesystem profiles imported", "count", n)
	}

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
	// Probe endpoints — outside /api/ prefix, accessed directly by container healthchecks
	r.Get("/livez", h.Livez)
	r.Get("/readyz", h.Readyz)
	r.Get("/healthz", h.Healthz)
	r.Get("/health", h.Healthz) // backwards compat alias
	r.Get("/api/v1/coverage/info", h.CoverageBuildInfo)
	r.Get("/api/v1/coverage/tiles/*", h.CoverageTile)
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

		// ── Authenticated auth routes (JWT required) ──
		r.Group(func(r chi.Router) {
			if cfg.DevAuth {
				r.Use(middleware.DevAuthMiddleware)
			} else {
				jwtAuth := middleware.NewJWTAuth(cfg.JWTSecretBytes)
				r.Use(jwtAuth.Middleware)
			}
			r.Use(middleware.RequirePasswordChanged())

			r.With(middleware.RateLimitChangePassword(authRL)).
				Post("/change-password", authHandler.ChangePassword)
			r.With(middleware.RateLimitGeneric(authRL)).
				Get("/sessions", authHandler.ListMySessions)
			r.With(middleware.RateLimitGeneric(authRL)).
				Post("/sessions/{familyId}/revoke", authHandler.RevokeSession)
			r.Post("/link/{provider}", authHandler.LinkSocialAccount)
			r.Delete("/link/{provider}", authHandler.UnlinkSocialAccount)
			r.With(middleware.RateLimitPerUser(authRL, "resend:", 3.0/3600, 3)).
				Post("/resend-verification", authHandler.ResendVerification)
			r.With(middleware.RateLimitGeneric(authRL)).
				With(middleware.RateLimitPerUser(authRL, "change-email:", 3.0/3600, 3)).
				Post("/change-email", authHandler.ChangeEmail)
			r.With(middleware.RateLimitGeneric(authRL)).
				Delete("/account", authHandler.DeleteAccount)
			r.With(middleware.RateLimitPerUser(authRL, "export:", 1.0/300, 1)).
					Post("/data-export", authHandler.DataExport)
			r.With(middleware.RateLimitPerUser(authRL, "export-status:", 60.0/60, 10)).
				Get("/data-export/{jobID}", authHandler.DataExportStatus)
			r.Get("/data-export/{jobID}/download", authHandler.DataExportDownload)
			r.With(middleware.RateLimitPerUser(authRL, "import:", 1.0/300, 1)).
				Post("/data-import", authHandler.DataImport)
			r.With(middleware.RateLimitPerUser(authRL, "import-status:", 60.0/60, 10)).
				Get("/data-import/{jobID}", authHandler.DataImportStatus)
		})
	})

	// Internal routes (no JWT auth — service-to-service only, network-level access control)
	r.Get("/internal/coverage/query", h.QueryCoverage)
	// ML service callback: authenticated via X-ML-Callback-Secret header, not JWT.
	r.Post("/api/v1/internal/ml-callback", h.HandleMLCallback)

	// Protected routes
	r.Group(func(r chi.Router) {
		if cfg.DevAuth {
			slog.Warn("DEV_AUTH enabled – using fixed dev user ID, NOT for production")
			r.Use(middleware.DevAuthMiddleware)
		} else {
			jwtAuth := middleware.NewJWTAuth(cfg.JWTSecretBytes)
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
				r.Post("/backfill-timezones", h.BackfillTimezones)
				r.Post("/phash-backfill", h.StartPhashBackfill)
				r.Post("/backfill-developer-apps", h.BackfillDeveloperApps)
				r.Post("/backfill-developer-field-columns", h.BackfillDeveloperFieldColumns)
				r.Post("/detect-max-hr", h.DetectMaxHR)
				r.Post("/activities/recompute-all", h.RecomputeAllActivities)
				r.Get("/activities/recompute-status", h.RecomputeStatus)
				r.Post("/recompute-tss", h.RecomputeAllTSS)

				// OSM
				r.Get("/osm-status", h.GetOSMStatus)
				r.Get("/osm-regions/status", h.GetOSMRegionStatus)
				r.Get("/osm-disk-usage", h.GetOSMDiskUsage)
				r.Post("/osm-regions/update-all", h.BatchUpdateAllRegions)
				r.Delete("/osm-regions/{regionID}", h.DeleteOSMRegion)
				r.Post("/osm-regions/{regionID}/update", h.TriggerOSMRegionUpdate)
				r.Get("/osm-regions", h.ListOSMRegions)
				r.Post("/osm-import/start", h.StartOSMImport)
				r.Post("/osm-import/cancel/{jobID}", h.CancelOSMImport)
				r.Post("/osm-import/retry-enrichment", h.RetryOSMEnrichment)
				r.Get("/osm-schedule", h.GetOSMSchedule)
				r.Post("/osm-schedule", h.SetOSMSchedule)

				// DEM
				r.Get("/dem-status", h.GetDEMStatus)
				r.Post("/dem/download", h.DownloadDEM)
				r.Get("/dem/status", h.GetDEMRegionStatus)

				// Routing infrastructure
				r.Post("/routing/rebuild-tiles", h.RebuildRoutingTiles)
				r.Post("/routing/rebuild-landmarks", h.RebuildLandmarks)
				r.Post("/routing/build-brouter-segments", h.BuildBRouterSegments)
				r.Post("/routing/download-brouter-segments", h.DownloadBRouterSegments)
				r.Put("/routing/cache-budget", h.SetCacheBudget)
				r.Post("/routing/brouter-profiles", h.HandleUploadBrouterProfile)
				r.Delete("/routing/brouter-profiles/{name}", h.HandleDeleteBrouterProfile)
				r.Post("/build-coverage", h.BuildCoverage)

				// LLM config (system-level)
				r.Get("/llm-config", h.GetLLMConfig)
				r.Put("/llm-config/{role}", h.PutLLMConfigRole)
				r.Get("/llm-config/models", h.GetLLMModels)
				r.Post("/llm-config/test", h.TestLLMConnection)

				// ML System Settings (nightly job toggles etc.)
				r.Get("/settings/ml", h.GetMLSystemSettings)
				r.Put("/settings/ml/{key}", h.PutMLSystemSetting)

				// Best efforts batch operations
				r.Post("/best-efforts/recalc-all", h.RecalcAllUsersBestEfforts)

				// ShadowFS dashboard & reconciliation
				r.Get("/storage", h.StorageDashboard)
				r.Post("/storage/reconcile", h.StartReconciliation)
				r.Post("/storage/backfill-thumbnails", h.BackfillThumbnails)
				r.Post("/storage/backfill-meta", h.BackfillMetaSidecars)

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
				r.Post("/users/{id}/hotfolder", h.AdminToggleUserHotfolder)
				r.Post("/users/{id}/set-admin", authHandler.AdminSetUserAdmin)
				r.Delete("/users/{id}", authHandler.AdminDeleteUser)
				r.Get("/audit-log", authHandler.AdminAuditLog)
				r.Post("/users/{id}/restore", authHandler.AdminRestoreUser)

				// Server settings
				r.Get("/settings", h.AdminGetSettings)
				r.Put("/settings", h.AdminPutSettings)
				r.Post("/settings/reload", h.AdminReloadSettings)
				r.Post("/settings/test-email", h.AdminTestEmail)
			})

			// ── Regular authenticated routes ──
			// Auth-spezifische Routen (change-password, sessions, etc.) sind
			// im /api/v1/auth Block als authentifizierte Sub-Gruppe registriert.

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
			r.Get("/activities/{id}/developer-fields", h.GetActivityDeveloperFields)
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
			r.Get("/activities/{id}/dive", h.GetActivityDiveInfo)
			r.Get("/activities/{id}/health-context", h.GetActivityHealthContext)
			r.Get("/activities/{id}/best-efforts", h.GetActivityBestEfforts)

			// Tile proxy — authenticates and enforces per-user heatmap isolation
			r.Get("/tiles/*", h.ProxyMartinTile)

			// Training Zones
			r.Get("/training-zones", h.GetTrainingZonesAll)
			r.Put("/training-zones", h.UpdateTrainingZones)

			// Best Efforts
			r.Get("/personal-records", h.GetBestEffortRecords)
			r.Get("/personal-records/history", h.GetPRHistory)
			r.Get("/best-effort-exclusions", h.GetBestEffortExclusions)
			r.Post("/best-effort-exclusions", h.AddBestEffortExclusion)
			r.Delete("/best-effort-exclusions/{activityID}/{category}", h.RemoveBestEffortExclusion)
			r.Post("/best-efforts/recalc", h.RecalcBestEfforts)

			r.Get("/workouts", h.HandleListWorkouts)
			r.Get("/workouts/{id}", h.HandleGetWorkout)
			r.Get("/stats/pmc", h.GetPMC)
			r.Get("/stats/power-curve", h.GetPowerCurve)

			r.Get("/privacy-zones", h.ListPrivacyZones)
			r.Post("/privacy-zones", h.CreatePrivacyZone)
			r.Put("/privacy-zones/{id}", h.UpdatePrivacyZone)
			r.Delete("/privacy-zones/{id}", h.DeletePrivacyZone)

			r.Post("/automation-rules/commute", h.CreateOrExtendCommuteRule)

			// Compat shims: read/write user_file_schemas (legacy column dropped).
			r.Get("/settings/shadow-fs", h.GetShadowFSSettingsCompat)
			r.Put("/settings/shadow-fs", h.UpdateShadowFSSettingsCompat)
			r.Post("/shadow-fs/preview-rename", h.PreviewRename)
			r.Post("/shadow-fs/execute-rename", h.ExecuteRename)
			r.Get("/imports", h.GetImports)

			r.Get("/activities/{id}/geo-tags", h.GetGeoTags)
			r.Get("/activities/{id}/enrichment-status", h.GetEnrichmentStatus)

			// User-triggered enrichment backfills (non-admin, user-scoped).
			r.Get("/enrichment/geocoding-gap", h.GeocodingGap)
			r.Post("/enrichment/geocoding-backfill", h.GeocodingBackfill)

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
			r.Get("/tags/{id}/cover", h.ServeTagCover)
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

			r.Get("/settings/file-schemas", h.GetFileSchemas)
			r.Put("/settings/file-schemas/{type}", h.UpdateFileSchema)
			r.Post("/settings/file-schemas/{type}/rename-preview", h.PreviewFileRename)
			r.Post("/settings/file-schemas/{type}/rename-execute", h.ExecuteFileRename)
			r.Get("/settings/library-dir", h.GetLibraryDir)
			r.Put("/settings/library-dir", h.UpdateLibraryDir)

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
			r.Post("/automation-rules/{id}/apply", h.ApplySingleRule)

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
			r.Get("/files/{fileID}", h.ServeFile)

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
			r.Get("/segment-prs", h.GetSegmentPRs)
			r.Get("/segment-prs/global-top", h.GetGlobalTopSegmentEfforts)
			r.Get("/segments", h.ListSegments)
			r.Post("/segments", h.CreateSegment)
			r.Delete("/segments", h.DeleteSegments)
			r.Post("/segments/export", h.ExportSegments)
			r.Post("/segments/import-lifts", h.ImportLiftsFromOSM)
			r.Get("/segments/{id}", h.GetSegment)
			r.Put("/segments/{id}", h.UpdateSegment)
			r.Delete("/segments/{id}", h.DeleteSegment)
			r.Get("/segments/{id}/leaderboard", h.GetSegmentLeaderboard)
			r.Post("/segments/{id}/recompute", h.RecomputeSegment)
			r.Post("/segments/{id}/publish", h.PublishSegment)
			r.Get("/segments/{id}/favorite", h.GetSegmentFavoriteStatus)
			r.Post("/segments/{id}/favorite", h.AddSegmentFavorite)
			r.Delete("/segments/{id}/favorite", h.RemoveSegmentFavorite)
			r.Get("/segments/{id}/performances", h.GetSegmentPerformances)
			r.Get("/segments/{id}/export", h.ExportSegment)
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
			r.Post("/fusion-merge", h.FusionMergeV2) // unified timeline, no target
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
				r.Post("/blood-pressure", h.CreateHealthBloodPressure)
				r.Get("/fitness-age", h.GetHealthFitnessAge)
				r.Get("/overview", h.GetHealthOverview)
				r.Get("/monitoring", h.GetHealthMonitoring)
				r.Get("/monitoring/daily", h.GetHealthMonitoringDaily)
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

			// Recording Groups
			r.Get("/recording-groups", h.GetRecordingGroups)
			r.Get("/recording-groups/{id}", h.GetRecordingGroup)
			r.Get("/recording-groups/{id}/members", h.GetRecordingGroupMembers)
			r.Post("/recording-groups/{id}/dissolve", h.DissolveRecordingGroup)
			r.Patch("/recording-groups/{id}/primary", h.SetRecordingGroupPrimary)
			r.Patch("/recording-groups/{id}/sport-type", h.SetRecordingGroupSportType)

			// Chat
			r.Mount("/chat", h.ChatRoutes())

			// Analytics
			r.Mount("/analytics", analyticsHandler.Routes())

			// ML Status
			r.Get("/ml/status", analyticsHandler.GetMLStatus)

			// Running Form
			r.Get("/running-form/baseline", h.GetRunningFormBaseline)
			r.Get("/running-form/trend", h.GetRunningFormTrend)

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
			r.Route("/routing/profiles", func(r chi.Router) {
				r.Get("/", h.HandleListProfiles)
				r.Post("/", h.HandleCreateProfile)
				r.Post("/import", h.HandleImportProfiles)
				r.Get("/defaults/{name}", h.HandleGetProfileDefaults)
				r.Put("/{id}", h.HandleUpdateProfile)
				r.Delete("/{id}", h.HandleDeleteProfile)
				r.Get("/{id}/export", h.HandleExportProfile)
			})

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
		for _, uid := range userIDs {
			if err := pmcService.RecalculatePMC(ctx, uid, time.Now()); err != nil {
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
		}, settingsStore)
		h.WatcherManager = wm
		defer wm.StopAll()

		// Start watchers for all users that have hotfolder enabled in DB.
		enabledUsers, err := db.GetUsersWithHotfolderEnabled(ctx)
		if err != nil {
			slog.Warn("hotfolder: failed to load enabled users", "error", err)
		} else {
			for _, userID := range enabledUsers {
				if err := wm.EnableUser(userID, false); err != nil {
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
