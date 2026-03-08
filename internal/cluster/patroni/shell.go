package patroni

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ShellProvider uses patronictl via os/exec to retrieve cluster information.
type ShellProvider struct {
	ctlPath    string
	configPath string
}

// NewShellProvider creates a ShellProvider that invokes patronictl at ctlPath
// with the given config file.
func NewShellProvider(ctlPath, configPath string) *ShellProvider {
	return &ShellProvider{ctlPath: ctlPath, configPath: configPath}
}

// patroniCtlMember matches the JSON output of `patronictl list -f json`.
type patroniCtlMember struct {
	Member  string `json:"Member"`
	Host    string `json:"Host"`
	Role    string `json:"Role"`
	State   string `json:"State"`
	TL      int    `json:"TL"`
	LagInMB int64  `json:"Lag in MB"`
	Cluster string `json:"Cluster"`
}

// GetClusterState runs `patronictl list -f json` and parses the output.
func (s *ShellProvider) GetClusterState(ctx context.Context) (*ClusterState, error) {
	out, err := s.run(ctx, "list", "-f", "json")
	if err != nil {
		return nil, err
	}

	var members []patroniCtlMember
	if err := json.Unmarshal(out, &members); err != nil {
		return nil, fmt.Errorf("patroni shell: parse list output: %w", err)
	}

	state := &ClusterState{
		Members: make([]ClusterMember, len(members)),
	}

	for i, m := range members {
		if i == 0 && m.Cluster != "" {
			state.ClusterName = m.Cluster
		}
		state.Members[i] = ClusterMember{
			Name:     m.Member,
			Host:     m.Host,
			Role:     strings.ToLower(m.Role),
			State:    m.State,
			Timeline: m.TL,
			Lag:      m.LagInMB * 1024 * 1024, // convert MB to bytes
			Port:     0,                        // not available in patronictl output
		}
	}

	return state, nil
}

// GetHistory runs `patronictl history` and parses the tabular output.
// The output format is:
//
//	+----+----------+------------------------------+------------------+
//	| TL |      LSN | Reason                       | Timestamp        |
//	+----+----------+------------------------------+------------------+
//	|  1 | 12345678 | no recovery target specified | 2026-03-01T10:00 |
//	+----+----------+------------------------------+------------------+
func (s *ShellProvider) GetHistory(ctx context.Context) ([]SwitchoverEvent, error) {
	out, err := s.run(ctx, "history")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(out), "\n")
	var events []SwitchoverEvent

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip empty lines, separators, and headers.
		if line == "" || strings.HasPrefix(line, "+") {
			continue
		}
		if !strings.HasPrefix(line, "|") {
			continue
		}

		parts := strings.Split(line, "|")
		// We expect at least 5 parts: empty, TL, LSN, Reason, Timestamp, empty
		if len(parts) < 5 {
			continue
		}

		tl := strings.TrimSpace(parts[1])
		reason := strings.TrimSpace(parts[3])
		ts := strings.TrimSpace(parts[4])

		// Skip header row.
		if tl == "TL" {
			continue
		}

		events = append(events, SwitchoverEvent{
			Timestamp: ts,
			Reason:    reason,
		})
	}

	return events, nil
}

// GetVersion runs `patronictl version` and parses the output.
// Expected output: "patronictl version 3.0.4"
func (s *ShellProvider) GetVersion(ctx context.Context) (string, error) {
	out, err := s.run(ctx, "version")
	if err != nil {
		return "", err
	}

	output := strings.TrimSpace(string(out))
	// Extract version from "patronictl version X.Y.Z"
	parts := strings.Fields(output)
	if len(parts) >= 3 {
		return parts[len(parts)-1], nil
	}

	return output, nil
}

// run executes patronictl with the given arguments.
func (s *ShellProvider) run(ctx context.Context, args ...string) ([]byte, error) {
	if _, err := os.Stat(s.ctlPath); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrPatroniUnavailable, err)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var cmdArgs []string
	if s.configPath != "" {
		cmdArgs = append(cmdArgs, "-c", s.configPath)
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(cmdCtx, s.ctlPath, cmdArgs...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: patronictl %s: %s", ErrPatroniUnavailable, strings.Join(args, " "), err)
	}

	return out, nil
}
