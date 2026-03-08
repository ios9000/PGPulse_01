package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ios9000/PGPulse_01/internal/cluster/etcd"
	"github.com/ios9000/PGPulse_01/internal/cluster/patroni"
)

// ServerConfig holds configuration for the agent HTTP server.
type ServerConfig struct {
	ListenAddr      string
	MountPoints     []string
	PatroniProvider patroni.PatroniProvider // may be nil
	ETCDProvider    etcd.ETCDProvider       // may be nil
}

// Server is the agent HTTP server that exposes OS and cluster metrics.
type Server struct {
	cfg    ServerConfig
	router *chi.Mux
	logger *slog.Logger
}

// NewServer creates a new agent HTTP server with the given config and logger.
func NewServer(cfg ServerConfig, logger *slog.Logger) *Server {
	s := &Server{
		cfg:    cfg,
		router: chi.NewRouter(),
		logger: logger,
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.router.Get("/health", s.handleHealth)
	s.router.Get("/metrics/os", s.handleMetricsOS)
	s.router.Get("/metrics/cluster", s.handleMetricsCluster)
}

// Run starts the HTTP server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:         s.cfg.ListenAddr,
		Handler:      s.router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("agent HTTP server starting", "addr", s.cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	s.logger.Info("shutting down agent HTTP server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	s.logger.Info("agent HTTP server stopped")
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]string{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error("failed to write health response", "error", err)
	}
}

func (s *Server) handleMetricsOS(w http.ResponseWriter, _ *http.Request) {
	snap, err := CollectOS(s.cfg.MountPoints)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"available": false,
			"reason":    err.Error(),
		}
		if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
			s.logger.Error("failed to write OS unavailable response", "error", encErr)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(snap); err != nil {
		s.logger.Error("failed to write OS metrics response", "error", err)
	}
}

func (s *Server) handleMetricsCluster(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	snap := &ClusterSnapshot{}

	if s.cfg.PatroniProvider != nil {
		state, err := s.cfg.PatroniProvider.GetClusterState(ctx)
		if err != nil {
			s.logger.Warn("failed to get patroni cluster state", "error", err)
		} else {
			snap.Patroni = state
		}
	}

	if s.cfg.ETCDProvider != nil {
		members, err := s.cfg.ETCDProvider.GetMembers(ctx)
		if err != nil {
			s.logger.Warn("failed to get etcd members", "error", err)
		} else {
			snap.ETCD = members
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(snap); err != nil {
		s.logger.Error("failed to write cluster metrics response", "error", err)
	}
}
