package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Scraper is an HTTP client that fetches metrics from a remote pgpulse-agent.
type Scraper struct {
	baseURL    string
	httpClient *http.Client
}

// NewScraper creates a new Scraper targeting the given agent base URL.
func NewScraper(baseURL string) *Scraper {
	return &Scraper{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// ScrapeOS fetches OS metrics from the agent's /metrics/os endpoint.
func (s *Scraper) ScrapeOS(ctx context.Context) (*OSSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/metrics/os", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scrape OS metrics: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d", resp.StatusCode)
	}
	var snap OSSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return nil, fmt.Errorf("decode OS metrics: %w", err)
	}
	return &snap, nil
}

// ScrapeCluster fetches cluster metrics from the agent's /metrics/cluster endpoint.
func (s *Scraper) ScrapeCluster(ctx context.Context) (*ClusterSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/metrics/cluster", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scrape cluster metrics: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d", resp.StatusCode)
	}
	var snap ClusterSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return nil, fmt.Errorf("decode cluster metrics: %w", err)
	}
	return &snap, nil
}

// IsAlive checks whether the agent is reachable via its /health endpoint.
func (s *Scraper) IsAlive(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
