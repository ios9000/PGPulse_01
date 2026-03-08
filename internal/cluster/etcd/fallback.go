package etcd

import "context"

// FallbackProvider tries the primary ETCDProvider first; if it returns an error,
// the secondary is attempted.
type FallbackProvider struct {
	primary   ETCDProvider
	secondary ETCDProvider
}

// NewFallbackProvider creates a FallbackProvider with the given primary and secondary.
func NewFallbackProvider(primary, secondary ETCDProvider) *FallbackProvider {
	return &FallbackProvider{primary: primary, secondary: secondary}
}

// GetMembers tries primary, falls back to secondary on error.
func (f *FallbackProvider) GetMembers(ctx context.Context) ([]ETCDMember, error) {
	members, err := f.primary.GetMembers(ctx)
	if err != nil {
		return f.secondary.GetMembers(ctx)
	}
	return members, nil
}

// GetEndpointHealth tries primary, falls back to secondary on error.
func (f *FallbackProvider) GetEndpointHealth(ctx context.Context) (map[string]bool, error) {
	health, err := f.primary.GetEndpointHealth(ctx)
	if err != nil {
		return f.secondary.GetEndpointHealth(ctx)
	}
	return health, nil
}
