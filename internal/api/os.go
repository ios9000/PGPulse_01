package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ios9000/PGPulse_01/internal/agent"
	"github.com/ios9000/PGPulse_01/internal/config"
)

// handleOSMetrics returns OS metrics for an instance by scraping its agent.
func (s *APIServer) handleOSMetrics(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	cfg := s.findInstanceConfig(id)
	if cfg == nil {
		writeError(w, http.StatusNotFound, "not_found", "instance not found")
		return
	}

	if cfg.AgentURL == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"available": false,
			"reason":    "OS agent not configured for this instance",
		})
		return
	}

	scraper := agent.NewScraper(cfg.AgentURL)
	snap, err := scraper.ScrapeOS(r.Context())
	if err != nil {
		s.logger.Warn("failed to scrape OS metrics", "instance", id, "error", err)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"available": false,
			"reason":    "OS agent unreachable",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"available": true,
		"data":      snap,
	})
}

// handleClusterMetrics returns cluster (Patroni + ETCD) metrics for an instance.
func (s *APIServer) handleClusterMetrics(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	cfg := s.findInstanceConfig(id)
	if cfg == nil {
		writeError(w, http.StatusNotFound, "not_found", "instance not found")
		return
	}

	if cfg.AgentURL == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"available": false,
			"patroni":   nil,
			"etcd":      nil,
		})
		return
	}

	scraper := agent.NewScraper(cfg.AgentURL)
	snap, err := scraper.ScrapeCluster(r.Context())
	if err != nil {
		s.logger.Warn("failed to scrape cluster metrics", "instance", id, "error", err)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"available": false,
			"patroni":   nil,
			"etcd":      nil,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"available": true,
		"patroni":   snap.Patroni,
		"etcd":      snap.ETCD,
	})
}

// findInstanceConfig finds the config for an instance by ID.
func (s *APIServer) findInstanceConfig(id string) *config.InstanceConfig {
	for i := range s.instances {
		if s.instances[i].ID == id {
			return &s.instances[i]
		}
	}
	return nil
}
