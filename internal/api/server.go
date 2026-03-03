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
}

// New creates an APIServer. jwtSvc and userStore are nil when auth is disabled.
// pool may be nil (LogStore/no-storage mode).
// alertRuleStore, alertHistoryStore, evaluator, and registry are nil when alerting is disabled.
func New(cfg config.Config, store collector.MetricStore, pool Pinger,
	jwtSvc *auth.JWTService, userStore auth.UserStore, logger *slog.Logger,
	alertRuleStore alert.AlertRuleStore, alertHistoryStore alert.AlertHistoryStore,
	evaluator *alert.Evaluator, registry *alert.NotifierRegistry,
) *APIServer {
	var rl *auth.RateLimiter
	if cfg.Auth.Enabled {
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
	}
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
		// Health is always public.
		r.Get("/health", s.handleHealth)

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

				// User management routes — require user_management permission.
				r.Group(func(r chi.Router) {
					r.Use(auth.RequirePermission(auth.PermUserManagement, writeErrorRaw))
					r.Post("/auth/register", s.handleRegister)
					r.Get("/auth/users", s.handleListUsers)
					r.Put("/auth/users/{id}", s.handleUpdateUser)
				})
				r.Put("/auth/me/password", s.handleChangePassword)
			})
		} else {
			// Auth disabled — preserve current open behaviour.
			r.Group(func(r chi.Router) {
				r.Use(authStubMiddleware)
				r.Get("/instances", s.handleListInstances)
				r.Get("/instances/{id}", s.handleGetInstance)
				r.Get("/instances/{id}/metrics", s.handleQueryMetrics)

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

// instanceExists reports whether an instance with the given ID is configured.
func (s *APIServer) instanceExists(id string) bool {
	for _, inst := range s.instances {
		if inst.ID == id {
			return true
		}
	}
	return false
}
