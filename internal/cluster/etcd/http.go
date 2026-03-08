package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPProvider uses the ETCD v2 HTTP API to retrieve cluster information.
type HTTPProvider struct {
	endpoints  []string
	httpClient *http.Client
}

// NewHTTPProvider creates an HTTPProvider targeting the given ETCD endpoints.
func NewHTTPProvider(endpoints []string) *HTTPProvider {
	return &HTTPProvider{
		endpoints:  endpoints,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// etcdMembersResponse matches the JSON returned by GET /v2/members.
type etcdMembersResponse struct {
	Members []struct {
		ID         string   `json:"id"`
		Name       string   `json:"name"`
		PeerURLs   []string `json:"peerURLs"`
		ClientURLs []string `json:"clientURLs"`
	} `json:"members"`
}

// GetMembers retrieves ETCD cluster members via GET /v2/members.
// It tries each endpoint in order until one succeeds.
func (h *HTTPProvider) GetMembers(ctx context.Context) ([]ETCDMember, error) {
	var lastErr error

	for _, ep := range h.endpoints {
		members, err := h.getMembersFromEndpoint(ctx, ep)
		if err != nil {
			lastErr = err
			continue
		}
		return members, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("%w: no endpoints configured", ErrETCDUnavailable)
}

func (h *HTTPProvider) getMembersFromEndpoint(ctx context.Context, endpoint string) ([]ETCDMember, error) {
	url := strings.TrimRight(endpoint, "/") + "/v2/members"

	body, err := h.doGet(ctx, url)
	if err != nil {
		return nil, err
	}

	var resp etcdMembersResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("etcd http: parse members response: %w", err)
	}

	members := make([]ETCDMember, len(resp.Members))
	for i, m := range resp.Members {
		var peerURL, clientURL string
		if len(m.PeerURLs) > 0 {
			peerURL = m.PeerURLs[0]
		}
		if len(m.ClientURLs) > 0 {
			clientURL = m.ClientURLs[0]
		}

		members[i] = ETCDMember{
			ID:        m.ID,
			Name:      m.Name,
			PeerURL:   peerURL,
			ClientURL: clientURL,
			IsLeader:  false, // v2 API does not expose leader info directly
			Status:    "running",
		}
	}

	return members, nil
}

// etcdHealthResponse matches the JSON returned by GET /health.
type etcdHealthResponse struct {
	Health string `json:"health"`
}

// GetEndpointHealth checks the health of each configured endpoint.
// It returns a map of endpoint URL to health status (true = healthy).
func (h *HTTPProvider) GetEndpointHealth(ctx context.Context) (map[string]bool, error) {
	result := make(map[string]bool, len(h.endpoints))

	for _, ep := range h.endpoints {
		healthy := h.checkHealth(ctx, ep)
		result[ep] = healthy
	}

	return result, nil
}

func (h *HTTPProvider) checkHealth(ctx context.Context, endpoint string) bool {
	url := strings.TrimRight(endpoint, "/") + "/health"

	body, err := h.doGet(ctx, url)
	if err != nil {
		return false
	}

	var resp etcdHealthResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return false
	}

	return resp.Health == "true"
}

// doGet performs a GET request and returns the response body.
func (h *HTTPProvider) doGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("etcd http: create request: %w", err)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrETCDUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("etcd http: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrETCDUnavailable, resp.StatusCode)
	}

	return body, nil
}
