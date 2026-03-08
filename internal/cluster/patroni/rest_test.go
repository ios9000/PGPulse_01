package patroni

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRESTProvider_GetClusterState_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cluster" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"scope": "main",
			"members": [
				{
					"name": "pg1",
					"host": "10.0.0.1",
					"port": 5432,
					"role": "leader",
					"state": "running",
					"timeline": 1,
					"lag": 0
				},
				{
					"name": "pg2",
					"host": "10.0.0.2",
					"port": 5432,
					"role": "replica",
					"state": "streaming",
					"timeline": 1,
					"lag": 1024,
					"tags": {"nofailover": "false"}
				}
			]
		}`))
	}))
	defer server.Close()

	provider := NewRESTProvider(server.URL)
	state, err := provider.GetClusterState(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if state.ClusterName != "main" {
		t.Errorf("expected cluster name 'main', got %q", state.ClusterName)
	}
	if len(state.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(state.Members))
	}

	m1 := state.Members[0]
	if m1.Name != "pg1" || m1.Host != "10.0.0.1" || m1.Port != 5432 {
		t.Errorf("unexpected member 0: %+v", m1)
	}
	if m1.Role != "leader" || m1.State != "running" {
		t.Errorf("unexpected role/state: %s/%s", m1.Role, m1.State)
	}
	if m1.Timeline != 1 || m1.Lag != 0 {
		t.Errorf("unexpected timeline/lag: %d/%d", m1.Timeline, m1.Lag)
	}

	m2 := state.Members[1]
	if m2.Name != "pg2" || m2.Lag != 1024 {
		t.Errorf("unexpected member 1: %+v", m2)
	}
	if m2.Tags["nofailover"] != "false" {
		t.Errorf("expected nofailover tag 'false', got %q", m2.Tags["nofailover"])
	}
}

func TestRESTProvider_GetClusterState_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	provider := NewRESTProvider(server.URL)
	_, err := provider.GetClusterState(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrPatroniUnavailable) {
		t.Errorf("expected ErrPatroniUnavailable, got %v", err)
	}
}

func TestRESTProvider_GetVersion_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"patroni": {
				"version": "3.0.4",
				"scope": "main"
			},
			"state": "running",
			"role": "master"
		}`))
	}))
	defer server.Close()

	provider := NewRESTProvider(server.URL)
	version, err := provider.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if version != "3.0.4" {
		t.Errorf("expected version '3.0.4', got %q", version)
	}
}

func TestRESTProvider_GetHistory_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/history" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Patroni history: array of arrays [timeline, lsn, reason, timestamp]
		_, _ = w.Write([]byte(`[
			[1, 12345678, "no recovery target specified", "2026-03-01T10:00:00+00:00"],
			[2, 23456789, "manual switchover", "2026-03-02T12:00:00+00:00"]
		]`))
	}))
	defer server.Close()

	provider := NewRESTProvider(server.URL)
	events, err := provider.GetHistory(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Reason != "no recovery target specified" {
		t.Errorf("unexpected reason: %q", events[0].Reason)
	}
	if events[1].Reason != "manual switchover" {
		t.Errorf("unexpected reason: %q", events[1].Reason)
	}
}

func TestRESTProvider_GetHistory_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	provider := NewRESTProvider(server.URL)
	events, err := provider.GetHistory(context.Background())
	// Invalid JSON should return nil, nil (non-fatal).
	if err != nil {
		t.Fatalf("expected no error for invalid JSON, got %v", err)
	}
	if events != nil {
		t.Errorf("expected nil events, got %+v", events)
	}
}
