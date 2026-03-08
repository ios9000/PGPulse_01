package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ShellProvider uses etcdctl via os/exec to retrieve ETCD cluster information.
type ShellProvider struct {
	ctlPath   string
	endpoints []string
}

// NewShellProvider creates a ShellProvider that invokes etcdctl at ctlPath.
func NewShellProvider(ctlPath string, endpoints []string) *ShellProvider {
	return &ShellProvider{ctlPath: ctlPath, endpoints: endpoints}
}

// etcdCtlMemberListResponse matches the JSON output of `etcdctl member list --write-out=json`.
type etcdCtlMemberListResponse struct {
	Members []struct {
		ID         uint64   `json:"ID"`
		Name       string   `json:"name"`
		PeerURLs   []string `json:"peerURLs"`
		ClientURLs []string `json:"clientURLs"`
	} `json:"members"`
}

// GetMembers runs `etcdctl member list --write-out=json` and parses the output.
func (s *ShellProvider) GetMembers(ctx context.Context) ([]ETCDMember, error) {
	args := []string{"member", "list", "--write-out=json"}
	out, err := s.run(ctx, args...)
	if err != nil {
		return nil, err
	}

	var resp etcdCtlMemberListResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("etcd shell: parse member list: %w", err)
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
			ID:        fmt.Sprintf("%x", m.ID),
			Name:      m.Name,
			PeerURL:   peerURL,
			ClientURL: clientURL,
			IsLeader:  false,
			Status:    "running",
		}
	}

	return members, nil
}

// etcdCtlEndpointHealth matches one entry from `etcdctl endpoint health --write-out=json`.
type etcdCtlEndpointHealth struct {
	Endpoint string `json:"endpoint"`
	Health   bool   `json:"health"`
	Took     string `json:"took"`
}

// GetEndpointHealth runs `etcdctl endpoint health --write-out=json` and parses the output.
func (s *ShellProvider) GetEndpointHealth(ctx context.Context) (map[string]bool, error) {
	args := []string{"endpoint", "health", "--write-out=json"}
	out, err := s.run(ctx, args...)
	if err != nil {
		return nil, err
	}

	var entries []etcdCtlEndpointHealth
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("etcd shell: parse endpoint health: %w", err)
	}

	result := make(map[string]bool, len(entries))
	for _, e := range entries {
		result[e.Endpoint] = e.Health
	}

	return result, nil
}

// run executes etcdctl with the given arguments.
func (s *ShellProvider) run(ctx context.Context, args ...string) ([]byte, error) {
	if _, err := os.Stat(s.ctlPath); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrETCDUnavailable, err)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var cmdArgs []string
	if len(s.endpoints) > 0 {
		cmdArgs = append(cmdArgs, "--endpoints="+strings.Join(s.endpoints, ","))
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(cmdCtx, s.ctlPath, cmdArgs...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: etcdctl %s: %s", ErrETCDUnavailable, strings.Join(args, " "), err)
	}

	return out, nil
}
