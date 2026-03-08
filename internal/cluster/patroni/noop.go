package patroni

import "context"

// NoOpProvider is returned when no Patroni configuration is provided.
// All methods return ErrPatroniNotConfigured.
type NoOpProvider struct{}

// GetClusterState returns ErrPatroniNotConfigured.
func (n *NoOpProvider) GetClusterState(_ context.Context) (*ClusterState, error) {
	return nil, ErrPatroniNotConfigured
}

// GetHistory returns ErrPatroniNotConfigured.
func (n *NoOpProvider) GetHistory(_ context.Context) ([]SwitchoverEvent, error) {
	return nil, ErrPatroniNotConfigured
}

// GetVersion returns ErrPatroniNotConfigured.
func (n *NoOpProvider) GetVersion(_ context.Context) (string, error) {
	return "", ErrPatroniNotConfigured
}
