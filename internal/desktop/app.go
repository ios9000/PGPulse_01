//go:build desktop

package desktop

import (
	"io/fs"
	"net/http"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Options configures the desktop application.
type Options struct {
	// Router is the chi HTTP handler that serves both /api/v1/* and static files.
	Router http.Handler
	// WebFS is the embedded frontend filesystem (result of fs.Sub(web.DistFS, "dist")).
	WebFS fs.FS
}

// DesktopApp wraps the Wails application, main window, and system tray.
type DesktopApp struct {
	app    *application.App
	window *application.WebviewWindow
	tray   *SystemTray
}

// NewDesktopApp creates and configures a Wails v3 desktop application.
// The chi router handles ALL HTTP requests (API + frontend) inside the webview.
func NewDesktopApp(opts Options) (*DesktopApp, error) {
	app := application.New(application.Options{
		Name: "PGPulse",
		Icon: IconWindow,
		Assets: application.AssetOptions{
			Handler: opts.Router,
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
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

	return &DesktopApp{
		app:    app,
		window: window,
		tray:   tray,
	}, nil
}

// Run starts the Wails event loop. Blocks until the application exits.
func (d *DesktopApp) Run() error {
	return d.app.Run()
}
