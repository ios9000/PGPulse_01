package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ios9000/PGPulse_01/internal/ml"
)

// handleGetMetricForecast returns forecast points for a metric on an instance.
// GET /instances/{id}/metrics/{metric}/forecast?horizon=N
func (s *APIServer) handleGetMetricForecast(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	metric := chi.URLParam(r, "metric")

	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.mlDetector == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "ML detector not configured")
		return
	}

	// Determine horizon: query param > per-metric config > global config > default 60.
	horizon := s.defaultForecastHorizon(metric)
	if hStr := r.URL.Query().Get("horizon"); hStr != "" {
		h, err := strconv.Atoi(hStr)
		if err != nil || h <= 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "horizon must be a positive integer")
			return
		}
		// Cap at 2x global horizon or 240, whichever is larger.
		capVal := s.mlConfig.Forecast.Horizon * 2
		if capVal < 240 {
			capVal = 240
		}
		if h > capVal {
			h = capVal
		}
		horizon = h
	}

	result, err := s.mlDetector.Forecast(r.Context(), instanceID, metric, horizon)
	if err != nil {
		if errors.Is(err, ml.ErrNotBootstrapped) {
			writeError(w, http.StatusServiceUnavailable, "not_ready",
				"ML detector not yet bootstrapped")
			return
		}
		if errors.Is(err, ml.ErrNoBaseline) {
			writeError(w, http.StatusNotFound, "no_baseline",
				"no fitted baseline for this metric")
			return
		}
		s.logger.ErrorContext(r.Context(), "forecast failed",
			"instance_id", instanceID, "metric", metric, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "forecast failed")
		return
	}

	writeJSON(w, http.StatusOK, Envelope{Data: result})
}

// defaultForecastHorizon returns the forecast horizon for a metric,
// checking per-metric config first, then global default.
func (s *APIServer) defaultForecastHorizon(metric string) int {
	for _, mc := range s.mlConfig.Metrics {
		if mc.Key == metric && mc.ForecastHorizon > 0 {
			return mc.ForecastHorizon
		}
	}
	if s.mlConfig.Forecast.Horizon > 0 {
		return s.mlConfig.Forecast.Horizon
	}
	return 60 // fallback default
}
