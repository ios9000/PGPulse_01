package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/ios9000/PGPulse_01/internal/api"
	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/config"
	"github.com/ios9000/PGPulse_01/internal/orchestrator"
	"github.com/ios9000/PGPulse_01/internal/storage"
)

func main() {
	configPath := flag.String("config", "pgpulse.yml", "path to configuration file")
	flag.Parse()

	// Bootstrap logger at info level; reconfigured once config is loaded.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	// Reconfigure logger with the level from config.
	level, err := parseLogLevel(cfg.Server.LogLevel)
	if err != nil {
		logger.Error("invalid log level", "level", cfg.Server.LogLevel, "err", err)
		os.Exit(1)
	}
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	// Activate signal handling early so Ctrl-C during startup is caught.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Initialise storage: real PostgreSQL when DSN is configured, log-only otherwise.
	var store collector.MetricStore
	var pinger api.Pinger // nil unless using PGStore

	if cfg.Storage.DSN != "" {
		pgPool, err := storage.NewPool(ctx, cfg.Storage.DSN)
		if err != nil {
			logger.Error("failed to connect to storage DB", "err", err)
			os.Exit(1)
		}

		if err := storage.Migrate(ctx, pgPool, logger, storage.MigrateOptions{
			UseTimescaleDB: cfg.Storage.UseTimescaleDB,
		}); err != nil {
			logger.Error("failed to run migrations", "err", err)
			os.Exit(1)
		}

		pgStore := storage.NewPGStore(pgPool, logger)
		store = pgStore
		pinger = pgStore.Pool()
		logger.Info("storage initialized with PostgreSQL")
	} else {
		store = orchestrator.NewLogStore(logger)
		logger.Info("no storage DSN configured, using log-only mode")
	}

	orch := orchestrator.New(cfg, store, logger)

	// Build and start HTTP API server.
	apiServer := api.New(cfg, store, pinger, logger)
	httpServer := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      apiServer.Routes(),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		logger.Info("starting HTTP server", "address", cfg.Server.Listen)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", "err", err)
		}
	}()

	if err := orch.Start(ctx); err != nil {
		logger.Error("failed to start orchestrator", "err", err)
		os.Exit(1)
	}

	logger.Info("PGPulse server running",
		"listen", cfg.Server.Listen,
		"instances", len(cfg.Instances),
	)

	<-ctx.Done()
	logger.Info("shutdown signal received")

	// Stop listening for signals; subsequent signals will terminate immediately.
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	logger.Info("shutting down HTTP server")
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP shutdown error", "err", err)
	}

	logger.Info("stopping orchestrator")
	orch.Stop()

	logger.Info("closing storage")
	if err := store.Close(); err != nil {
		logger.Error("failed to close store", "err", err)
	}

	logger.Info("PGPulse server stopped")
}

func parseLogLevel(s string) (slog.Level, error) {
	switch s {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level %q", s)
	}
}
