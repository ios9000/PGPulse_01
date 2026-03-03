package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleWaitEvents_NoAuth(t *testing.T) {
	jwtSvc := testJWTSvc()
	s := newAuthTestServer(t, newMockUserStore(), jwtSvc)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/x/activity/wait-events", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	var errResp ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "UNAUTHORIZED", errResp.Error.Code)
}

func TestHandleLongTransactions_NoAuth(t *testing.T) {
	jwtSvc := testJWTSvc()
	s := newAuthTestServer(t, newMockUserStore(), jwtSvc)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/x/activity/long-transactions", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	var errResp ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "UNAUTHORIZED", errResp.Error.Code)
}

func TestHandleWaitEvents_NoConnProvider(t *testing.T) {
	// Auth disabled, no conn provider — should return 503.
	s := newTestServer(t, &mockStore{}, nil, testInstance("inst1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/inst1/activity/wait-events", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)

	var errResp ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "not_available", errResp.Error.Code)
}

func TestHandleLongTransactions_NoConnProvider(t *testing.T) {
	s := newTestServer(t, &mockStore{}, nil, testInstance("inst1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/inst1/activity/long-transactions", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

func TestHandleLongTransactions_InstanceNotFound(t *testing.T) {
	s := newTestServer(t, &mockStore{}, nil, testInstance("inst1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/unknown/activity/long-transactions", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandleWaitEvents_InstanceNotFound(t *testing.T) {
	s := newTestServer(t, &mockStore{}, nil, testInstance("inst1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/unknown/activity/wait-events", nil)
	s.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}
