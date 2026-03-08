package agent

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestScraper_ScrapeOS_Success(t *testing.T) {
	expected := &OSSnapshot{
		CollectedAt: time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC),
		Hostname:    "testhost",
		UptimeSecs:  86400,
		LoadAvg:     LoadAvg{One: 0.5, Five: 0.3, Fifteen: 0.1},
		Memory: MemoryInfo{
			TotalKB:     16384000,
			AvailableKB: 8192000,
			UsedKB:      8192000,
		},
		CPU: CPUInfo{
			UserPct:   25.0,
			SystemPct: 10.0,
			IOWaitPct: 2.0,
			IdlePct:   63.0,
			NumCPUs:   4,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics/os" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(expected); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer srv.Close()

	scraper := NewScraper(srv.URL)
	snap, err := scraper.ScrapeOS(context.Background())
	if err != nil {
		t.Fatalf("ScrapeOS returned error: %v", err)
	}

	if snap.Hostname != expected.Hostname {
		t.Errorf("hostname = %q, want %q", snap.Hostname, expected.Hostname)
	}
	if snap.CPU.NumCPUs != expected.CPU.NumCPUs {
		t.Errorf("num_cpus = %d, want %d", snap.CPU.NumCPUs, expected.CPU.NumCPUs)
	}
	if snap.Memory.TotalKB != expected.Memory.TotalKB {
		t.Errorf("total_kb = %d, want %d", snap.Memory.TotalKB, expected.Memory.TotalKB)
	}
}

func TestScraper_ScrapeOS_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	scraper := NewScraper(srv.URL)
	_, err := scraper.ScrapeOS(context.Background())
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}

func TestScraper_IsAlive_True(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	scraper := NewScraper(srv.URL)
	if !scraper.IsAlive(context.Background()) {
		t.Error("IsAlive returned false, expected true")
	}
}

func TestScraper_IsAlive_False(t *testing.T) {
	// Create a listener and immediately close it to get an unreachable address.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	scraper := NewScraper("http://" + addr)
	if scraper.IsAlive(context.Background()) {
		t.Error("IsAlive returned true for unreachable server, expected false")
	}
}

func TestScraper_ScrapeCluster_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics/cluster" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"patroni":{"cluster_name":"main"},"etcd":null}`))
	}))
	defer srv.Close()

	scraper := NewScraper(srv.URL)
	snap, err := scraper.ScrapeCluster(context.Background())
	if err != nil {
		t.Fatalf("ScrapeCluster returned error: %v", err)
	}
	if snap.Patroni == nil {
		t.Error("expected patroni data, got nil")
	}
}
