package patroni

import "context"

// FallbackProvider tries the primary PatroniProvider first; if it returns an error,
// the secondary is attempted.
type FallbackProvider struct {
	primary   PatroniProvider
	secondary PatroniProvider
}

// NewFallbackProvider creates a FallbackProvider with the given primary and secondary.
func NewFallbackProvider(primary, secondary PatroniProvider) *FallbackProvider {
	return &FallbackProvider{primary: primary, secondary: secondary}
}

// GetClusterState tries primary, falls back to secondary on error.
func (f *FallbackProvider) GetClusterState(ctx context.Context) (*ClusterState, error) {
	state, err := f.primary.GetClusterState(ctx)
	if err != nil {
		return f.secondary.GetClusterState(ctx)
	}
	return state, nil
}

// GetHistory tries primary, falls back to secondary on error.
func (f *FallbackProvider) GetHistory(ctx context.Context) ([]SwitchoverEvent, error) {
	history, err := f.primary.GetHistory(ctx)
	if err != nil {
		return f.secondary.GetHistory(ctx)
	}
	return history, nil
}

// GetVersion tries primary, falls back to secondary on error.
func (f *FallbackProvider) GetVersion(ctx context.Context) (string, error) {
	version, err := f.primary.GetVersion(ctx)
	if err != nil {
		return f.secondary.GetVersion(ctx)
	}
	return version, nil
}
