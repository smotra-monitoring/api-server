package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/smotra-monitoring/server/internal/config"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/handlers/health"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/middleware"
)

const version = "0.0.1"

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New(logger.Config{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
	})

	log.Info("starting smotra monitoring server",
		"version", version,
		"environment", cfg.Server.Environment,
	)

	// Initialize database
	dbConfig := database.Config{
		Type:            cfg.Database.Type,
		Host:            cfg.Database.Host,
		Port:            cfg.Database.Port,
		Username:        cfg.Database.Username,
		Password:        cfg.Database.Password,
		Database:        cfg.Database.Database,
		SSLMode:         cfg.Database.SSLMode,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.Database.ConnMaxIdleTime,
		FilePath:        cfg.Database.FilePath,
	}

	db, err := database.New(dbConfig)
	if err != nil {
		log.Error("failed to create database", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	if err := db.Open(ctx); err != nil {
		log.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	log.Info("database connection established", "type", cfg.Database.Type)

	// Initialize handlers
	healthHandler := health.NewHandler(log, db, version)

	// Create router
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.Recovery(log))
	r.Use(middleware.Logger(log))
	r.Use(middleware.CORS)

	// Health check routes (no authentication required)
	r.Get("/healthz", healthHandler.HealthCheck)
	r.Get("/healthz/ready", healthHandler.ReadinessCheck)
	r.Get("/healthz/live", healthHandler.LivenessCheck)

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Future API endpoints will be added here
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"message":"Smotra Monitoring API v1","version":"` + version + `"}`))
		})
	})

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  120 * time.Second,
	}

	// Server run context
	serverCtx, serverStopCtx := context.WithCancel(context.Background())

	// Listen for syscall signals for process to interrupt/quit
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Start server in a goroutine
	go func() {
		log.Info("server starting",
			"address", srv.Addr,
			"environment", cfg.Server.Environment,
		)

		// Mark server as ready
		healthHandler.SetReady(true)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for signal
	<-sig

	// Shutdown signal received
	log.Info("shutting down server gracefully")

	// Mark server as not ready
	healthHandler.SetReady(false)

	// Shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(serverCtx, cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	// Attempt graceful shutdown
	go func() {
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("server shutdown error", "error", err)
		}
		serverStopCtx()
	}()

	// Wait for shutdown to complete or timeout
	<-shutdownCtx.Done()
	if shutdownCtx.Err() == context.DeadlineExceeded {
		log.Warn("shutdown timeout exceeded, forcing shutdown")
	}

	log.Info("server stopped")
}
