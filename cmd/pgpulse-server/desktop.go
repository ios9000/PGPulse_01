//go:build desktop

package main

import (
	"flag"
	"io/fs"
	"net/http"

	"github.com/ios9000/PGPulse_01/internal/desktop"
)

var desktopMode string

func init() {
	flag.StringVar(&desktopMode, "mode", "server", "Run mode: server or desktop")
}

// GetDesktopMode returns the current run mode (server or desktop).
func GetDesktopMode() string {
	return desktopMode
}

// RunDesktop starts the Wails-based desktop application.
func RunDesktop(router http.Handler, assets fs.FS) error {
	app, err := desktop.NewDesktopApp(desktop.Options{
		Router: router,
		WebFS:  assets,
	})
	if err != nil {
		return err
	}
	return app.Run()
}
