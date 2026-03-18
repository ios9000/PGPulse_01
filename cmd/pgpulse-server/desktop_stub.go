//go:build !desktop

package main

import (
	"fmt"
	"io/fs"
	"net/http"

	"github.com/ios9000/PGPulse_01/internal/alert"
)

// GetDesktopMode always returns "server" when built without the desktop tag.
func GetDesktopMode() string { return "server" }

// RunDesktop returns an error when the binary was built without -tags desktop.
func RunDesktop(_ http.Handler, _ fs.FS, _ func(func(alert.AlertEvent))) error {
	return fmt.Errorf("desktop mode not available: binary built without -tags desktop")
}

// ResolveConfigDesktop is a no-op in server mode — returns the path unchanged.
func ResolveConfigDesktop(configPath string) (string, error) {
	return configPath, nil
}
