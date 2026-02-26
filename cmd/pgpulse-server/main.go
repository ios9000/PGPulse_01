package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ios9000/PGPulse_01/internal/config"
	"github.com/ios9000/PGPulse_01/internal/orchestrator"
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

	store := orchestrator.NewLogStore(logger)
	orch := orchestrator.New(cfg, store, logger)

	// Listen for SIGINT / SIGTERM before starting so we don't miss signals
	// that arrive during startup.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = shutdownCtx // will be used by the HTTP server in M2

	orch.Stop()

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
