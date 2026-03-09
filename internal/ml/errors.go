package ml

import "errors"

// Sentinel errors for forecast operations.
var (
	ErrNotBootstrapped = errors.New("ml detector not yet bootstrapped")
	ErrNoBaseline      = errors.New("no fitted baseline for this metric")
)
