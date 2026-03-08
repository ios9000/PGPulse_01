package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/storage"
)

// MetricsQuerier provides access to advanced metric query methods.
type MetricsQuerier interface {
	CurrentMetrics(ctx context.Context, instanceID string) (*storage.CurrentMetricsResult, error)
	HistoryMetrics(ctx context.Context, req storage.HistoryRequest) (*storage.HistoryResult, error)
	CurrentMetricValues(ctx context.Context, instanceID string) (map[string]float64, error)
}

func (s *APIServer) handleQueryMetrics(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")

	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	q, err := parseMetricQuery(r, instanceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	points, err := s.store.Query(r.Context(), q)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "metrics query failed",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to query metrics")
		return
	}

	if resolveFormat(r) == "csv" {
		writeCSV(w, points)
		return
	}

	writeJSON(w, http.StatusOK, Envelope{
		Data: points,
		Meta: map[string]any{
			"count":       len(points),
			"instance_id": instanceID,
			"query": map[string]any{
				"metric": q.Metric,
				"start":  q.Start.Format(time.RFC3339),
				"end":    q.End.Format(time.RFC3339),
				"limit":  q.Limit,
			},
		},
	})
}

// parseMetricQuery extracts and validates metric query parameters from the request.
func parseMetricQuery(r *http.Request, instanceID string) (collector.MetricQuery, error) {
	now := time.Now()
	q := collector.MetricQuery{
		InstanceID: instanceID,
		Metric:     r.URL.Query().Get("metric"),
		Start:      now.Add(-1 * time.Hour),
		End:        now,
		Limit:      1000,
	}

	if s := r.URL.Query().Get("start"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return q, fmt.Errorf("invalid 'start' parameter: must be RFC3339")
		}
		q.Start = t
	}

	if s := r.URL.Query().Get("end"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return q, fmt.Errorf("invalid 'end' parameter: must be RFC3339")
		}
		q.End = t
	}

	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 10000 {
			return q, fmt.Errorf("invalid 'limit' parameter: must be 1-10000")
		}
		q.Limit = n
	}

	return q, nil
}

// resolveFormat checks ?format= param first, then Accept header.
func resolveFormat(r *http.Request) string {
	if f := r.URL.Query().Get("format"); f == "csv" {
		return "csv"
	}
	if strings.Contains(r.Header.Get("Accept"), "text/csv") {
		return "csv"
	}
	return "json"
}

// writeCSV writes metric points as a CSV response with attachment disposition.
func writeCSV(w http.ResponseWriter, points []collector.MetricPoint) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="metrics.csv"`)
	w.WriteHeader(http.StatusOK)

	cw := csv.NewWriter(w)
	defer cw.Flush()

	_ = cw.Write([]string{"instance_id", "metric", "value", "labels", "timestamp"})
	for _, p := range points {
		labelsJSON, _ := json.Marshal(p.Labels)
		_ = cw.Write([]string{
			p.InstanceID,
			p.Metric,
			strconv.FormatFloat(p.Value, 'f', 6, 64),
			string(labelsJSON),
			p.Timestamp.Format(time.RFC3339),
		})
	}
}

// metricsQuerier attempts to obtain a MetricsQuerier from the store via type assertion.
func (s *APIServer) metricsQuerier() MetricsQuerier {
	if mq, ok := s.store.(MetricsQuerier); ok {
		return mq
	}
	return nil
}

// handleCurrentMetrics returns the latest value of each metric for an instance.
func (s *APIServer) handleCurrentMetrics(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")

	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	mq := s.metricsQuerier()
	if mq == nil {
		writeError(w, http.StatusServiceUnavailable, "not_available",
			"metrics querier not available")
		return
	}

	result, err := mq.CurrentMetrics(r.Context(), instanceID)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "current metrics query failed",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to query current metrics")
		return
	}

	writeJSON(w, http.StatusOK, Envelope{Data: result})
}

// handleMetricsHistory returns time series data for one or more metrics.
func (s *APIServer) handleMetricsHistory(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")

	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	// Accept both forms:
	//   ?metrics=a,b,c        (comma-separated, sent by the frontend)
	//   ?metric=a&metric=b    (repeated params, legacy/API clients)
	var metrics []string
	if csv := r.URL.Query().Get("metrics"); csv != "" {
		for _, m := range strings.Split(csv, ",") {
			if m = strings.TrimSpace(m); m != "" {
				metrics = append(metrics, m)
			}
		}
	}
	if len(metrics) == 0 {
		metrics = r.URL.Query()["metric"]
	}
	if len(metrics) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request",
			"at least one 'metric' or 'metrics' query parameter is required")
		return
	}

	now := time.Now()
	from := now.Add(-1 * time.Hour)
	to := now

	if s := r.URL.Query().Get("from"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request",
				"invalid 'from' parameter: must be RFC3339")
			return
		}
		from = t
	}

	if s := r.URL.Query().Get("to"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request",
				"invalid 'to' parameter: must be RFC3339")
			return
		}
		to = t
	}

	if !from.Before(to) {
		writeError(w, http.StatusBadRequest, "bad_request",
			"'from' must be before 'to'")
		return
	}

	step := r.URL.Query().Get("step")
	if step != "" {
		if err := storage.ValidStep(step); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
	}

	mq := s.metricsQuerier()
	if mq == nil {
		writeError(w, http.StatusServiceUnavailable, "not_available",
			"metrics querier not available")
		return
	}

	result, err := mq.HistoryMetrics(r.Context(), storage.HistoryRequest{
		InstanceID: instanceID,
		Metrics:    metrics,
		From:       from,
		To:         to,
		Step:       step,
	})
	if err != nil {
		s.logger.ErrorContext(r.Context(), "metrics history query failed",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to query metrics history")
		return
	}

	writeJSON(w, http.StatusOK, Envelope{Data: result})
}
