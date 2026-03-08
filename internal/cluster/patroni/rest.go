package patroni

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RESTProvider calls Patroni's HTTP API to retrieve cluster information.
type RESTProvider struct {
	baseURL    string
	httpClient *http.Client
}

// NewRESTProvider creates a RESTProvider targeting the given Patroni REST endpoint.
func NewRESTProvider(baseURL string) *RESTProvider {
	return &RESTProvider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// patroniClusterResponse matches the JSON returned by GET /cluster.
type patroniClusterResponse struct {
	Scope   string `json:"scope"`
	Members []struct {
		Name     string            `json:"name"`
		Host     string            `json:"host"`
		Port     int               `json:"port"`
		Role     string            `json:"role"`
		State    string            `json:"state"`
		Timeline int              `json:"timeline"`
		Lag      int64             `json:"lag"`
		Tags     map[string]string `json:"tags,omitempty"`
	} `json:"members"`
}

// GetClusterState retrieves cluster state via GET /cluster.
func (r *RESTProvider) GetClusterState(ctx context.Context) (*ClusterState, error) {
	body, err := r.doGet(ctx, "/cluster")
	if err != nil {
		return nil, err
	}

	var resp patroniClusterResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("patroni rest: parse cluster response: %w", err)
	}

	state := &ClusterState{
		ClusterName: resp.Scope,
		Members:     make([]ClusterMember, len(resp.Members)),
	}
	for i, m := range resp.Members {
		state.Members[i] = ClusterMember{
			Name:     m.Name,
			Host:     m.Host,
			Port:     m.Port,
			Role:     m.Role,
			State:    m.State,
			Timeline: m.Timeline,
			Lag:      m.Lag,
			Tags:     m.Tags,
		}
	}

	return state, nil
}

// GetHistory retrieves switchover/failover history via GET /history.
// Patroni returns an array of arrays: each sub-array contains
// [timeline, lsn, reason, timestamp]. Parsing failures are silently
// skipped; an empty slice is returned rather than an error.
func (r *RESTProvider) GetHistory(ctx context.Context) ([]SwitchoverEvent, error) {
	body, err := r.doGet(ctx, "/history")
	if err != nil {
		return nil, err
	}

	var raw []json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, nil //nolint:nilerr // history parse failure is non-fatal
	}

	var events []SwitchoverEvent
	for _, entry := range raw {
		var arr []json.RawMessage
		if err := json.Unmarshal(entry, &arr); err != nil {
			continue
		}
		if len(arr) < 4 {
			continue
		}

		evt := SwitchoverEvent{}

		// arr[2] = reason (string)
		var reason string
		if err := json.Unmarshal(arr[2], &reason); err == nil {
			evt.Reason = reason
		}

		// arr[3] = timestamp (string)
		var ts string
		if err := json.Unmarshal(arr[3], &ts); err == nil {
			evt.Timestamp = ts
		}

		// Some Patroni versions include from_node and to_node at index 4 and 5.
		if len(arr) >= 5 {
			var fromNode string
			if err := json.Unmarshal(arr[3], &fromNode); err == nil {
				evt.FromNode = fromNode
			}
		}
		if len(arr) >= 6 {
			var toNode string
			if err := json.Unmarshal(arr[4], &toNode); err == nil {
				evt.ToNode = toNode
			}
		}

		events = append(events, evt)
	}

	return events, nil
}

// patroniRootResponse matches the JSON returned by GET /.
type patroniRootResponse struct {
	Patroni struct {
		Version string `json:"version"`
		Scope   string `json:"scope"`
	} `json:"patroni"`
}

// GetVersion retrieves Patroni version via GET /.
func (r *RESTProvider) GetVersion(ctx context.Context) (string, error) {
	body, err := r.doGet(ctx, "/")
	if err != nil {
		return "", err
	}

	var resp patroniRootResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("patroni rest: parse root response: %w", err)
	}

	if resp.Patroni.Version == "" {
		return "", fmt.Errorf("%w: empty version in response", ErrPatroniUnavailable)
	}

	return resp.Patroni.Version, nil
}

// doGet performs a GET request and returns the response body.
func (r *RESTProvider) doGet(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("patroni rest: create request: %w", err)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrPatroniUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("patroni rest: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrPatroniUnavailable, resp.StatusCode)
	}

	return body, nil
}
