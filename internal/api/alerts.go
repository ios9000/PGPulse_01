package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ios9000/PGPulse_01/internal/alert"
)

type createRuleRequest struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Metric           string            `json:"metric"`
	Operator         alert.Operator    `json:"operator"`
	Threshold        float64           `json:"threshold"`
	Severity         alert.Severity    `json:"severity"`
	Labels           map[string]string `json:"labels"`
	ConsecutiveCount int               `json:"consecutive_count"`
	CooldownMinutes  int               `json:"cooldown_minutes"`
	Channels         []string          `json:"channels"`
	Enabled          *bool             `json:"enabled"`
}

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func validateRuleRequest(req createRuleRequest) error {
	if req.ID == "" {
		return fmt.Errorf("id is required")
	}
	if !slugPattern.MatchString(req.ID) {
		return fmt.Errorf("id must be lowercase alphanumeric with dashes/underscores")
	}
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if req.Metric == "" {
		return fmt.Errorf("metric is required")
	}
	if !validOperator(req.Operator) {
		return fmt.Errorf("operator must be one of: >, >=, <, <=, ==, !=")
	}
	if req.Severity == "" {
		return fmt.Errorf("severity is required")
	}
	if !validSeverity(req.Severity) {
		return fmt.Errorf("severity must be one of: info, warning, critical")
	}
	return nil
}

func validOperator(op alert.Operator) bool {
	switch op {
	case alert.OpGreater, alert.OpGreaterEqual, alert.OpLess, alert.OpLessEqual, alert.OpEqual, alert.OpNotEqual:
		return true
	default:
		return false
	}
}

func validSeverity(s alert.Severity) bool {
	switch s {
	case alert.SeverityInfo, alert.SeverityWarning, alert.SeverityCritical:
		return true
	default:
		return false
	}
}

// handleGetActiveAlerts returns all unresolved alert events.
func (s *APIServer) handleGetActiveAlerts(w http.ResponseWriter, r *http.Request) {
	events, err := s.alertHistoryStore.ListUnresolved(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list active alerts")
		s.logger.Error("list active alerts failed", "error", err)
		return
	}
	writeJSON(w, http.StatusOK, Envelope{
		Data: events,
		Meta: map[string]interface{}{"count": len(events)},
	})
}

// handleGetAlertHistory returns filtered alert history.
func (s *APIServer) handleGetAlertHistory(w http.ResponseWriter, r *http.Request) {
	q := alert.AlertHistoryQuery{}

	q.InstanceID = r.URL.Query().Get("instance_id")
	q.RuleID = r.URL.Query().Get("rule_id")

	if sev := r.URL.Query().Get("severity"); sev != "" {
		if !validSeverity(alert.Severity(sev)) {
			writeError(w, http.StatusBadRequest, "INVALID_SEVERITY", "severity must be info, warning, or critical")
			return
		}
		q.Severity = alert.Severity(sev)
	}

	if startStr := r.URL.Query().Get("start"); startStr != "" {
		t, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_TIME", "start must be RFC3339 format")
			return
		}
		q.Start = t
	}

	if endStr := r.URL.Query().Get("end"); endStr != "" {
		t, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_TIME", "end must be RFC3339 format")
			return
		}
		q.End = t
	}

	if r.URL.Query().Get("unresolved") == "true" {
		q.UnresolvedOnly = true
	}

	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		n, err := strconv.Atoi(limitStr)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "INVALID_LIMIT", "limit must be a positive integer")
			return
		}
		if n > 1000 {
			n = 1000
		}
		limit = n
	}
	q.Limit = limit

	events, err := s.alertHistoryStore.Query(r.Context(), q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to query alert history")
		s.logger.Error("query alert history failed", "error", err)
		return
	}
	writeJSON(w, http.StatusOK, Envelope{
		Data: events,
		Meta: map[string]interface{}{"count": len(events)},
	})
}

// handleGetAlertRules returns all alert rules.
func (s *APIServer) handleGetAlertRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.alertRuleStore.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list alert rules")
		s.logger.Error("list alert rules failed", "error", err)
		return
	}
	writeJSON(w, http.StatusOK, Envelope{
		Data: rules,
		Meta: map[string]interface{}{"count": len(rules)},
	})
}

// handleCreateAlertRule creates a new custom alert rule.
func (s *APIServer) handleCreateAlertRule(w http.ResponseWriter, r *http.Request) {
	var req createRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	if err := validateRuleRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	consecutiveCount := req.ConsecutiveCount
	if consecutiveCount <= 0 {
		consecutiveCount = s.alertingCfg.DefaultConsecutiveCount
		if consecutiveCount <= 0 {
			consecutiveCount = 3
		}
	}

	cooldownMinutes := req.CooldownMinutes
	if cooldownMinutes <= 0 {
		cooldownMinutes = s.alertingCfg.DefaultCooldownMinutes
		if cooldownMinutes <= 0 {
			cooldownMinutes = 15
		}
	}

	rule := alert.Rule{
		ID:               req.ID,
		Name:             req.Name,
		Description:      req.Description,
		Metric:           req.Metric,
		Operator:         req.Operator,
		Threshold:        req.Threshold,
		Severity:         req.Severity,
		Labels:           req.Labels,
		ConsecutiveCount: consecutiveCount,
		CooldownMinutes:  cooldownMinutes,
		Channels:         req.Channels,
		Source:           alert.SourceCustom,
		Enabled:          enabled,
	}

	if err := s.alertRuleStore.Create(r.Context(), &rule); err != nil {
		writeError(w, http.StatusConflict, "DUPLICATE_ID", fmt.Sprintf("rule %q already exists", rule.ID))
		return
	}

	if s.evaluator != nil {
		if err := s.evaluator.LoadRules(r.Context()); err != nil {
			s.logger.Error("failed to reload rules after create", "error", err)
		}
	}

	writeJSON(w, http.StatusCreated, Envelope{Data: rule})
}

// handleUpdateAlertRule updates an existing alert rule.
func (s *APIServer) handleUpdateAlertRule(w http.ResponseWriter, r *http.Request) {
	ruleID := chi.URLParam(r, "id")

	existing, err := s.alertRuleStore.Get(r.Context(), ruleID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("rule %q not found", ruleID))
		return
	}

	var req createRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	if existing.Source == alert.SourceBuiltin {
		// Builtin rules: only threshold, consecutive_count, cooldown_minutes, enabled, channels.
		if req.Threshold != 0 {
			existing.Threshold = req.Threshold
		}
		if req.ConsecutiveCount > 0 {
			existing.ConsecutiveCount = req.ConsecutiveCount
		}
		if req.CooldownMinutes > 0 {
			existing.CooldownMinutes = req.CooldownMinutes
		}
		if req.Enabled != nil {
			existing.Enabled = *req.Enabled
		}
		if req.Channels != nil {
			existing.Channels = req.Channels
		}
	} else {
		// Custom rules: all fields.
		if req.Name != "" {
			existing.Name = req.Name
		}
		if req.Description != "" {
			existing.Description = req.Description
		}
		if req.Metric != "" {
			existing.Metric = req.Metric
		}
		if req.Operator != "" {
			if !validOperator(req.Operator) {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "operator must be one of: >, >=, <, <=, ==, !=")
				return
			}
			existing.Operator = req.Operator
		}
		if req.Threshold != 0 {
			existing.Threshold = req.Threshold
		}
		if req.Severity != "" {
			if !validSeverity(req.Severity) {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "severity must be info, warning, or critical")
				return
			}
			existing.Severity = req.Severity
		}
		if req.Labels != nil {
			existing.Labels = req.Labels
		}
		if req.ConsecutiveCount > 0 {
			existing.ConsecutiveCount = req.ConsecutiveCount
		}
		if req.CooldownMinutes > 0 {
			existing.CooldownMinutes = req.CooldownMinutes
		}
		if req.Enabled != nil {
			existing.Enabled = *req.Enabled
		}
		if req.Channels != nil {
			existing.Channels = req.Channels
		}
	}

	if err := s.alertRuleStore.Update(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to update rule")
		s.logger.Error("update alert rule failed", "rule_id", ruleID, "error", err)
		return
	}

	if s.evaluator != nil {
		if err := s.evaluator.LoadRules(r.Context()); err != nil {
			s.logger.Error("failed to reload rules after update", "error", err)
		}
	}

	writeJSON(w, http.StatusOK, Envelope{Data: existing})
}

// handleDeleteAlertRule deletes a custom alert rule. Builtin rules cannot be deleted.
func (s *APIServer) handleDeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	ruleID := chi.URLParam(r, "id")

	existing, err := s.alertRuleStore.Get(r.Context(), ruleID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("rule %q not found", ruleID))
		return
	}

	if existing.Source == alert.SourceBuiltin {
		writeError(w, http.StatusConflict, "BUILTIN_RULE", "builtin rules cannot be deleted; disable them instead")
		return
	}

	if err := s.alertRuleStore.Delete(r.Context(), ruleID); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to delete rule")
		s.logger.Error("delete alert rule failed", "rule_id", ruleID, "error", err)
		return
	}

	if s.evaluator != nil {
		if err := s.evaluator.LoadRules(r.Context()); err != nil {
			s.logger.Error("failed to reload rules after delete", "error", err)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

type testNotificationRequest struct {
	Channel string `json:"channel"`
	Message string `json:"message"`
}

// handleTestNotification sends a test notification to a specified channel.
func (s *APIServer) handleTestNotification(w http.ResponseWriter, r *http.Request) {
	var req testNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	if req.Channel == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "channel is required")
		return
	}

	if s.notifierRegistry == nil {
		writeError(w, http.StatusServiceUnavailable, "NO_NOTIFIERS", "no notifier registry configured")
		return
	}

	n := s.notifierRegistry.Get(req.Channel)
	if n == nil {
		writeError(w, http.StatusBadRequest, "UNKNOWN_CHANNEL", fmt.Sprintf("channel %q is not registered", req.Channel))
		return
	}

	testEvent := alert.AlertEvent{
		RuleID:     "pgpulse.test",
		RuleName:   "Test Notification",
		InstanceID: "test",
		Severity:   alert.SeverityInfo,
		Metric:     "pgpulse.test",
		FiredAt:    time.Now(),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := n.Send(ctx, testEvent); err != nil {
		writeError(w, http.StatusBadGateway, "SEND_FAILED", fmt.Sprintf("failed to send test notification: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, Envelope{
		Data: map[string]interface{}{"sent": true, "channel": req.Channel},
	})
}
