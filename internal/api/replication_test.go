package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleReplication_NoAuth(t *testing.T) {
	jwtSvc := testJWTSvc()
	s := newAuthTestServer(t, newMockUserStore(), jwtSvc)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/x/replication", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	var errResp ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "UNAUTHORIZED", errResp.Error.Code)
}

func TestHandleReplication_NoConnProvider(t *testing.T) {
	s := newTestServer(t, &mockStore{}, nil, testInstance("inst1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/inst1/replication", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)

	var errResp ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "not_available", errResp.Error.Code)
}

func TestHandleReplication_InstanceNotFound(t *testing.T) {
	s := newTestServer(t, &mockStore{}, nil, testInstance("inst1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/unknown/replication", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}
