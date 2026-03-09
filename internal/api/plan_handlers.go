package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/plans"
)

// handleListPlans returns captured plans for an instance.
// GET /instances/{id}/plans?fingerprint=&since=&trigger=
func (s *APIServer) handleListPlans(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.planStore == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "plan capture storage not configured")
		return
	}

	fingerprint := r.URL.Query().Get("fingerprint")
	triggerType := r.URL.Query().Get("trigger")
	var since *time.Time
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		t, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_param", "since must be RFC3339 format")
			return
		}
		since = &t
	}

	result, err := s.planStore.ListPlans(r.Context(), instanceID, fingerprint, since, triggerType)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to list plans", "instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to list plans")
		return
	}

	writeJSON(w, http.StatusOK, Envelope{
		Data: result,
		Meta: map[string]int{"count": len(result)},
	})
}

// handleGetPlan returns a single plan by ID including full plan text.
// GET /instances/{id}/plans/{plan_id}
func (s *APIServer) handleGetPlan(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.planStore == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "plan capture storage not configured")
		return
	}

	planIDStr := chi.URLParam(r, "plan_id")
	planID, err := strconv.ParseInt(planIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_param", "plan_id must be an integer")
		return
	}

	plan, err := s.planStore.GetPlan(r.Context(), planID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "plan not found")
			return
		}
		s.logger.ErrorContext(r.Context(), "failed to get plan", "plan_id", planID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get plan")
		return
	}

	if plan.InstanceID != instanceID {
		writeError(w, http.StatusNotFound, "not_found", "plan not found for this instance")
		return
	}

	writeJSON(w, http.StatusOK, Envelope{Data: plan})
}

// handleListRegressions returns plan hash changes (regressions) for an instance.
// GET /instances/{id}/plans/regressions
func (s *APIServer) handleListRegressions(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.planStore == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "plan capture storage not configured")
		return
	}

	result, err := s.planStore.ListRegressions(r.Context(), instanceID)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to list regressions", "instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to list regressions")
		return
	}

	writeJSON(w, http.StatusOK, Envelope{
		Data: result,
		Meta: map[string]int{"count": len(result)},
	})
}

// manualCaptureRequest is the request body for manual plan capture.
type manualCaptureRequest struct {
	Query    string `json:"query"`
	Database string `json:"database"`
}

// handleManualCapture triggers a manual EXPLAIN capture for a query.
// POST /instances/{id}/plans/capture
func (s *APIServer) handleManualCapture(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.planStore == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "plan capture storage not configured")
		return
	}

	if s.connProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "connection provider not available")
		return
	}

	var req manualCaptureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
		return
	}
	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "invalid_body", "query is required")
		return
	}

	conn, err := s.connProvider.ConnFor(r.Context(), instanceID)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get connection for manual capture",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to connect to instance")
		return
	}
	defer func() { _ = conn.Close(r.Context()) }()

	// Run EXPLAIN (FORMAT JSON) on the query.
	var planJSON string
	err = conn.QueryRow(r.Context(),
		"EXPLAIN (FORMAT JSON) "+req.Query).Scan(&planJSON)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "EXPLAIN failed",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusBadRequest, "explain_failed",
			fmt.Sprintf("EXPLAIN failed: %v", err))
		return
	}

	capture := plans.PlanCapture{
		InstanceID:       instanceID,
		DatabaseName:     req.Database,
		QueryFingerprint: "manual",
		PlanHash:         fmt.Sprintf("%x", time.Now().UnixNano()),
		PlanText:         planJSON,
		TriggerType:      plans.TriggerManual,
		QueryText:        req.Query,
		CapturedAt:       time.Now(),
	}

	if err := s.planStore.SavePlan(r.Context(), capture); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to save manual capture",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to save plan")
		return
	}

	writeJSON(w, http.StatusOK, Envelope{
		Data: map[string]any{
			"plan_json": planJSON,
			"query":     req.Query,
			"database":  req.Database,
		},
	})
}
