package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ios9000/PGPulse_01/internal/auth"
	"github.com/ios9000/PGPulse_01/internal/remediation"
)

// handleListRecommendations returns paginated recommendations for a specific instance.
func (s *APIServer) handleListRecommendations(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "instance not found")
		return
	}

	opts := parseListOpts(r)
	recs, total, err := s.remediationStore.ListByInstance(r.Context(), instanceID, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list recommendations")
		s.logger.Error("list recommendations failed", "instance", instanceID, "error", err)
		return
	}
	if recs == nil {
		recs = []remediation.Recommendation{}
	}
	writeJSON(w, http.StatusOK, Envelope{
		Data: recs,
		Meta: map[string]interface{}{"count": len(recs), "total": total},
	})
}

// handleDiagnose runs all remediation rules against the current metric snapshot.
func (s *APIServer) handleDiagnose(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "instance not found")
		return
	}

	if s.metricSource == nil {
		writeError(w, http.StatusServiceUnavailable, "NO_METRIC_SOURCE", "metric source not configured")
		return
	}

	snapshot, err := s.metricSource.CurrentSnapshot(r.Context(), instanceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to get metric snapshot")
		s.logger.Error("diagnose snapshot failed", "instance", instanceID, "error", err)
		return
	}

	recs := s.remediationEngine.Diagnose(r.Context(), instanceID, snapshot)
	if recs == nil {
		recs = []remediation.Recommendation{}
	}
	writeJSON(w, http.StatusOK, Envelope{
		Data: map[string]interface{}{
			"recommendations":  recs,
			"metrics_evaluated": len(snapshot),
			"rules_evaluated":   len(s.remediationEngine.Rules()),
		},
	})
}

// handleListAllRecommendations returns fleet-wide recommendations.
func (s *APIServer) handleListAllRecommendations(w http.ResponseWriter, r *http.Request) {
	opts := parseListOpts(r)
	recs, total, err := s.remediationStore.ListAll(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list recommendations")
		s.logger.Error("list all recommendations failed", "error", err)
		return
	}
	if recs == nil {
		recs = []remediation.Recommendation{}
	}
	writeJSON(w, http.StatusOK, Envelope{
		Data: recs,
		Meta: map[string]interface{}{"count": len(recs), "total": total},
	})
}

// handleAcknowledgeRecommendation marks a recommendation as acknowledged.
func (s *APIServer) handleAcknowledgeRecommendation(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "id must be an integer")
		return
	}

	username := "unknown"
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil {
		username = claims.Username
	}

	if err := s.remediationStore.Acknowledge(r.Context(), id, username); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "recommendation not found")
		return
	}
	writeJSON(w, http.StatusOK, Envelope{Data: map[string]interface{}{"acknowledged": true, "id": id}})
}

// handleListRemediationRules returns all compiled-in remediation rules.
func (s *APIServer) handleListRemediationRules(w http.ResponseWriter, _ *http.Request) {
	rules := s.remediationEngine.Rules()
	type ruleInfo struct {
		ID       string `json:"id"`
		Priority string `json:"priority"`
		Category string `json:"category"`
	}
	infos := make([]ruleInfo, len(rules))
	for i, r := range rules {
		infos[i] = ruleInfo{
			ID:       r.ID,
			Priority: string(r.Priority),
			Category: string(r.Category),
		}
	}
	writeJSON(w, http.StatusOK, Envelope{
		Data: infos,
		Meta: map[string]interface{}{"count": len(infos)},
	})
}

func parseListOpts(r *http.Request) remediation.ListOpts {
	opts := remediation.ListOpts{
		Priority: r.URL.Query().Get("priority"),
		Category: r.URL.Query().Get("category"),
		Status:   r.URL.Query().Get("status"),
		Source:   r.URL.Query().Get("source"),
		OrderBy:  r.URL.Query().Get("order_by"),
	}

	if ack := r.URL.Query().Get("acknowledged"); ack != "" {
		v := ack == "true"
		opts.Acknowledged = &v
	}

	if incStr := r.URL.Query().Get("incident_id"); incStr != "" {
		if n, err := strconv.ParseInt(incStr, 10, 64); err == nil {
			opts.IncidentID = &n
		}
	}

	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 500 {
		limit = 500
	}
	opts.Limit = limit

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if n, err := strconv.Atoi(offsetStr); err == nil && n >= 0 {
			opts.Offset = n
		}
	}

	return opts
}
