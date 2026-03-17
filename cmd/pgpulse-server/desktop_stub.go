//go:build !desktop

package main

import (
	"fmt"
	"io/fs"
	"net/http"
)

// GetDesktopMode always returns "server" when built without the desktop tag.
func GetDesktopMode() string { return "server" }

// RunDesktop returns an error when the binary was built without -tags desktop.
func RunDesktop(_ http.Handler, _ fs.FS) error {
	return fmt.Errorf("desktop mode not available: binary built without -tags desktop")
}
