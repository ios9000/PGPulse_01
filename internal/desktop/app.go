//go:build desktop

package desktop

import (
	"io/fs"
	"net/http"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/ios9000/PGPulse_01/internal/alert"
)

// Options configures the desktop application.
type Options struct {
	// Router is the chi HTTP handler that serves both /api/v1/* and static files.
	Router http.Handler
	// WebFS is the embedded frontend filesystem (result of fs.Sub(web.DistFS, "dist")).
	WebFS fs.FS
	// OnAlertHook registers a callback on the alert dispatcher. Nil-safe.
	OnAlertHook func(fn func(alert.AlertEvent))
}

// DesktopApp wraps the Wails application, main window, and system tray.
type DesktopApp struct {
	app      *application.App
	window   *application.WebviewWindow
	tray     *SystemTray
	notifier *AlertNotifier
	done     chan struct{}
}

// NewDesktopApp creates and configures a Wails v3 desktop application.
// The chi router handles ALL HTTP requests (API + frontend) inside the webview.
func NewDesktopApp(opts Options) (*DesktopApp, error) {
	// Create notification service.
	notifSvc := notifications.New()

	app := application.New(application.Options{
		Name: "PGPulse",
		Icon: IconWindow,
		Assets: application.AssetOptions{
			Handler: opts.Router,
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		Services: []application.Service{
			application.NewService(notifSvc),
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "PGPulse",
		Width:     1440,
		Height:    900,
		MinWidth:  1024,
		MinHeight: 700,
		URL:       "/",
	})

	// Hide window instead of quitting on close.
	window.OnWindowEvent(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		window.Hide()
	})

	tray := NewSystemTray(app, window)

	// Create alert notifier and wire to dispatcher hook.
	alertNotifier := NewAlertNotifier(notifSvc, window)
	if opts.OnAlertHook != nil {
		opts.OnAlertHook(alertNotifier.HandleAlert)
	}

	da := &DesktopApp{
		app:      app,
		window:   window,
		tray:     tray,
		notifier: alertNotifier,
		done:     make(chan struct{}),
	}

	// Start tray status ticker (placeholder until orchestrator state is wired).
	go da.trayStatusLoop()

	return da, nil
}

// trayStatusLoop updates the tray icon on a 10-second interval.
// Currently uses placeholder values; will be wired to orchestrator state later.
func (d *DesktopApp) trayStatusLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Set initial status.
	d.tray.UpdateStatus(0, 0, "ok")

	for {
		select {
		case <-ticker.C:
			d.tray.UpdateStatus(0, 0, "ok")
		case <-d.done:
			return
		}
	}
}

// Run starts the Wails event loop. Blocks until the application exits.
func (d *DesktopApp) Run() error {
	return d.app.Run()
}

// Shutdown performs cleanup of desktop resources.
func (d *DesktopApp) Shutdown() {
	close(d.done)
}
