//go:build desktop

package desktop

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// SystemTray manages the PGPulse system tray icon and context menu.
type SystemTray struct {
	tray       *application.SystemTray
	window     *application.WebviewWindow
	statusItem *application.MenuItem
}

// NewSystemTray creates and configures the system tray with icon, tooltip, and context menu.
func NewSystemTray(app *application.App, window *application.WebviewWindow) *SystemTray {
	menu := application.NewMenu()
	showItem := menu.Add("Show PGPulse")
	menu.AddSeparator()
	statusItem := menu.Add("Status: Monitoring...")
	statusItem.SetEnabled(false)
	menu.AddSeparator()
	quitItem := menu.Add("Quit")

	tray := app.SystemTray.New()
	tray.SetIcon(IconTrayDefault)
	tray.SetTooltip("PGPulse — PostgreSQL Monitor")
	tray.SetMenu(menu)
	tray.AttachWindow(window)

	showItem.OnClick(func(_ *application.Context) {
		window.Show().Focus()
	})

	quitItem.OnClick(func(_ *application.Context) {
		app.Quit()
	})

	return &SystemTray{
		tray:       tray,
		window:     window,
		statusItem: statusItem,
	}
}

// UpdateStatus changes the tray icon and tooltip based on monitoring state.
// maxSeverity must be "ok", "warning", or "critical".
func (s *SystemTray) UpdateStatus(instanceCount, alertCount int, maxSeverity string) {
	var icon []byte
	switch maxSeverity {
	case "critical":
		icon = IconTrayCritical
	case "warning":
		icon = IconTrayWarning
	default:
		icon = IconTrayDefault
	}
	s.tray.SetIcon(icon)

	tooltip := fmt.Sprintf("PGPulse — %d instance(s), %d alert(s)", instanceCount, alertCount)
	s.tray.SetTooltip(tooltip)

	status := fmt.Sprintf("Status: %d instance(s), %d alert(s)", instanceCount, alertCount)
	s.statusItem.SetLabel(status)
}
