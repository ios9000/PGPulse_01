package etcd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPProvider_GetMembers_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/members" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"members": [
				{
					"id": "abc123",
					"name": "etcd1",
					"peerURLs": ["http://10.0.0.1:2380"],
					"clientURLs": ["http://10.0.0.1:2379"]
				},
				{
					"id": "def456",
					"name": "etcd2",
					"peerURLs": ["http://10.0.0.2:2380"],
					"clientURLs": ["http://10.0.0.2:2379"]
				}
			]
		}`))
	}))
	defer server.Close()

	provider := NewHTTPProvider([]string{server.URL})
	members, err := provider.GetMembers(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}

	if members[0].Name != "etcd1" || members[0].ID != "abc123" {
		t.Errorf("unexpected member 0: %+v", members[0])
	}
	if members[0].PeerURL != "http://10.0.0.1:2380" {
		t.Errorf("unexpected peer URL: %q", members[0].PeerURL)
	}
	if members[0].ClientURL != "http://10.0.0.1:2379" {
		t.Errorf("unexpected client URL: %q", members[0].ClientURL)
	}
	if members[1].Name != "etcd2" {
		t.Errorf("unexpected member 1: %+v", members[1])
	}
}

func TestHTTPProvider_GetMembers_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	provider := NewHTTPProvider([]string{server.URL})
	_, err := provider.GetMembers(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrETCDUnavailable) {
		t.Errorf("expected ErrETCDUnavailable, got %v", err)
	}
}

func TestHTTPProvider_GetEndpointHealth_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"health": "true"}`))
	}))
	defer server.Close()

	provider := NewHTTPProvider([]string{server.URL})
	health, err := provider.GetEndpointHealth(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !health[server.URL] {
		t.Errorf("expected endpoint healthy, got %v", health)
	}
}

func TestHTTPProvider_GetEndpointHealth_Unhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"health": "false"}`))
	}))
	defer server.Close()

	provider := NewHTTPProvider([]string{server.URL})
	health, err := provider.GetEndpointHealth(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if health[server.URL] {
		t.Errorf("expected endpoint unhealthy, got true")
	}
}
