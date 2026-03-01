package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/skadia-app/skadia/services/api/internal/config"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/handler"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/pmc"
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

	// S3 Storage (Garage)
	s3Client, err := storage.NewS3Client(ctx, cfg.GarageEndpoint, cfg.GarageAccessKey, cfg.GarageSecretKey, cfg.GarageBucket)
	if err != nil {
		slog.Error("failed to create S3 client", "error", err)
		os.Exit(1)
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
			OIDCClientID:     cfg.OIDCClientID,
			OIDCClientSecret: cfg.OIDCClientSecret,
			OIDCAppSlug:      cfg.OIDCAppSlug,
			AppRedirectURI:        cfg.AppRedirectURI,
			FrontendURL:           cfg.FrontendURL,
			JWTSecret:             []byte(cfg.JWTSecret),
		}
		slog.Info("OIDC OIDC configured",
			"base_url", cfg.OIDCBaseURL,
			"redirect_uri", cfg.AppRedirectURI,
		)
	} else {
		slog.Warn("OIDC OIDC not configured (OIDC_CLIENT_ID or JWT_SECRET missing)")
	}

	// Handler
	h := &handler.Handler{
		DB:      db,
		Storage: s3Client,
		Events:  pub,
		Auth:    authCfg,
	}

	// Router
	r := chi.NewRouter()
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(chimiddleware.Recoverer)

	// Public routes (no authentication required)
	r.Get("/health", h.Health)
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Get("/login", h.AuthLogin)
		r.Get("/callback", h.AuthCallback)
		r.Post("/refresh", h.AuthRefresh)
		r.Post("/logout", h.AuthLogout)
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
			r.Post("/activities/recompute-all", h.RecomputeAllActivities)
			r.Get("/activities", h.ListActivities)
			r.Get("/activities/{id}", h.GetActivity)
			r.Patch("/activities/{id}", h.UpdateActivity)
			r.Delete("/activities/{id}", h.DeleteActivity)
			r.Post("/activities/{id}/recompute", h.RecomputeActivity)
			r.Get("/activities/{id}/streams", h.GetStreams)
			r.Get("/activities/{id}/sensors", h.GetSensors)
			r.Get("/stats/pmc", h.GetPMC)
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
