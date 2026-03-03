package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleProgress_NoConnProvider(t *testing.T) {
	srv := newTestServer(t, &mockStore{}, nil, testInstance("test-1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/test-1/activity/progress", nil)
	srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var errResp ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if errResp.Error.Code != "not_available" {
		t.Errorf("expected error code 'not_available', got %q", errResp.Error.Code)
	}
}

func TestHandleProgress_InstanceNotFound(t *testing.T) {
	srv := newTestServer(t, &mockStore{}, nil, testInstance("test-1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/nonexistent/activity/progress", nil)
	srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var errResp ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if errResp.Error.Code != "not_found" {
		t.Errorf("expected error code 'not_found', got %q", errResp.Error.Code)
	}
}
