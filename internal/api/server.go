package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ios9000/PGPulse_01/internal/alert"
	"github.com/ios9000/PGPulse_01/internal/auth"
	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/config"
	"github.com/ios9000/PGPulse_01/internal/ml"
	"github.com/ios9000/PGPulse_01/internal/plans"
	"github.com/ios9000/PGPulse_01/internal/settings"
	"github.com/ios9000/PGPulse_01/internal/storage"
)

// Version is set at build time; defaults to "dev".
var Version = "0.1.0-dev"

// Pinger is satisfied by *pgxpool.Pool. Allows mock in tests.
type Pinger interface {
	Ping(ctx context.Context) error
}

// APIServer holds dependencies for all HTTP handlers.
type APIServer struct {
	store             collector.MetricStore
	instances         []config.InstanceConfig
	serverCfg         config.ServerConfig
	authCfg           config.AuthConfig
	jwtService        *auth.JWTService         // nil when auth disabled
	userStore         auth.UserStore            // nil when auth disabled
	rateLimiter       *auth.RateLimiter         // nil when auth disabled
	logger            *slog.Logger
	startTime         time.Time
	pool              Pinger                    // nil when using LogStore
	alertRuleStore    alert.AlertRuleStore      // nil when alerting disabled
	alertHistoryStore alert.AlertHistoryStore   // nil when alerting disabled
	evaluator         *alert.Evaluator          // nil when alerting disabled
	notifierRegistry  *alert.NotifierRegistry   // nil when alerting disabled
	alertingCfg       config.AlertingConfig
	connProvider      InstanceConnProvider      // nil until SetConnProvider called
	instanceStore     storage.InstanceStore     // nil when no storage DSN
	planStore         *plans.PGPlanStore        // nil when plan capture disabled
	snapshotStore     *settings.PGSnapshotStore // nil when settings snapshot disabled
	mlDetector        *ml.Detector              // nil until SetMLDetector called
	mlConfig          config.MLConfig
	liveMode          bool
	memoryRetention   time.Duration
	authMode          auth.AuthMode
}

// New creates an APIServer. jwtSvc and userStore are nil when auth is disabled.
// pool may be nil (LogStore/no-storage mode).
// alertRuleStore, alertHistoryStore, evaluator, and registry are nil when alerting is disabled.
// instanceStore is nil when no storage DSN is configured.
func New(cfg config.Config, store collector.MetricStore, pool Pinger,
	jwtSvc *auth.JWTService, userStore auth.UserStore, logger *slog.Logger,
	alertRuleStore alert.AlertRuleStore, alertHistoryStore alert.AlertHistoryStore,
	evaluator *alert.Evaluator, registry *alert.NotifierRegistry,
	instanceStore storage.InstanceStore,
	liveMode bool, memoryRetention time.Duration, authMode auth.AuthMode,
) *APIServer {
	var rl *auth.RateLimiter
	if cfg.Auth.Enabled && authMode == auth.AuthEnabled {
		rl = auth.NewRateLimiter(10, 15*time.Minute)
	}
	return &APIServer{
		store:             store,
		instances:         cfg.Instances,
		serverCfg:         cfg.Server,
		authCfg:           cfg.Auth,
		jwtService:        jwtSvc,
		userStore:         userStore,
		rateLimiter:       rl,
		logger:            logger,
		startTime:         time.Now(),
		pool:              pool,
		alertRuleStore:    alertRuleStore,
		alertHistoryStore: alertHistoryStore,
		evaluator:         evaluator,
		notifierRegistry:  registry,
		alertingCfg:       cfg.Alerting,
		instanceStore:     instanceStore,
		liveMode:          liveMode,
		memoryRetention:   memoryRetention,
		authMode:          authMode,
	}
}

// SetConnProvider sets the connection provider for live-query endpoints
// (replication, activity). Called from main.go after both API server and
// orchestrator are created.
func (s *APIServer) SetConnProvider(cp InstanceConnProvider) {
	s.connProvider = cp
}

// SetPlanStore sets the plan capture store for plan API endpoints.
func (s *APIServer) SetPlanStore(ps *plans.PGPlanStore) {
	s.planStore = ps
}

// SetSnapshotStore sets the settings snapshot store for settings API endpoints.
func (s *APIServer) SetSnapshotStore(ss *settings.PGSnapshotStore) {
	s.snapshotStore = ss
}

// SetMLDetector sets the ML detector for forecast endpoints.
func (s *APIServer) SetMLDetector(d *ml.Detector, cfg config.MLConfig) {
	s.mlDetector = d
	s.mlConfig = cfg
}

// Routes builds the chi router with all middleware and endpoints.
func (s *APIServer) Routes() http.Handler {
	r := chi.NewRouter()

	r.Use(requestIDMiddleware)
	r.Use(securityHeadersMiddleware)
	r.Use(loggerMiddleware(s.logger))
	r.Use(recovererMiddleware(s.logger))
	if s.serverCfg.CORSEnabled {
		r.Use(corsMiddleware(s.serverCfg.CORSOrigin))
	}

	r.Route("/api/v1", func(r chi.Router) {
		// Public endpoints — no auth required.
		r.Get("/health", s.handleHealth)
		r.Get("/system/mode", s.handleSystemMode)

		if s.authCfg.Enabled {
			// Login is rate-limited and public.
			r.Group(func(r chi.Router) {
				r.Use(s.rateLimitMiddleware)
				r.Post("/auth/login", s.handleLogin)
			})
			// Refresh is public (token is the credential).
			r.Post("/auth/refresh", s.handleRefresh)

			// Protected routes — require a valid access token.
			r.Group(func(r chi.Router) {
				r.Use(auth.RequireAuth(s.jwtService, writeErrorRaw))
				r.Get("/auth/me", s.handleMe)
				r.Get("/instances", s.handleListInstances)
				r.Get("/instances/{id}", s.handleGetInstance)
				r.Get("/instances/{id}/metrics", s.handleQueryMetrics)
				r.Get("/instances/{id}/metrics/current", s.handleCurrentMetrics)
				r.Get("/instances/{id}/metrics/history", s.handleMetricsHistory)
				r.Get("/instances/{id}/metrics/{metric}/forecast", s.handleGetMetricForecast)
				r.Get("/instances/{id}/replication", s.handleReplication)
				r.Get("/instances/{id}/activity/wait-events", s.handleWaitEvents)
				r.Get("/instances/{id}/activity/long-transactions", s.handleLongTransactions)
				r.Get("/instances/{id}/activity/statements", s.handleStatements)
				r.Get("/instances/{id}/activity/locks", s.handleLockTree)
				r.Get("/instances/{id}/activity/progress", s.handleProgress)
				r.Get("/instances/{id}/os", s.handleOSMetrics)
				r.Get("/instances/{id}/cluster", s.handleClusterMetrics)
				r.Get("/instances/{id}/databases", s.handleListDatabases)
				r.Get("/instances/{id}/databases/{dbname}/metrics", s.handleGetDatabaseMetrics)
				r.Get("/instances/{id}/logical-replication", s.handleLogicalReplication)

				// Plan capture routes (M8_02).
				r.Get("/instances/{id}/plans", s.handleListPlans)
				r.Get("/instances/{id}/plans/regressions", s.handleListRegressions)
				r.Get("/instances/{id}/plans/{plan_id}", s.handleGetPlan)

				// Settings snapshot routes (M8_02).
				r.Get("/instances/{id}/settings/history", s.handleSettingsHistory)
				r.Get("/instances/{id}/settings/diff", s.handleSettingsDiff)
				r.Get("/instances/{id}/settings/latest", s.handleSettingsLatest)
				r.Get("/instances/{id}/settings/pending-restart", s.handleSettingsPendingRestart)

				// Plan/settings mutation — require instance_management permission.
				r.Group(func(r chi.Router) {
					r.Use(auth.RequirePermission(auth.PermInstanceManagement, writeErrorRaw))
					r.Post("/instances/{id}/plans/capture", s.handleManualCapture)
					r.Post("/instances/{id}/settings/snapshot", s.handleSettingsManualSnapshot)
					r.Post("/instances/{id}/sessions/{pid}/cancel", s.handleSessionCancel)
					r.Post("/instances/{id}/sessions/{pid}/terminate", s.handleSessionTerminate)
					r.Post("/instances/{id}/explain", s.handleExplain)
				})

				// Alert routes (only when alerting enabled).
				if s.alertRuleStore != nil {
					r.Get("/alerts", s.handleGetActiveAlerts)
					r.Get("/alerts/history", s.handleGetAlertHistory)
					r.Get("/alerts/rules", s.handleGetAlertRules)

					// Alert management — require alert_management permission.
					r.Group(func(r chi.Router) {
						r.Use(auth.RequirePermission(auth.PermAlertManagement, writeErrorRaw))
						r.Post("/alerts/rules", s.handleCreateAlertRule)
						r.Put("/alerts/rules/{id}", s.handleUpdateAlertRule)
						r.Delete("/alerts/rules/{id}", s.handleDeleteAlertRule)
						r.Post("/alerts/test", s.handleTestNotification)
					})
				}

				// Instance management — require instance_management permission.
				if s.instanceStore != nil {
					r.Group(func(r chi.Router) {
						r.Use(auth.RequirePermission(auth.PermInstanceManagement, writeErrorRaw))
						r.Post("/instances", s.handleCreateInstance)
						r.Put("/instances/{id}", s.handleUpdateInstance)
						r.Delete("/instances/{id}", s.handleDeleteInstance)
						r.Post("/instances/bulk", s.handleBulkImport)
						r.Post("/instances/{id}/test", s.handleTestConnection)
					})
				}

				// User management routes — require user_management permission.
				r.Group(func(r chi.Router) {
					r.Use(auth.RequirePermission(auth.PermUserManagement, writeErrorRaw))
					r.Post("/auth/register", s.handleRegister)
					r.Get("/auth/users", s.handleListUsers)
					r.Put("/auth/users/{id}", s.handleUpdateUser)
					r.Delete("/auth/users/{id}", s.handleDeleteUser)
					r.Put("/auth/users/{id}/password", s.handleAdminResetPassword)
				})
				r.Put("/auth/me/password", s.handleChangePassword)
			})
		} else {
			// Auth disabled — stub login/refresh so the frontend doesn't break.
			r.Post("/auth/login", func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, http.StatusOK, map[string]interface{}{
					"access_token":  "disabled",
					"refresh_token": "disabled",
				})
			})
			r.Post("/auth/refresh", func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, http.StatusOK, map[string]interface{}{
					"access_token":  "disabled",
					"refresh_token": "disabled",
				})
			})
			// Inject implicit admin claims for downstream handlers.
			r.Group(func(r chi.Router) {
				r.Use(auth.NewAuthMiddleware(nil, auth.AuthDisabled, writeErrorRaw))
				r.Get("/auth/me", s.handleMe)
				r.Get("/instances", s.handleListInstances)
				r.Get("/instances/{id}", s.handleGetInstance)
				r.Get("/instances/{id}/metrics", s.handleQueryMetrics)
				r.Get("/instances/{id}/metrics/current", s.handleCurrentMetrics)
				r.Get("/instances/{id}/metrics/history", s.handleMetricsHistory)
				r.Get("/instances/{id}/metrics/{metric}/forecast", s.handleGetMetricForecast)
				r.Get("/instances/{id}/replication", s.handleReplication)
				r.Get("/instances/{id}/activity/wait-events", s.handleWaitEvents)
				r.Get("/instances/{id}/activity/long-transactions", s.handleLongTransactions)
				r.Get("/instances/{id}/activity/statements", s.handleStatements)
				r.Get("/instances/{id}/activity/locks", s.handleLockTree)
				r.Get("/instances/{id}/activity/progress", s.handleProgress)
				r.Get("/instances/{id}/os", s.handleOSMetrics)
				r.Get("/instances/{id}/cluster", s.handleClusterMetrics)
				r.Get("/instances/{id}/databases", s.handleListDatabases)
				r.Get("/instances/{id}/databases/{dbname}/metrics", s.handleGetDatabaseMetrics)
				r.Get("/instances/{id}/logical-replication", s.handleLogicalReplication)

				// Plan capture routes (M8_02).
				r.Get("/instances/{id}/plans", s.handleListPlans)
				r.Get("/instances/{id}/plans/regressions", s.handleListRegressions)
				r.Get("/instances/{id}/plans/{plan_id}", s.handleGetPlan)
				r.Post("/instances/{id}/plans/capture", s.handleManualCapture)

				// Settings snapshot routes (M8_02).
				r.Get("/instances/{id}/settings/history", s.handleSettingsHistory)
				r.Get("/instances/{id}/settings/diff", s.handleSettingsDiff)
				r.Get("/instances/{id}/settings/latest", s.handleSettingsLatest)
				r.Get("/instances/{id}/settings/pending-restart", s.handleSettingsPendingRestart)
				r.Post("/instances/{id}/settings/snapshot", s.handleSettingsManualSnapshot)

				// Session kill routes (M8_03).
				r.Post("/instances/{id}/sessions/{pid}/cancel", s.handleSessionCancel)
				r.Post("/instances/{id}/sessions/{pid}/terminate", s.handleSessionTerminate)

				// Explain route (M8_10).
				r.Post("/instances/{id}/explain", s.handleExplain)

				// Alert routes (only when alerting enabled).
				if s.alertRuleStore != nil {
					r.Get("/alerts", s.handleGetActiveAlerts)
					r.Get("/alerts/history", s.handleGetAlertHistory)
					r.Get("/alerts/rules", s.handleGetAlertRules)
					r.Post("/alerts/rules", s.handleCreateAlertRule)
					r.Put("/alerts/rules/{id}", s.handleUpdateAlertRule)
					r.Delete("/alerts/rules/{id}", s.handleDeleteAlertRule)
					r.Post("/alerts/test", s.handleTestNotification)
				}

				// Instance management (no auth check when auth disabled).
				if s.instanceStore != nil {
					r.Post("/instances", s.handleCreateInstance)
					r.Put("/instances/{id}", s.handleUpdateInstance)
					r.Delete("/instances/{id}", s.handleDeleteInstance)
					r.Post("/instances/bulk", s.handleBulkImport)
					r.Post("/instances/{id}/test", s.handleTestConnection)
				}
			})
		}
	})

	// Serve embedded frontend — catch-all after API routes.
	r.Handle("/*", s.staticHandler())

	return r
}

// rateLimitMiddleware checks per-IP rate limits before the login handler.
func (s *APIServer) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.rateLimiter == nil {
			next.ServeHTTP(w, r)
			return
		}

		ip := auth.ClientIP(r)
		if !s.rateLimiter.Allow(ip) {
			retryAfter := s.rateLimiter.RetryAfter(ip)
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many login attempts")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleSystemMode returns the current operating mode (live or persistent).
func (s *APIServer) handleSystemMode(w http.ResponseWriter, _ *http.Request) {
	resp := map[string]any{
		"mode": "persistent",
	}
	if s.liveMode {
		resp["mode"] = "live"
		resp["retention"] = s.memoryRetention.String()
	}
	writeJSON(w, http.StatusOK, resp)
}

// instanceExists reports whether an instance with the given ID is known (DB store or config).
func (s *APIServer) instanceExists(id string) bool {
	if s.instanceStore != nil {
		rec, err := s.instanceStore.Get(context.Background(), id)
		if err == nil && rec != nil {
			return true
		}
	}
	for _, inst := range s.instances {
		if inst.ID == id {
			return true
		}
	}
	return false
}
