package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/smotra-monitoring/server/internal/api"
	"github.com/smotra-monitoring/server/internal/config"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/handlers"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/middleware"
)

const appVersion = "0.0.1"

func main() {
	// Parse command line flags
	configFile := flag.String("c", "config.yaml", "Path to configuration file")
	flag.Parse()

	if *configFile == "" || configFile == nil {
		fmt.Fprintf(os.Stderr, "Configuration file path cannot be empty\n")
		os.Exit(1)
	}

	cfg, err := config.LoadAndValidate(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New(logger.Config{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
	})

	log.Info("starting the server",
		"version", appVersion,
		"environment", cfg.Server.Environment,
	)

	// Initialize database
	var db database.Database
	switch cfg.DatabaseType {
	case "postgres":
		db = database.NewPostgresDB(*cfg.PostgresConfig)
	case "sqlite":
		db = database.NewSQLiteDB(*cfg.SQLiteConfig)
	default:
		err = fmt.Errorf("unsupported database type: %s", cfg.DatabaseType)
	}

	if err != nil {
		log.Error("failed to create database", "error", err)
		os.Exit(1)
	}

	db_ctx := context.Background()
	if err := db.Open(db_ctx); err != nil {
		log.Error("failed to open database", "error", err)
		os.Exit(1)
	}

	defer db.Close()

	log.Info("database connection established", "type", cfg.DatabaseType)

	// Create router
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID(log))
	r.Use(middleware.Logger(log))
	r.Use(middleware.Recovery(log))
	r.Use(middleware.CORS)

	// Authentication middleware - only attempts authentication, doesn't require it
	// This allows public endpoints to work while authenticated endpoints can check the context
	r.Use(middleware.AgentAPIKeyAuth(log, db))
	r.Use(middleware.OAuth2Auth(log))

	// Initialize handlers with authentication wrapper
	handler := handlers.NewAuthenticatedHandler(log, db, cfg, appVersion)

	// Register API handler
	strictHandler := api.NewStrictHandler(handler, nil)
	api.HandlerFromMux(strictHandler, r)

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Future API endpoints will be added here
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"message":"Monitoring API v1","version":"` + appVersion + `"}`))
		})
	})

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Listen for syscall signals for process to interrupt/quit
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Start server in a goroutine
	go func() {
		log.Info("server starting",
			"address", srv.Addr,
			"environment", cfg.Server.Environment,
		)

		// Mark server as ready
		handler.SetReady(true)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for signal
	<-sig

	// Shutdown signal received
	log.Info("shutting down the server ...")

	// Mark server as not ready
	handler.SetReady(false)

	// Shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	// Attempt graceful shutdown
	if err := srv.Shutdown(shutdownCtx); err != nil {
		if err == context.DeadlineExceeded {
			log.Warn("graceful shutdown timed out, forcing exit")
		} else {
			log.Error("graceful shutdown error", "error", err)
		}
	} else {
		log.Info("server stopped gracefully")
	}
}
