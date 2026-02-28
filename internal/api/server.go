package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

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
	store     collector.MetricStore
	instances []config.InstanceConfig
	serverCfg config.ServerConfig
	logger    *slog.Logger
	startTime time.Time
	pool      Pinger // nil when using LogStore
}

// New creates an APIServer. pool may be nil (LogStore/no-storage mode).
func New(cfg config.Config, store collector.MetricStore, pool Pinger, logger *slog.Logger) *APIServer {
	return &APIServer{
		store:     store,
		instances: cfg.Instances,
		serverCfg: cfg.Server,
		logger:    logger,
		startTime: time.Now(),
		pool:      pool,
	}
}

// Routes builds the chi router with all middleware and endpoints.
func (s *APIServer) Routes() http.Handler {
	r := chi.NewRouter()

	r.Use(requestIDMiddleware)
	r.Use(loggerMiddleware(s.logger))
	r.Use(recovererMiddleware(s.logger))
	if s.serverCfg.CORSEnabled {
		r.Use(corsMiddleware)
	}
	r.Use(authStubMiddleware)

	r.Get("/api/v1/health", s.handleHealth)
	r.Get("/api/v1/instances", s.handleListInstances)
	r.Get("/api/v1/instances/{id}", s.handleGetInstance)
	r.Get("/api/v1/instances/{id}/metrics", s.handleQueryMetrics)

	return r
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
