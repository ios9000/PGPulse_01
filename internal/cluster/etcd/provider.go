package etcd

import (
	"context"
	"errors"
)

var (
	// ErrETCDNotConfigured is returned when no ETCD config is provided.
	ErrETCDNotConfigured = errors.New("etcd not configured")

	// ErrETCDUnavailable is returned when ETCD cannot be reached.
	ErrETCDUnavailable = errors.New("etcd unavailable")
)

// ETCDConfig holds configuration for building an ETCD provider.
type ETCDConfig struct {
	Endpoints []string
	CtlPath   string
}

// ETCDMember represents a single node in an ETCD cluster.
type ETCDMember struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	PeerURL   string `json:"peer_url"`
	ClientURL string `json:"client_url"`
	IsLeader  bool   `json:"is_leader"`
	Status    string `json:"status"`
	DBSize    int64  `json:"db_size"`
}

// ETCDProvider defines the interface for interacting with an ETCD cluster.
type ETCDProvider interface {
	GetMembers(ctx context.Context) ([]ETCDMember, error)
	GetEndpointHealth(ctx context.Context) (map[string]bool, error)
}

// NewProvider builds the right provider chain based on config.
// If both endpoints and etcdctl path are configured, a FallbackProvider
// is created with HTTP as primary and shell as secondary.
func NewProvider(cfg ETCDConfig) ETCDProvider {
	var providers []ETCDProvider

	if len(cfg.Endpoints) > 0 {
		providers = append(providers, NewHTTPProvider(cfg.Endpoints))
	}
	if cfg.CtlPath != "" {
		providers = append(providers, NewShellProvider(cfg.CtlPath, cfg.Endpoints))
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
