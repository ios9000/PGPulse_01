package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ios9000/PGPulse_01/internal/config"
)

func boolPtr(b bool) *bool { return &b }

func TestListInstances_Empty(t *testing.T) {
	s := newTestServer(t, &mockStore{}, nil, []config.InstanceConfig{})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var env struct {
		Data []InstanceResponse `json:"data"`
		Meta struct {
			Count int `json:"count"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &env))
	assert.Empty(t, env.Data)
	assert.Equal(t, 0, env.Meta.Count)
}

func TestListInstances_Multiple(t *testing.T) {
	instances := []config.InstanceConfig{
		{ID: "a", DSN: "postgres://u:p@host1:5432/db", Enabled: boolPtr(true)},
		{ID: "b", DSN: "postgres://u:p@host2:5433/db", Enabled: boolPtr(false)},
		{ID: "c", DSN: "postgres://u:p@host3/db", Enabled: nil},
	}
	s := newTestServer(t, &mockStore{}, nil, instances)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var env struct {
		Data []InstanceResponse `json:"data"`
		Meta struct {
			Count int `json:"count"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &env))
	assert.Len(t, env.Data, 3)
	assert.Equal(t, 3, env.Meta.Count)
}

func TestGetInstance_Found(t *testing.T) {
	instances := []config.InstanceConfig{
		{ID: "prod-01", DSN: "postgres://u:p@10.0.0.1:5432/db", Description: "Primary", Enabled: boolPtr(true)},
	}
	s := newTestServer(t, &mockStore{}, nil, instances)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/prod-01", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var env struct {
		Data InstanceResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &env))
	assert.Equal(t, "prod-01", env.Data.ID)
	assert.Equal(t, "10.0.0.1", env.Data.Host)
	assert.Equal(t, 5432, env.Data.Port)
}

func TestGetInstance_NotFound(t *testing.T) {
	s := newTestServer(t, &mockStore{}, nil, []config.InstanceConfig{})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/missing", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)

	var errResp ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "not_found", errResp.Error.Code)
}

func TestParseHostPort(t *testing.T) {
	tests := []struct {
		name     string
		dsn      string
		wantHost string
		wantPort int
	}{
		{"url with port", "postgres://user:pass@myhost:5433/mydb", "myhost", 5433},
		{"url without port", "postgres://user:pass@myhost/mydb", "myhost", 5432},
		{"url with IP", "postgres://user:pass@10.0.0.1:5432/db", "10.0.0.1", 5432},
		{"keyword/value with port", "host=myhost port=5433 dbname=mydb user=pgpulse", "myhost", 5433},
		{"keyword/value without port", "host=myhost dbname=mydb user=pgpulse", "myhost", 5432},
		{"keyword/value with IP", "host=10.0.0.1 port=5434 dbname=db", "10.0.0.1", 5434},
		{"empty string", "", "", 5432},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port := parseHostPort(tt.dsn)
			assert.Equal(t, tt.wantHost, host)
			assert.Equal(t, tt.wantPort, port)
		})
	}
}

func TestExtractHostPort(t *testing.T) {
	tests := []struct {
		name     string
		dsn      string
		wantHost string
		wantPort int
	}{
		{"url with port", "postgres://user:pass@myhost:5433/mydb", "myhost", 5433},
		{"url without port", "postgres://user:pass@myhost/mydb", "myhost", 5432},
		{"keyword/value with port", "host=myhost port=5433 dbname=mydb", "myhost", 5433},
		{"keyword/value without port", "host=myhost dbname=mydb", "myhost", 5432},
		{"empty string", "", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port := extractHostPort(tt.dsn)
			assert.Equal(t, tt.wantHost, host)
			assert.Equal(t, tt.wantPort, port)
		})
	}
}

func TestListInstances_FieldMapping(t *testing.T) {
	instances := []config.InstanceConfig{
		{
			ID:          "replica-01",
			DSN:         "postgres://u:p@host:5433/db",
			Description: "Read replica",
			Enabled:     boolPtr(false),
		},
	}
	s := newTestServer(t, &mockStore{}, nil, instances)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances", nil)
	s.Routes().ServeHTTP(rr, req)

	var env struct {
		Data []InstanceResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &env))
	require.Len(t, env.Data, 1)

	item := env.Data[0]
	assert.Equal(t, "Read replica", item.Description)
	assert.False(t, item.Enabled)
	assert.Equal(t, "host", item.Host)
	assert.Equal(t, 5433, item.Port)
}
