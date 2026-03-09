package ml

import "github.com/ios9000/PGPulse_01/internal/mlerrors"

// Re-export sentinel errors from mlerrors for backward compatibility.
var (
	ErrNotBootstrapped = mlerrors.ErrNotBootstrapped
	ErrNoBaseline      = mlerrors.ErrNoBaseline
)
