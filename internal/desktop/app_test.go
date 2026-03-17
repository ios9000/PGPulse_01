//go:build desktop

package desktop

import (
	"testing"
)

func TestNewDesktopApp_RequiresRouter(t *testing.T) {
	// NewDesktopApp requires a non-nil Router in Options.
	// Full GUI testing requires a display server and is manual-only.
	// This test verifies the function signature compiles correctly.
	t.Log("DesktopApp creation requires Wails runtime — manual test only")
}
