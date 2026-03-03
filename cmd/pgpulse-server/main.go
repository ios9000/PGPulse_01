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

	"github.com/ios9000/PGPulse_01/internal/alert"
	"github.com/ios9000/PGPulse_01/internal/alert/notifier"
	"github.com/ios9000/PGPulse_01/internal/api"
	"github.com/ios9000/PGPulse_01/internal/auth"
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

		// Alert pipeline setup.
		var (
			evaluator         *alert.Evaluator
			alertDispatcher   *alert.Dispatcher
			notifierRegistry  *alert.NotifierRegistry
			alertRuleStore    alert.AlertRuleStore
			alertHistoryStore alert.AlertHistoryStore
		)

		if cfg.Alerting.Enabled {
			alertRuleStore = alert.NewPGAlertRuleStore(pgPool)
			alertHistoryStore = alert.NewPGAlertHistoryStore(pgPool)

			if err := alert.SeedBuiltinRules(ctx, alertRuleStore, logger); err != nil {
				logger.Error("failed to seed alert rules", "error", err)
				os.Exit(1)
			}

			evaluator = alert.NewEvaluator(alertRuleStore, alertHistoryStore, logger)
			if err := evaluator.LoadRules(ctx); err != nil {
				logger.Error("failed to load alert rules", "error", err)
				os.Exit(1)
			}
			if err := evaluator.RestoreState(ctx); err != nil {
				logger.Error("failed to restore alert state", "error", err)
				os.Exit(1)
			}
			evaluator.StartCleanup(ctx, cfg.Alerting.HistoryRetentionDays)

			notifierRegistry = alert.NewNotifierRegistry()
			if cfg.Alerting.Email != nil {
				emailNotifier := notifier.NewEmailNotifier(*cfg.Alerting.Email, cfg.Alerting.DashboardURL, logger)
				notifierRegistry.Register(emailNotifier)
			}

			alertDispatcher = alert.NewDispatcher(notifierRegistry, cfg.Alerting.DefaultChannels, cfg.Alerting.DefaultCooldownMinutes, logger)
			alertDispatcher.Start()

			logger.Info("alerting pipeline started",
				"channels", notifierRegistry.Names())
		}

		// Wire auth when enabled — requires a storage DSN (validated in config).
		if cfg.Auth.Enabled {
			userStore := auth.NewPGUserStore(pgPool)

			count, err := userStore.Count(ctx)
			if err != nil {
				logger.Error("failed to count users", "err", err)
				os.Exit(1)
			}

			if count == 0 {
				if cfg.Auth.InitialAdmin == nil {
					logger.Error("auth enabled but no users exist and auth.initial_admin not configured")
					os.Exit(1)
				}
				hash, err := auth.HashPassword(cfg.Auth.InitialAdmin.Password, cfg.Auth.BcryptCost)
				if err != nil {
					logger.Error("failed to hash initial admin password", "err", err)
					os.Exit(1)
				}
				if _, err := userStore.Create(ctx, cfg.Auth.InitialAdmin.Username, hash, auth.RoleAdmin); err != nil {
					logger.Error("failed to create initial admin user", "err", err)
					os.Exit(1)
				}
				logger.Warn("created initial admin user — change password immediately",
					"username", cfg.Auth.InitialAdmin.Username)
			}

			jwtSvc := auth.NewJWTService(cfg.Auth.JWTSecret, cfg.Auth.AccessTokenTTL, cfg.Auth.RefreshTokenTTL)
			apiServer := api.New(cfg, store, pinger, jwtSvc, userStore, logger,
				alertRuleStore, alertHistoryStore, evaluator, notifierRegistry)
			startServer(ctx, stop, cfg, apiServer, store, logger, evaluator, alertDispatcher)
			return
		}

		// Auth disabled with storage.
		apiServer := api.New(cfg, store, pinger, nil, nil, logger,
			alertRuleStore, alertHistoryStore, evaluator, notifierRegistry)
		startServer(ctx, stop, cfg, apiServer, store, logger, evaluator, alertDispatcher)
	} else {
		store = orchestrator.NewLogStore(logger)
		logger.Info("no storage DSN configured, using log-only mode")

		apiServer := api.New(cfg, store, pinger, nil, nil, logger, nil, nil, nil, nil)
		startServer(ctx, stop, cfg, apiServer, store, logger, nil, nil)
	}
}

// startServer wires the HTTP server and orchestrator, then blocks until shutdown.
func startServer(ctx context.Context, stop context.CancelFunc, cfg config.Config,
	apiServer *api.APIServer, store collector.MetricStore, logger *slog.Logger,
	evaluator *alert.Evaluator, dispatcher *alert.Dispatcher) {

	orch := orchestrator.New(cfg, store, logger, evaluator, dispatcher)

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
		logger.Warn("orchestrator not started, HTTP server will continue serving",
			"err", err)
	}

	logger.Info("PGPulse server running",
		"listen", cfg.Server.Listen,
		"instances", len(cfg.Instances),
	)

	<-ctx.Done()
	logger.Info("shutdown signal received")

	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	logger.Info("shutting down HTTP server")
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP shutdown error", "err", err)
	}

	logger.Info("stopping orchestrator")
	orch.Stop()

	// Drain buffered alert events before closing storage.
	if dispatcher != nil {
		logger.Info("stopping alert dispatcher")
		dispatcher.Stop()
	}

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
