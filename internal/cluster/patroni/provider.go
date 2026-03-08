package patroni

import (
	"context"
	"errors"
)

var (
	// ErrPatroniNotConfigured is returned when no Patroni config is provided.
	ErrPatroniNotConfigured = errors.New("patroni not configured")

	// ErrPatroniUnavailable is returned when Patroni cannot be reached.
	ErrPatroniUnavailable = errors.New("patroni unavailable")
)

// PatroniConfig holds configuration for building a Patroni provider.
type PatroniConfig struct {
	PatroniURL     string
	PatroniConfig  string
	PatroniCtlPath string
}

// ClusterMember represents a single node in a Patroni cluster.
type ClusterMember struct {
	Name     string            `json:"name"`
	Host     string            `json:"host"`
	Port     int               `json:"port"`
	Role     string            `json:"role"`
	State    string            `json:"state"`
	Timeline int               `json:"timeline"`
	Lag      int64             `json:"lag"`
	Tags     map[string]string `json:"tags,omitempty"`
}

// ClusterState represents the full state of a Patroni cluster.
type ClusterState struct {
	ClusterName string          `json:"cluster_name"`
	Members     []ClusterMember `json:"members"`
}

// SwitchoverEvent represents a historical switchover/failover event.
type SwitchoverEvent struct {
	Timestamp string `json:"timestamp"`
	FromNode  string `json:"from_node"`
	ToNode    string `json:"to_node"`
	Reason    string `json:"reason"`
}

// PatroniProvider defines the interface for interacting with Patroni.
type PatroniProvider interface {
	GetClusterState(ctx context.Context) (*ClusterState, error)
	GetHistory(ctx context.Context) ([]SwitchoverEvent, error)
	GetVersion(ctx context.Context) (string, error)
}

// NewProvider builds the right provider chain based on config.
// If both REST URL and patronictl path are configured, a FallbackProvider
// is created with REST as primary and shell as secondary.
func NewProvider(cfg PatroniConfig) PatroniProvider {
	var providers []PatroniProvider

	if cfg.PatroniURL != "" {
		providers = append(providers, NewRESTProvider(cfg.PatroniURL))
	}
	if cfg.PatroniCtlPath != "" || cfg.PatroniConfig != "" {
		ctlPath := cfg.PatroniCtlPath
		if ctlPath == "" {
			ctlPath = "/usr/bin/patronictl"
		}
		providers = append(providers, NewShellProvider(ctlPath, cfg.PatroniConfig))
	}

	switch len(providers) {
	case 0:
		return &NoOpProvider{}
	case 1:
		return providers[0]
	default:
		return NewFallbackProvider(providers[0], providers[1])
	}
}
