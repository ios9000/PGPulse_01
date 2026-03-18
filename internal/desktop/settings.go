//go:build desktop

package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// AppSettings persists user preferences between desktop sessions.
type AppSettings struct {
	LastConfigPath string `json:"last_config_path,omitempty"`
}

// settingsPath returns the platform-appropriate settings file path.
// On Windows: %APPDATA%/PGPulse/settings.json
func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "PGPulse", "settings.json"), nil
}

// LoadSettings reads settings from the user config directory.
// Returns empty settings if the file does not exist.
func LoadSettings() (AppSettings, error) {
	p, err := settingsPath()
	if err != nil {
		return AppSettings{}, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return AppSettings{}, nil
		}
		return AppSettings{}, err
	}

	var s AppSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return AppSettings{}, err
	}
	return s, nil
}

// SaveSettings writes settings to the user config directory, creating the
// directory if it does not exist.
func SaveSettings(s AppSettings) error {
	p, err := settingsPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
