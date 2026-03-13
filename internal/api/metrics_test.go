package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/config"
	"github.com/ios9000/PGPulse_01/internal/storage"
)

// testInstance is a convenience helper for a single instance.
func testInstance(id string) []config.InstanceConfig {
	return []config.InstanceConfig{
		{ID: id, DSN: "postgres://u:p@host:5432/db", Enabled: boolPtr(true)},
	}
}

func TestQueryMetrics_DefaultParams(t *testing.T) {
	store := &mockStore{points: []collector.MetricPoint{}}
	s := newTestServer(t, store, nil, testInstance("inst1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/inst1/metrics", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, 1000, store.lastQuery.Limit)
	assert.Equal(t, "inst1", store.lastQuery.InstanceID)
	// Default time range: start should be ~1 hour before end.
	diff := store.lastQuery.End.Sub(store.lastQuery.Start)
	assert.InDelta(t, time.Hour.Seconds(), diff.Seconds(), 5)
}

func TestQueryMetrics_CustomTimeRange(t *testing.T) {
	store := &mockStore{points: []collector.MetricPoint{}}
	s := newTestServer(t, store, nil, testInstance("inst1"))

	start := time.Date(2026, 2, 27, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 27, 11, 0, 0, 0, time.UTC)
	path := fmt.Sprintf("/api/v1/instances/inst1/metrics?start=%s&end=%s",
		start.Format(time.RFC3339), end.Format(time.RFC3339))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.True(t, store.lastQuery.Start.Equal(start))
	assert.True(t, store.lastQuery.End.Equal(end))
}

func TestQueryMetrics_InvalidStart(t *testing.T) {
	s := newTestServer(t, &mockStore{}, nil, testInstance("inst1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/inst1/metrics?start=not-a-date", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)

	var errResp ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "bad_request", errResp.Error.Code)
}

func TestQueryMetrics_InvalidLimit(t *testing.T) {
	s := newTestServer(t, &mockStore{}, nil, testInstance("inst1"))

	for _, badLimit := range []string{"-1", "0", "99999", "abc"} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/instances/inst1/metrics?limit="+badLimit, nil)
		s.Routes().ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code, "limit=%s should be 400", badLimit)
	}
}

func TestQueryMetrics_InstanceNotFound(t *testing.T) {
	s := newTestServer(t, &mockStore{}, nil, testInstance("inst1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/does-not-exist/metrics", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)

	var errResp ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "not_found", errResp.Error.Code)
}

func TestQueryMetrics_StorageError(t *testing.T) {
	store := &mockStore{err: errors.New("db down")}
	s := newTestServer(t, store, nil, testInstance("inst1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/inst1/metrics", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	var errResp ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "internal_error", errResp.Error.Code)
}

func TestQueryMetrics_JSONFormat(t *testing.T) {
	ts := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	store := &mockStore{points: []collector.MetricPoint{
		{InstanceID: "inst1", Metric: "pg.conn.active", Value: 42, Labels: map[string]string{"state": "active"}, Timestamp: ts},
	}}
	s := newTestServer(t, store, nil, testInstance("inst1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/inst1/metrics", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Header().Get("Content-Type"), "application/json")

	var env struct {
		Data []collector.MetricPoint `json:"data"`
		Meta struct {
			Count int `json:"count"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &env))
	assert.Len(t, env.Data, 1)
	assert.Equal(t, 1, env.Meta.Count)
}

func TestQueryMetrics_CSVFormat(t *testing.T) {
	ts := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	store := &mockStore{points: []collector.MetricPoint{
		{InstanceID: "inst1", Metric: "pg.conn.active", Value: 42, Labels: map[string]string{}, Timestamp: ts},
	}}
	s := newTestServer(t, store, nil, testInstance("inst1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/inst1/metrics?format=csv", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "text/csv", rr.Header().Get("Content-Type"))

	body := rr.Body.String()
	lines := strings.Split(strings.TrimSpace(body), "\n")
	assert.Len(t, lines, 2)
	assert.Equal(t, "instance_id,metric,value,labels,timestamp", lines[0])
	assert.Contains(t, lines[1], "inst1")
	assert.Contains(t, lines[1], "42.000000")
}

func TestQueryMetrics_CSVAcceptHeader(t *testing.T) {
	store := &mockStore{points: []collector.MetricPoint{}}
	s := newTestServer(t, store, nil, testInstance("inst1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/inst1/metrics", nil)
	req.Header.Set("Accept", "text/csv")
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "text/csv", rr.Header().Get("Content-Type"))
}

func TestQueryMetrics_EmptyResult(t *testing.T) {
	store := &mockStore{points: []collector.MetricPoint{}}
	s := newTestServer(t, store, nil, testInstance("inst1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/inst1/metrics", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var env struct {
		Data []collector.MetricPoint `json:"data"`
		Meta struct {
			Count int `json:"count"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &env))
	assert.Empty(t, env.Data)
	assert.Equal(t, 0, env.Meta.Count)
}

// --- GET /api/v1/instances/{id}/metrics/current ---

func TestHandleCurrentMetrics_Success(t *testing.T) {
	now := time.Date(2026, 3, 3, 12, 0, 0, 0, time.UTC)
	store := &mockMetricsStore{
		currentResult: &storage.CurrentMetricsResult{
			InstanceID:  "inst1",
			CollectedAt: now,
			Metrics: map[string]storage.MetricValue{
				"pg.conn.active": {Value: 42, Labels: map[string]string{"state": "active"}},
				"pg.cache.ratio": {Value: 0.99},
			},
		},
	}
	s := newTestServer(t, store, nil, testInstance("inst1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/inst1/metrics/current", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Header().Get("Content-Type"), "application/json")

	var env struct {
		Data storage.CurrentMetricsResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &env))
	assert.Equal(t, "inst1", env.Data.InstanceID)
	assert.Len(t, env.Data.Metrics, 2)
	assert.InDelta(t, 42.0, env.Data.Metrics["pg.conn.active"].Value, 0.01)
}

func TestHandleCurrentMetrics_InstanceNotFound(t *testing.T) {
	store := &mockMetricsStore{}
	s := newTestServer(t, store, nil, testInstance("inst1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/unknown/metrics/current", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// --- GET /api/v1/instances/{id}/metrics/history ---

func TestHandleMetricsHistory_Success(t *testing.T) {
	from := time.Date(2026, 3, 3, 10, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 3, 11, 0, 0, 0, time.UTC)
	store := &mockMetricsStore{
		historyResult: &storage.HistoryResult{
			InstanceID: "inst1",
			From:       from,
			To:         to,
			Step:       "5m",
			Series: map[string][]storage.TimeSeriesPoint{
				"pg.conn.active": {
					{T: from, V: 10},
					{T: from.Add(5 * time.Minute), V: 12},
				},
			},
		},
	}
	s := newTestServer(t, store, nil, testInstance("inst1"))

	path := fmt.Sprintf("/api/v1/instances/inst1/metrics/history?metric=pgpulse.conn.active&from=%s&to=%s&step=5m",
		from.Format(time.RFC3339), to.Format(time.RFC3339))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var env struct {
		Data storage.HistoryResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &env))
	assert.Equal(t, "inst1", env.Data.InstanceID)
	assert.Len(t, env.Data.Series["pg.conn.active"], 2)
}

func TestHandleMetricsHistory_InvalidStep(t *testing.T) {
	store := &mockMetricsStore{}
	s := newTestServer(t, store, nil, testInstance("inst1"))

	path := "/api/v1/instances/inst1/metrics/history?metric=pgpulse.conn.active&step=2m"

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)

	var errResp ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "bad_request", errResp.Error.Code)
}

func TestHandleMetricsHistory_MissingMetric(t *testing.T) {
	store := &mockMetricsStore{}
	s := newTestServer(t, store, nil, testInstance("inst1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/inst1/metrics/history", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)

	var errResp ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "bad_request", errResp.Error.Code)
}

func TestHandleMetricsHistory_FromAfterTo(t *testing.T) {
	store := &mockMetricsStore{}
	s := newTestServer(t, store, nil, testInstance("inst1"))

	from := time.Date(2026, 3, 3, 12, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 3, 10, 0, 0, 0, time.UTC)
	path := fmt.Sprintf("/api/v1/instances/inst1/metrics/history?metric=pgpulse.conn.active&from=%s&to=%s",
		from.Format(time.RFC3339), to.Format(time.RFC3339))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
