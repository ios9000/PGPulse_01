package api

import (
	"context"
	"net/http"
	"time"
)

// HealthResponse is the JSON body returned by GET /api/v1/health.
type HealthResponse struct {
	Status  string `json:"status"`  // "ok" or "error"
	Storage string `json:"storage"` // "ok", "error", or "disabled"
	Uptime  string `json:"uptime"`  // e.g. "2h35m12s"
	Version string `json:"version"` // e.g. "0.1.0-dev"
}

func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Status:  "ok",
		Uptime:  time.Since(s.startTime).Truncate(time.Second).String(),
		Version: Version,
	}

	if s.pool == nil {
		resp.Storage = "disabled"
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := s.pool.Ping(ctx); err != nil {
			resp.Storage = "error"
			resp.Status = "error"
		} else {
			resp.Storage = "ok"
		}
	}

	statusCode := http.StatusOK
	if resp.Status == "error" {
		statusCode = http.StatusServiceUnavailable
	}
	writeJSON(w, statusCode, resp)
}
