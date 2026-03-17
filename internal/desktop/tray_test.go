//go:build desktop

package desktop

import (
	"testing"
)

func TestUpdateStatus_IconSelection(t *testing.T) {
	// UpdateStatus changes the tray icon based on maxSeverity.
	// Verifying Wails tray behavior requires a running app — manual test only.
	// Document the expected behavior:
	//   "ok"       → IconTrayDefault (green)
	//   "warning"  → IconTrayWarning (yellow)
	//   "critical" → IconTrayCritical (red)
	t.Log("SystemTray.UpdateStatus requires Wails runtime — manual test only")
}
