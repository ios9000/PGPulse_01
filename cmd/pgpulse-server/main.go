package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ios9000/PGPulse_01/internal/alert"
	"github.com/ios9000/PGPulse_01/internal/alert/notifier"
	"github.com/ios9000/PGPulse_01/internal/api"
	"github.com/ios9000/PGPulse_01/internal/auth"
	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/config"
	"github.com/ios9000/PGPulse_01/internal/ml"
	"github.com/ios9000/PGPulse_01/internal/orchestrator"
	"github.com/ios9000/PGPulse_01/internal/plans"
	"github.com/ios9000/PGPulse_01/internal/remediation"
	"github.com/ios9000/PGPulse_01/internal/settings"
	"github.com/ios9000/PGPulse_01/internal/storage"
)

func main() {
	// CLI flags
	flagTarget := flag.String("target", "", "PostgreSQL DSN for quick-start mode")
	flagTargetHost := flag.String("target-host", "", "PostgreSQL host (alternative to --target)")
	flagTargetPort := flag.Int("target-port", 5432, "PostgreSQL port")
	flagTargetUser := flag.String("target-user", "pgpulse_monitor", "PostgreSQL user")
	flagTargetPassword := flag.String("target-password", "", "PostgreSQL password")
	flagTargetDBName := flag.String("target-dbname", "postgres", "PostgreSQL database")
	flagListen := flag.String("listen", "", "HTTP listen address:port (default :8989)")
	flagHistory := flag.Duration("history", 2*time.Hour, "Memory retention for live mode")
	flagNoAuth := flag.Bool("no-auth", false, "Disable authentication")
	flagConfig := flag.String("config", "pgpulse.yml", "Config file path")
	flag.Parse()

	// Bootstrap logger at info level; reconfigured once config is loaded.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Validate mutually exclusive flags.
	if *flagTarget != "" && *flagTargetHost != "" {
		logger.Error("--target and --target-host are mutually exclusive")
		os.Exit(1)
	}

	// Synthesize CLI instance if --target or --target-host provided.
	cliInstance, err := synthesizeCLIInstance(*flagTarget, *flagTargetHost, *flagTargetPort, *flagTargetUser, *flagTargetPassword, *flagTargetDBName)
	if err != nil {
		logger.Error("invalid target", "err", err)
		os.Exit(1)
	}

	// Load config, allowing missing file when CLI target is provided.
	cfg, err := config.Load(*flagConfig)
	if err != nil {
		if cliInstance != nil {
			// No config file but --target provided — use defaults.
			logger.Info("no config file found, using CLI flags", "config", *flagConfig)
			cfg = config.Config{}
		} else {
			logger.Error("failed to load config", "err", err,
				"hint", "provide a config file or use --target to monitor a PostgreSQL instance")
			os.Exit(1)
		}
	}

	// CLI flag overrides.
	if cliInstance != nil {
		cfg.Instances = []config.InstanceConfig{*cliInstance}
	}
	if *flagNoAuth {
		cfg.Auth.Enabled = false
	}
	if *flagListen != "" {
		cfg.Server.Listen = *flagListen
	}
	if cfg.Server.Port != 0 && cfg.Server.Listen == "" {
		cfg.Server.Listen = fmt.Sprintf(":%d", cfg.Server.Port)
	}
	if cfg.Server.Listen == "" {
		cfg.Server.Listen = ":8989"
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

	// Determine operating mode and auth mode.
	var (
		liveMode bool
		authMode = auth.AuthEnabled
	)
	if !cfg.Auth.Enabled || *flagNoAuth {
		authMode = auth.AuthDisabled
	}

	// Initialise storage: real PostgreSQL when DSN is configured, memory or log-only otherwise.
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

		// Instance store: create and seed from YAML config.
		instanceStore := storage.NewPGInstanceStore(pgPool)
		seedInstancesFromConfig(ctx, instanceStore, cfg.Instances, logger)

		// M8_02: Plan capture store + collector.
		planStore := plans.NewPGPlanStore(pgPool)
		planCollector := plans.NewCollector(plans.CaptureConfig{
			Enabled:               cfg.PlanCapture.Enabled,
			DurationThresholdMs:   cfg.PlanCapture.DurationThresholdMs,
			DedupWindowSeconds:    cfg.PlanCapture.DedupWindowSeconds,
			ScheduledTopNCount:    cfg.PlanCapture.ScheduledTopNCount,
			ScheduledTopNInterval: cfg.PlanCapture.ScheduledTopNInterval,
			MaxPlanBytes:          cfg.PlanCapture.MaxPlanBytes,
			RetentionDays:         cfg.PlanCapture.RetentionDays,
		}, planStore)
		_ = planCollector // used later by orchestrator integration

		// M8_02: Plan retention worker.
		if cfg.PlanCapture.Enabled && cfg.PlanCapture.RetentionDays > 0 {
			retentionWorker := plans.NewRetentionWorker(pgPool, cfg.PlanCapture.RetentionDays)
			go retentionWorker.Run(ctx)
			logger.Info("plan retention worker started", "retention_days", cfg.PlanCapture.RetentionDays)
		}

		// M8_02: Settings snapshot store + collector.
		snapshotStore := settings.NewPGSnapshotStore(pgPool)
		snapshotCollector := settings.NewSnapshotCollector(settings.SnapshotConfig{
			Enabled:           cfg.SettingsSnapshot.Enabled,
			ScheduledInterval: cfg.SettingsSnapshot.ScheduledInterval,
			CaptureOnStartup:  cfg.SettingsSnapshot.CaptureOnStartup,
			RetentionDays:     cfg.SettingsSnapshot.RetentionDays,
		}, snapshotStore)
		_ = snapshotCollector // used later by orchestrator integration

		// M8_03: ML anomaly detector bootstrap.
		var mlDetector *ml.Detector
		if cfg.ML.Enabled {
			mlMetrics := make([]ml.MetricConfig, len(cfg.ML.Metrics))
			for i, m := range cfg.ML.Metrics {
				mlMetrics[i] = ml.MetricConfig{Key: m.Key, Period: m.Period, Enabled: m.Enabled, ForecastHorizon: m.ForecastHorizon}
			}
			lister := ml.NewDBInstanceLister(pgPool)

			// M8_03: persistence store for ML baselines.
			var persistStore ml.PersistenceStore
			if cfg.ML.Persistence.Enabled {
				persistStore = ml.NewDBPersistenceStore(pgPool)
			}

			// AlertEvaluator will be set after alert pipeline is initialized.
			// Use a no-op for now; reassigned below if alerting is enabled.
			mlDetector = ml.NewDetector(ml.DetectorConfig{
				Enabled:            cfg.ML.Enabled,
				ZScoreWarn:         cfg.ML.ZScoreWarn,
				ZScoreCrit:         cfg.ML.ZScoreCrit,
				AnomalyLogic:       cfg.ML.AnomalyLogic,
				Metrics:            mlMetrics,
				CollectionInterval: cfg.ML.CollectionInterval,
				ForecastZ:          cfg.ML.Forecast.ConfidenceZ,
			}, store, lister, &noOpAlertEvaluator{}, persistStore)
		}

		// Alert pipeline setup.
		var (
			orchEvaluator     orchestrator.AlertEvaluator  = &orchestrator.NoOpAlertEvaluator{}
			orchDispatcher    orchestrator.AlertDispatcher  = &orchestrator.NoOpAlertDispatcher{}
			realDispatcher    *alert.Dispatcher
			notifierRegistry  *alert.NotifierRegistry
			alertRuleStore    alert.AlertRuleStore
			alertHistoryStore alert.AlertHistoryStore
			apiEvaluator      *alert.Evaluator // concrete type for API alert management
		)

		if cfg.Alerting.Enabled {
			alertRuleStore = alert.NewPGAlertRuleStore(pgPool)
			alertHistoryStore = alert.NewPGAlertHistoryStore(pgPool)

			if err := alert.SeedBuiltinRules(ctx, alertRuleStore, logger); err != nil {
				logger.Error("failed to seed alert rules", "error", err)
				os.Exit(1)
			}

			apiEvaluator = alert.NewEvaluator(alertRuleStore, alertHistoryStore, logger)
			if err := apiEvaluator.LoadRules(ctx); err != nil {
				logger.Error("failed to load alert rules", "error", err)
				os.Exit(1)
			}
			if err := apiEvaluator.RestoreState(ctx); err != nil {
				logger.Error("failed to restore alert state", "error", err)
				os.Exit(1)
			}
			apiEvaluator.StartCleanup(ctx, cfg.Alerting.HistoryRetentionDays)
			orchEvaluator = apiEvaluator

			notifierRegistry = alert.NewNotifierRegistry()
			if cfg.Alerting.Email != nil {
				emailNotifier := notifier.NewEmailNotifier(*cfg.Alerting.Email, cfg.Alerting.DashboardURL, logger)
				notifierRegistry.Register(emailNotifier)
			}

			realDispatcher = alert.NewDispatcher(notifierRegistry, cfg.Alerting.DefaultChannels, cfg.Alerting.DefaultCooldownMinutes, logger)
			realDispatcher.Start()
			orchDispatcher = realDispatcher

			logger.Info("alerting pipeline started",
				"channels", notifierRegistry.Names())
		}

		// M8_02: Upgrade ML detector to dispatch through the real alert pipeline.
		if mlDetector != nil && apiEvaluator != nil {
			mlDetector.SetAlertEvaluator(alert.NewMetricAlertAdapter(apiEvaluator))
			logger.Info("ML detector wired to alert pipeline")
		}

		// M8_05: Wire forecast provider into alert evaluator.
		if mlDetector != nil && apiEvaluator != nil {
			apiEvaluator.SetForecastProvider(mlDetector, cfg.ML.Forecast.AlertMinConsecutive)
			logger.Info("forecast alert provider wired to evaluator")
		}

		// M8_02: Bootstrap ML detector now that alert pipeline is ready.
		if mlDetector != nil {
			bootstrapCtx, bootstrapCancel := context.WithTimeout(ctx, 30*time.Second)
			if err := mlDetector.Bootstrap(bootstrapCtx); err != nil {
				logger.Warn("ML bootstrap incomplete", "err", err)
			}
			bootstrapCancel()
			logger.Info("ML anomaly detector initialized")
		}

		// M8_02: Wire plan + settings stores into API server (done via setters after creation).

		// REM_01a: Remediation engine setup.
		remEngine := remediation.NewEngine()
		remMetricSource := remediation.NewStoreMetricSource(store)
		remStore := remediation.NewPGStore(pgPool)
		remAdapter := remediation.NewAlertAdapter(remEngine, remMetricSource)
		if realDispatcher != nil {
			realDispatcher.SetRemediationProvider(remAdapter)
		}
		logger.Info("remediation engine initialized", "rules", len(remEngine.Rules()))

		// M10_01: Background advisor evaluation worker.
		if cfg.Remediation.Enabled {
			remLister := ml.NewDBInstanceLister(pgPool)
			bgEval := remediation.NewBackgroundEvaluator(
				remEngine, remStore, remMetricSource, remLister,
				cfg.Remediation.BackgroundInterval, cfg.Remediation.RetentionDays, logger,
			)
			bgEval.Start(ctx)
			defer bgEval.Stop()
			logger.Info("remediation background evaluator started",
				"interval", cfg.Remediation.BackgroundInterval,
				"retention_days", cfg.Remediation.RetentionDays,
			)
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
				if _, err := userStore.Create(ctx, cfg.Auth.InitialAdmin.Username, hash, string(auth.RoleSuperAdmin)); err != nil {
					logger.Error("failed to create initial admin user", "err", err)
					os.Exit(1)
				}
				logger.Warn("created initial admin user — change password immediately",
					"username", cfg.Auth.InitialAdmin.Username)
			}

			refreshSecret := cfg.Auth.RefreshSecret
			if refreshSecret == "" {
				refreshSecret = cfg.Auth.JWTSecret
			}
			jwtSvc := auth.NewJWTService(cfg.Auth.JWTSecret, refreshSecret, cfg.Auth.AccessTokenTTL, cfg.Auth.RefreshTokenTTL)
			apiServer := api.New(cfg, store, pinger, jwtSvc, userStore, logger,
				alertRuleStore, alertHistoryStore, apiEvaluator, notifierRegistry, instanceStore,
				false, 0, auth.AuthEnabled)
			apiServer.SetPlanStore(planStore)
			apiServer.SetSnapshotStore(snapshotStore)
			apiServer.SetRemediation(remEngine, remStore, remMetricSource)
			if mlDetector != nil {
				apiServer.SetMLDetector(mlDetector, cfg.ML)
			}
			startServer(ctx, stop, cfg, apiServer, store, logger, orchEvaluator, orchDispatcher, realDispatcher)
			return
		}

		// Auth disabled with storage.
		apiServer := api.New(cfg, store, pinger, nil, nil, logger,
			alertRuleStore, alertHistoryStore, apiEvaluator, notifierRegistry, instanceStore,
			false, 0, authMode)
		apiServer.SetPlanStore(planStore)
		apiServer.SetSnapshotStore(snapshotStore)
		apiServer.SetRemediation(remEngine, remStore, remMetricSource)
		startServer(ctx, stop, cfg, apiServer, store, logger, orchEvaluator, orchDispatcher, realDispatcher)
	} else if len(cfg.Instances) > 0 {
		// Live mode — in-memory storage with configured instances.
		liveMode = true
		memStore := storage.NewMemoryStore(*flagHistory)
		store = memStore
		logger.Info("starting in live mode", "storage", "memory", "retention", flagHistory.String())

		// Disable features that require persistent storage.
		cfg.ML.Enabled = false
		cfg.PlanCapture.Enabled = false
		cfg.SettingsSnapshot.Enabled = false

		apiServer := api.New(cfg, store, pinger, nil, nil, logger,
			nil, alert.NewNullAlertHistoryStore(), nil, nil, nil,
			liveMode, *flagHistory, authMode)
		startServer(ctx, stop, cfg, apiServer, store, logger,
			&orchestrator.NoOpAlertEvaluator{}, &orchestrator.NoOpAlertDispatcher{}, nil)
	} else {
		store = orchestrator.NewLogStore(logger)
		logger.Info("no storage DSN configured, using log-only mode")

		apiServer := api.New(cfg, store, pinger, nil, nil, logger, nil, nil, nil, nil, nil,
			false, 0, authMode)
		startServer(ctx, stop, cfg, apiServer, store, logger,
			&orchestrator.NoOpAlertEvaluator{}, &orchestrator.NoOpAlertDispatcher{}, nil)
	}
}

// seedInstancesFromConfig seeds YAML-configured instances into the database.
// Uses INSERT ON CONFLICT DO NOTHING so existing records are untouched.
func seedInstancesFromConfig(ctx context.Context, store *storage.PGInstanceStore, instances []config.InstanceConfig, logger *slog.Logger) {
	for _, inst := range instances {
		enabled := inst.Enabled == nil || *inst.Enabled
		host, port := extractHostPort(inst.DSN)
		maxConns := inst.MaxConns
		if maxConns == 0 {
			maxConns = 5
		}

		rec := storage.InstanceRecord{
			ID:       inst.ID,
			Name:     inst.Name,
			DSN:      inst.DSN,
			Host:     host,
			Port:     port,
			Enabled:  enabled,
			MaxConns: maxConns,
		}

		if err := store.Seed(ctx, rec); err != nil {
			logger.Warn("failed to seed instance", "id", inst.ID, "error", err)
		} else {
			logger.Debug("seeded instance from config", "id", inst.ID)
		}
	}
}

// extractHostPort parses a postgres URL DSN and returns host and port.
func extractHostPort(dsn string) (string, int) {
	u, err := url.Parse(dsn)
	if err != nil || u.Host == "" {
		return "", 5432
	}
	host := u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		return host, 5432
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return host, 5432
	}
	return host, port
}

// startServer wires the HTTP server and orchestrator, then blocks until shutdown.
func startServer(ctx context.Context, stop context.CancelFunc, cfg config.Config,
	apiServer *api.APIServer, store collector.MetricStore, logger *slog.Logger,
	evaluator orchestrator.AlertEvaluator, dispatcher orchestrator.AlertDispatcher,
	realDispatcher *alert.Dispatcher) {

	orch := orchestrator.New(cfg, store, logger, evaluator, dispatcher)
	apiServer.SetConnProvider(orch)

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
	if realDispatcher != nil {
		logger.Info("stopping alert dispatcher")
		realDispatcher.Stop()
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
	case "", "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level %q", s)
	}
}

// synthesizeCLIInstance builds an InstanceConfig from --target or --target-host CLI flags.
func synthesizeCLIInstance(target, host string, port int, user, password, dbname string) (*config.InstanceConfig, error) {
	dsn := target
	if dsn == "" && host != "" {
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
			url.PathEscape(user), url.PathEscape(password), host, port, dbname)
	}
	if dsn == "" {
		return nil, nil
	}
	h, p := extractHostPort(dsn)
	u, _ := url.Parse(dsn)
	db := ""
	if u != nil {
		db = strings.TrimPrefix(u.Path, "/")
	}
	if db == "" {
		db = "postgres"
	}
	name := fmt.Sprintf("%s:%d/%s", h, p, db)
	enabled := true
	return &config.InstanceConfig{
		ID:       "cli-target",
		Name:     name,
		DSN:      dsn,
		Enabled:  &enabled,
		MaxConns: 5,
	}, nil
}

// noOpAlertEvaluator discards anomaly alerts when alerting is disabled.
type noOpAlertEvaluator struct{}

func (n *noOpAlertEvaluator) Evaluate(_ context.Context, _ string, _ float64, _ map[string]string) error {
	return nil
}
