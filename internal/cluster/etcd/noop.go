package etcd

import "context"

// NoOpProvider is returned when no ETCD configuration is provided.
// All methods return ErrETCDNotConfigured.
type NoOpProvider struct{}

// GetMembers returns ErrETCDNotConfigured.
func (n *NoOpProvider) GetMembers(_ context.Context) ([]ETCDMember, error) {
	return nil, ErrETCDNotConfigured
}

// GetEndpointHealth returns ErrETCDNotConfigured.
func (n *NoOpProvider) GetEndpointHealth(_ context.Context) (map[string]bool, error) {
	return nil, ErrETCDNotConfigured
}
