package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ios9000/PGPulse_01/internal/forecast"
)

// handleForecastETA returns ETAs for all active operations on the instance.
func (s *APIServer) handleForecastETA(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.forecastEngine == nil {
		writeError(w, http.StatusNotImplemented, "disabled",
			"forecast subsystem is disabled")
		return
	}

	etas := s.forecastEngine.ETA.ComputeAll(instanceID)
	writeJSON(w, http.StatusOK, Envelope{Data: map[string]any{
		"operations":   etas,
		"evaluated_at": time.Now(),
	}})
}

// handleForecastETAByPID returns ETA for a specific PID.
func (s *APIServer) handleForecastETAByPID(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.forecastEngine == nil {
		writeError(w, http.StatusNotImplemented, "disabled",
			"forecast subsystem is disabled")
		return
	}

	pid, err := strconv.Atoi(chi.URLParam(r, "pid"))
	if err != nil || pid <= 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "pid must be a positive integer")
		return
	}

	eta := s.forecastEngine.ETA.ComputeByPID(instanceID, pid)
	if eta == nil {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("no active operation for PID %d", pid))
		return
	}

	writeJSON(w, http.StatusOK, Envelope{Data: eta})
}

// handleForecastNeeds returns cached maintenance forecasts.
func (s *APIServer) handleForecastNeeds(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.forecastEngine == nil {
		writeError(w, http.StatusNotImplemented, "disabled",
			"forecast subsystem is disabled")
		return
	}

	filter := forecast.ForecastFilter{
		InstanceID: instanceID,
		Operation:  r.URL.Query().Get("operation"),
	}
	if statuses := r.URL.Query().Get("status"); statuses != "" {
		filter.Statuses = strings.Split(statuses, ",")
	}

	forecasts, err := s.forecastEngine.Store.ListForecasts(r.Context(), filter)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "forecast: list forecasts failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to list forecasts")
		return
	}

	summary := computeSummary(forecasts)
	writeJSON(w, http.StatusOK, Envelope{Data: map[string]any{
		"forecasts": forecasts,
		"summary":   summary,
	}})
}

// handleForecastNeedsForTable returns forecasts for a specific table.
func (s *APIServer) handleForecastNeedsForTable(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.forecastEngine == nil {
		writeError(w, http.StatusNotImplemented, "disabled",
			"forecast subsystem is disabled")
		return
	}

	filter := forecast.ForecastFilter{
		InstanceID: instanceID,
		Database:   chi.URLParam(r, "database"),
		Table:      chi.URLParam(r, "table"),
	}

	forecasts, err := s.forecastEngine.Store.ListForecasts(r.Context(), filter)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "forecast: list table forecasts failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to list forecasts")
		return
	}

	writeJSON(w, http.StatusOK, Envelope{Data: map[string]any{
		"forecasts": forecasts,
	}})
}

// handleForecastHistory returns completed operation history with pagination.
func (s *APIServer) handleForecastHistory(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.forecastEngine == nil {
		writeError(w, http.StatusNotImplemented, "disabled",
			"forecast subsystem is disabled")
		return
	}

	page := 1
	perPage := 50
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if pp := r.URL.Query().Get("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 && v <= 200 {
			perPage = v
		}
	}

	filter := forecast.OperationFilter{
		InstanceID: instanceID,
		Operation:  r.URL.Query().Get("operation"),
		Database:   r.URL.Query().Get("database"),
		Table:      r.URL.Query().Get("table"),
		Limit:      perPage,
		Offset:     (page - 1) * perPage,
	}

	ops, total, err := s.forecastEngine.Store.ListOperations(r.Context(), filter)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "forecast: list history failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to list history")
		return
	}

	writeJSON(w, http.StatusOK, Envelope{Data: map[string]any{
		"operations": ops,
		"total":      total,
		"page":       page,
		"per_page":   perPage,
	}})
}

// computeSummary aggregates forecast counts.
func computeSummary(forecasts []forecast.MaintenanceForecast) forecast.ForecastSummary {
	var s forecast.ForecastSummary
	s.TotalTablesEvaluated = len(forecasts)
	for _, f := range forecasts {
		switch f.Status {
		case "imminent":
			s.ImminentCount++
		case "overdue":
			s.OverdueCount++
		case "predicted":
			s.PredictedCount++
		}
	}
	return s
}
