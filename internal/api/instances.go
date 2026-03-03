package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ios9000/PGPulse_01/internal/alert"
	"github.com/ios9000/PGPulse_01/internal/config"
)

// InstanceResponse is the JSON representation of a monitored PostgreSQL instance.
type InstanceResponse struct {
	ID          string `json:"id"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
}

// EnrichedInstanceResponse extends InstanceResponse with optional metrics and alert counts.
type EnrichedInstanceResponse struct {
	InstanceResponse
	Metrics     map[string]float64 `json:"metrics,omitempty"`
	AlertCounts map[string]int     `json:"alert_counts,omitempty"` // severity -> count
}

func (s *APIServer) handleListInstances(w http.ResponseWriter, r *http.Request) {
	includes := r.URL.Query()["include"]
	includeMetrics := containsStr(includes, "metrics")
	includeAlerts := containsStr(includes, "alerts")

	// If no enrichment requested, use the fast path.
	if !includeMetrics && !includeAlerts {
		items := make([]InstanceResponse, 0, len(s.instances))
		for _, inst := range s.instances {
			items = append(items, toInstanceResponse(inst))
		}
		writeJSON(w, http.StatusOK, Envelope{
			Data: items,
			Meta: map[string]int{"count": len(items)},
		})
		return
	}

	// Build per-instance alert counts if requested.
	var alertCountsByInstance map[string]map[string]int
	if includeAlerts && s.alertHistoryStore != nil {
		events, err := s.alertHistoryStore.ListUnresolved(r.Context())
		if err != nil {
			s.logger.ErrorContext(r.Context(), "failed to list unresolved alerts for enrichment", "error", err)
		} else {
			alertCountsByInstance = buildAlertCounts(events)
		}
	}

	items := make([]EnrichedInstanceResponse, 0, len(s.instances))
	for _, inst := range s.instances {
		enriched := EnrichedInstanceResponse{
			InstanceResponse: toInstanceResponse(inst),
		}

		if includeMetrics {
			mq := s.metricsQuerier()
			if mq != nil {
				vals, err := mq.CurrentMetricValues(r.Context(), inst.ID)
				if err != nil {
					s.logger.ErrorContext(r.Context(), "failed to get metrics for instance",
						"instance_id", inst.ID, "error", err)
				} else if len(vals) > 0 {
					enriched.Metrics = vals
				}
			}
		}

		if includeAlerts && alertCountsByInstance != nil {
			if counts, ok := alertCountsByInstance[inst.ID]; ok {
				enriched.AlertCounts = counts
			}
		}

		items = append(items, enriched)
	}

	writeJSON(w, http.StatusOK, Envelope{
		Data: items,
		Meta: map[string]int{"count": len(items)},
	})
}

// buildAlertCounts groups unresolved alert events by instance ID and severity.
func buildAlertCounts(events []alert.AlertEvent) map[string]map[string]int {
	result := make(map[string]map[string]int)
	for _, ev := range events {
		if _, ok := result[ev.InstanceID]; !ok {
			result[ev.InstanceID] = make(map[string]int)
		}
		result[ev.InstanceID][string(ev.Severity)]++
	}
	return result
}

// containsStr checks if a string slice contains a given value.
func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func (s *APIServer) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	for _, inst := range s.instances {
		if inst.ID == id {
			writeJSON(w, http.StatusOK, Envelope{Data: toInstanceResponse(inst)})
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found",
		fmt.Sprintf("instance '%s' not found", id))
}

// toInstanceResponse maps config.InstanceConfig to the API response shape.
// Host and port are derived from the DSN (postgres://... URL format).
func toInstanceResponse(ic config.InstanceConfig) InstanceResponse {
	host, port := extractHostPort(ic.DSN)
	enabled := ic.Enabled == nil || *ic.Enabled
	return InstanceResponse{
		ID:          ic.ID,
		Host:        host,
		Port:        port,
		Description: ic.Description,
		Enabled:     enabled,
	}
}

// extractHostPort parses a postgres URL DSN and returns host and port.
// Returns empty string and 0 if the DSN cannot be parsed or is not URL format.
func extractHostPort(dsn string) (string, int) {
	u, err := url.Parse(dsn)
	if err != nil || u.Host == "" {
		return "", 0
	}
	host := u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		return host, 5432
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return host, 5432
	}
	return host, port
}
