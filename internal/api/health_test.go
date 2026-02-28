package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealth_AllOK(t *testing.T) {
	s := newTestServer(t, &mockStore{}, &mockPinger{}, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, "ok", resp.Storage)
}

func TestHealth_StorageError(t *testing.T) {
	pinger := &mockPinger{err: errors.New("connection refused")}
	s := newTestServer(t, &mockStore{}, pinger, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "error", resp.Status)
	assert.Equal(t, "error", resp.Storage)
}

func TestHealth_NoStorage(t *testing.T) {
	s := newTestServer(t, &mockStore{}, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, "disabled", resp.Storage)
}

func TestHealth_ContainsVersion(t *testing.T) {
	s := newTestServer(t, &mockStore{}, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	s.Routes().ServeHTTP(rr, req)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Version)
}

func TestHealth_ContainsUptime(t *testing.T) {
	s := newTestServer(t, &mockStore{}, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	s.Routes().ServeHTTP(rr, req)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Uptime)
}
