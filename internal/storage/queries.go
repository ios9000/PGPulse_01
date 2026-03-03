package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// MetricValue holds a single metric's current value and labels.
type MetricValue struct {
	Value  float64           `json:"value"`
	Labels map[string]string `json:"labels,omitempty"`
}

// CurrentMetricsResult holds the latest value of each metric for an instance.
type CurrentMetricsResult struct {
	InstanceID  string                 `json:"instance_id"`
	CollectedAt time.Time              `json:"collected_at"`
	Metrics     map[string]MetricValue `json:"metrics"`
}

// HistoryRequest defines parameters for querying metric history with optional aggregation.
type HistoryRequest struct {
	InstanceID string
	Metrics    []string
	From       time.Time
	To         time.Time
	Step       string // "1m", "5m", "15m", "1h", "1d" or empty for raw
}

// TimeSeriesPoint is a single point in a time series.
type TimeSeriesPoint struct {
	T time.Time `json:"t"`
	V float64   `json:"v"`
}

// HistoryResult holds time series data for one or more metrics.
type HistoryResult struct {
	InstanceID string                       `json:"instance_id"`
	From       time.Time                    `json:"from"`
	To         time.Time                    `json:"to"`
	Step       string                       `json:"step,omitempty"`
	Series     map[string][]TimeSeriesPoint `json:"series"`
}

// validSteps maps user-facing step strings to validity.
var validSteps = map[string]bool{
	"1m": true, "5m": true, "15m": true, "1h": true, "1d": true,
}

// ValidStep returns an error if step is not one of the allowed values.
func ValidStep(step string) error {
	if !validSteps[step] {
		return fmt.Errorf("invalid step %q: must be one of 1m, 5m, 15m, 1h, 1d", step)
	}
	return nil
}

// bucketExpr returns a SQL expression that buckets the "time" column into the given step.
func bucketExpr(step string) string {
	switch step {
	case "1m":
		return "date_trunc('minute', time)"
	case "5m":
		return "date_trunc('hour', time) + (EXTRACT(MINUTE FROM time)::int / 5) * interval '5 minutes'"
	case "15m":
		return "date_trunc('hour', time) + (EXTRACT(MINUTE FROM time)::int / 15) * interval '15 minutes'"
	case "1h":
		return "date_trunc('hour', time)"
	case "1d":
		return "date_trunc('day', time)"
	default:
		return "date_trunc('minute', time)"
	}
}

// CurrentMetrics returns the most recent value of each metric for an instance.
func (s *PGStore) CurrentMetrics(ctx context.Context, instanceID string) (*CurrentMetricsResult, error) {
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	const sql = `SELECT DISTINCT ON (metric) metric, value, labels, time
FROM metrics WHERE instance_id = $1
ORDER BY metric, time DESC`

	rows, err := s.pool.Query(qCtx, sql, instanceID)
	if err != nil {
		return nil, fmt.Errorf("current metrics query: %w", err)
	}
	defer rows.Close()

	result := &CurrentMetricsResult{
		InstanceID: instanceID,
		Metrics:    make(map[string]MetricValue),
	}

	for rows.Next() {
		var metric string
		var value float64
		var labelsJSON []byte
		var ts time.Time

		if err := rows.Scan(&metric, &value, &labelsJSON, &ts); err != nil {
			return nil, fmt.Errorf("scan current metric row: %w", err)
		}

		var labels map[string]string
		if err := json.Unmarshal(labelsJSON, &labels); err != nil {
			return nil, fmt.Errorf("unmarshal labels: %w", err)
		}

		result.Metrics[metric] = MetricValue{Value: value, Labels: labels}

		if ts.After(result.CollectedAt) {
			result.CollectedAt = ts
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("current metrics rows: %w", err)
	}

	return result, nil
}

// HistoryMetrics returns time series data for the requested metrics and time range.
func (s *PGStore) HistoryMetrics(ctx context.Context, req HistoryRequest) (*HistoryResult, error) {
	qCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result := &HistoryResult{
		InstanceID: req.InstanceID,
		From:       req.From,
		To:         req.To,
		Step:       req.Step,
		Series:     make(map[string][]TimeSeriesPoint),
	}

	// Initialize empty slices for all requested metrics.
	for _, m := range req.Metrics {
		result.Series[m] = []TimeSeriesPoint{}
	}

	if req.Step != "" {
		// Aggregated query with bucketing.
		bucket := bucketExpr(req.Step)
		sql := fmt.Sprintf(`SELECT %s AS bucket, metric, AVG(value) AS value
FROM metrics
WHERE instance_id = $1 AND metric = ANY($2)
  AND time >= $3 AND time <= $4
GROUP BY bucket, metric ORDER BY bucket`, bucket)

		rows, err := s.pool.Query(qCtx, sql, req.InstanceID, req.Metrics, req.From, req.To)
		if err != nil {
			return nil, fmt.Errorf("history metrics aggregated query: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var t time.Time
			var metric string
			var value float64
			if err := rows.Scan(&t, &metric, &value); err != nil {
				return nil, fmt.Errorf("scan history row: %w", err)
			}
			result.Series[metric] = append(result.Series[metric], TimeSeriesPoint{T: t, V: value})
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("history metrics rows: %w", err)
		}
	} else {
		// Raw query, capped at 1000 per metric.
		limit := 1000 * len(req.Metrics)
		const sql = `SELECT metric, value, time
FROM metrics WHERE instance_id = $1 AND metric = ANY($2)
  AND time >= $3 AND time <= $4
ORDER BY metric, time
LIMIT $5`

		rows, err := s.pool.Query(qCtx, sql, req.InstanceID, req.Metrics, req.From, req.To, limit)
		if err != nil {
			return nil, fmt.Errorf("history metrics raw query: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var metric string
			var value float64
			var t time.Time
			if err := rows.Scan(&metric, &value, &t); err != nil {
				return nil, fmt.Errorf("scan raw history row: %w", err)
			}
			result.Series[metric] = append(result.Series[metric], TimeSeriesPoint{T: t, V: value})
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("raw history rows: %w", err)
		}
	}

	return result, nil
}

// CurrentMetricValues returns a simplified map of metric name to latest value for an instance.
// Used for fleet enrichment (instance list with metrics).
func (s *PGStore) CurrentMetricValues(ctx context.Context, instanceID string) (map[string]float64, error) {
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	const sql = `SELECT DISTINCT ON (metric) metric, value
FROM metrics WHERE instance_id = $1
ORDER BY metric, time DESC`

	rows, err := s.pool.Query(qCtx, sql, instanceID)
	if err != nil {
		return nil, fmt.Errorf("current metric values query: %w", err)
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var metric string
		var value float64
		if err := rows.Scan(&metric, &value); err != nil {
			return nil, fmt.Errorf("scan metric value: %w", err)
		}
		result[metric] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("metric values rows: %w", err)
	}

	return result, nil
}
