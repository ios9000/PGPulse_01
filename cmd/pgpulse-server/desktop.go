//go:build desktop

package main

import (
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ios9000/PGPulse_01/internal/alert"
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
func RunDesktop(router http.Handler, assets fs.FS, onAlertHook func(func(alert.AlertEvent))) error {
	app, err := desktop.NewDesktopApp(desktop.Options{
		Router:      router,
		WebFS:       assets,
		OnAlertHook: onAlertHook,
	})
	if err != nil {
		return err
	}
	return app.Run()
}

// ResolveConfigDesktop shows a connection dialog if the config file does not
// exist and returns the resolved config file path. In "quickconnect" mode it
// writes a temporary config so the rest of main.go can Load() it normally.
func ResolveConfigDesktop(configPath string) (string, error) {
	// If the config file exists, use it directly.
	if _, err := os.Stat(configPath); err == nil {
		return configPath, nil
	}

	// Load saved settings for last-used config path.
	settings, _ := desktop.LoadSettings()

	result, err := desktop.ShowConfigDialog(settings.LastConfigPath)
	if err != nil {
		return "", fmt.Errorf("config dialog failed: %w", err)
	}

	switch result.Mode {
	case "config":
		// Save the chosen config path for next time.
		settings.LastConfigPath = result.ConfigPath
		_ = desktop.SaveSettings(settings)
		return result.ConfigPath, nil

	case "quickconnect":
		// Write a minimal temp config with the provided DSN.
		tmpDir, err := os.MkdirTemp("", "pgpulse-")
		if err != nil {
			return "", fmt.Errorf("create temp dir: %w", err)
		}
		tmpConfig := filepath.Join(tmpDir, "pgpulse.yml")
		dsn := strings.ReplaceAll(result.DSN, `\`, `\\`)
		content := fmt.Sprintf("instances:\n  - id: desktop-quick\n    name: Quick Connect\n    dsn: \"%s\"\n", dsn)
		if err := os.WriteFile(tmpConfig, []byte(content), 0o644); err != nil {
			return "", fmt.Errorf("write temp config: %w", err)
		}
		return tmpConfig, nil

	case "cancel":
		return "", fmt.Errorf("user cancelled configuration")

	default:
		return "", fmt.Errorf("unexpected dialog result: %s", result.Mode)
	}
}
