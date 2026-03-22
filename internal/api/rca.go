package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ios9000/PGPulse_01/internal/rca"
)

// rcaAnalyzeRequest is the JSON body for POST /instances/{id}/rca/analyze.
type rcaAnalyzeRequest struct {
	Metric        string  `json:"metric"`
	Value         float64 `json:"value"`
	Timestamp     string  `json:"timestamp"`
	WindowMinutes int     `json:"window_minutes"`
}

// handleRCAAnalyze triggers an RCA analysis for a specific metric trigger.
func (s *APIServer) handleRCAAnalyze(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "instance not found")
		return
	}

	var req rcaAnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}
	if req.Metric == "" {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "metric is required")
		return
	}

	triggerTime := time.Now()
	if req.Timestamp != "" {
		parsed, err := time.Parse(time.RFC3339, req.Timestamp)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_BODY", "timestamp must be RFC3339")
			return
		}
		triggerTime = parsed
	}

	windowMinutes := req.WindowMinutes
	if windowMinutes == 0 {
		windowMinutes = 30
	}

	incident, err := s.rcaEngine.Analyze(r.Context(), rca.AnalyzeRequest{
		InstanceID:    instanceID,
		TriggerMetric: req.Metric,
		TriggerValue:  req.Value,
		TriggerTime:   triggerTime,
		WindowMinutes: windowMinutes,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RCA_ERROR", "analysis failed")
		s.logger.Error("rca analyze failed", "instance", instanceID, "metric", req.Metric, "error", err)
		return
	}
	writeJSON(w, http.StatusOK, Envelope{Data: incident})
}

// handleRCAListIncidents returns paginated RCA incidents for a specific instance.
func (s *APIServer) handleRCAListIncidents(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "instance not found")
		return
	}

	limit, offset := parseRCAPagination(r)
	incidents, total, err := s.rcaStore.ListByInstance(r.Context(), instanceID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list incidents")
		s.logger.Error("list rca incidents failed", "instance", instanceID, "error", err)
		return
	}
	if incidents == nil {
		incidents = []rca.Incident{}
	}
	writeJSON(w, http.StatusOK, Envelope{
		Data: map[string]interface{}{
			"incidents": incidents,
			"total":     total,
		},
	})
}

// handleRCAGetIncident returns a single RCA incident by ID.
func (s *APIServer) handleRCAGetIncident(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "instance not found")
		return
	}

	incidentIDStr := chi.URLParam(r, "incidentId")
	incidentID, err := strconv.ParseInt(incidentIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "incidentId must be an integer")
		return
	}

	incident, err := s.rcaStore.Get(r.Context(), incidentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to get incident")
		s.logger.Error("get rca incident failed", "id", incidentID, "error", err)
		return
	}
	if incident == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "incident not found")
		return
	}
	writeJSON(w, http.StatusOK, Envelope{Data: incident})
}

// handleRCAListAllIncidents returns fleet-wide paginated RCA incidents.
func (s *APIServer) handleRCAListAllIncidents(w http.ResponseWriter, r *http.Request) {
	limit, offset := parseRCAPagination(r)
	incidents, total, err := s.rcaStore.ListAll(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list incidents")
		s.logger.Error("list all rca incidents failed", "error", err)
		return
	}
	if incidents == nil {
		incidents = []rca.Incident{}
	}
	writeJSON(w, http.StatusOK, Envelope{
		Data: map[string]interface{}{
			"incidents": incidents,
			"total":     total,
		},
	})
}

// handleRCAGetGraph returns the causal graph definition (nodes, edges, chain IDs).
func (s *APIServer) handleRCAGetGraph(w http.ResponseWriter, _ *http.Request) {
	graph := s.rcaEngine.Graph()

	// Populate seconds fields from duration for JSON serialization.
	edges := make([]rca.CausalEdge, len(graph.Edges))
	copy(edges, graph.Edges)
	for i := range edges {
		edges[i].MinLagSeconds = edges[i].MinLag.Seconds()
		edges[i].MaxLagSeconds = edges[i].MaxLag.Seconds()
	}

	writeJSON(w, http.StatusOK, Envelope{
		Data: map[string]interface{}{
			"nodes":     graph.Nodes,
			"edges":     edges,
			"chain_ids": graph.ChainIDs,
		},
	})
}

// rcaReviewRequest is the JSON body for PUT /rca/incidents/{incidentId}/review.
type rcaReviewRequest struct {
	Status  string `json:"status"`
	Comment string `json:"comment"`
}

// handleRCAReviewIncident updates the review status and comment for an incident.
func (s *APIServer) handleRCAReviewIncident(w http.ResponseWriter, r *http.Request) {
	incidentIDStr := chi.URLParam(r, "incidentId")
	incidentID, err := strconv.ParseInt(incidentIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "incidentId must be an integer")
		return
	}

	var req rcaReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}
	if req.Status == "" {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "status is required")
		return
	}

	if err := s.rcaStore.UpdateReview(r.Context(), incidentID, req.Status, req.Comment); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "incident not found or update failed")
		s.logger.Error("rca review failed", "id", incidentID, "error", err)
		return
	}

	writeJSON(w, http.StatusOK, Envelope{Data: map[string]interface{}{
		"reviewed":    true,
		"incident_id": incidentID,
		"status":      req.Status,
	}})
}

// parseRCAPagination extracts limit and offset from query params with defaults.
func parseRCAPagination(r *http.Request) (int, int) {
	limit := 20
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 500 {
		limit = 500
	}

	offset := 0
	if s := r.URL.Query().Get("offset"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			offset = n
		}
	}

	return limit, offset
}
