//go:build !linux

package agent

import "errors"

// ErrOSMetricsUnavailable is returned when OS metrics collection
// is attempted on a non-Linux platform.
var ErrOSMetricsUnavailable = errors.New("OS metrics only available on Linux")

// CollectOS returns ErrOSMetricsUnavailable on non-Linux platforms.
func CollectOS(_ []string) (*OSSnapshot, error) {
	return nil, ErrOSMetricsUnavailable
}
