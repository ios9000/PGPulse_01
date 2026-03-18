//go:build desktop

package desktop

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// DialogResult communicates the user's choice from the config dialog.
type DialogResult struct {
	Mode       string // "config", "quickconnect", or "cancel"
	ConfigPath string // populated when Mode == "config"
	DSN        string // populated when Mode == "quickconnect"
}

// DialogService is a Wails-bound service that backs the connection dialog UI.
type DialogService struct {
	result chan DialogResult
	app    *application.App
}

// NewDialogService creates a DialogService with the given application reference.
func NewDialogService(app *application.App) *DialogService {
	return &DialogService{
		result: make(chan DialogResult, 1),
		app:    app,
	}
}

// OpenFilePicker opens a native file dialog filtered to YAML config files
// and sends the chosen path as a config-mode result.
func (ds *DialogService) OpenFilePicker() {
	path, err := ds.app.Dialog.OpenFile().
		SetTitle("Select PGPulse Config File").
		AddFilter("YAML Config", "*.yml;*.yaml").
		AddFilter("All Files", "*.*").
		PromptForSingleSelection()
	if err != nil || path == "" {
		// User cancelled the file picker — do nothing (don't close the dialog).
		return
	}
	ds.result <- DialogResult{Mode: "config", ConfigPath: path}
	ds.app.Quit()
}

// SubmitDSN sends a quick-connect result with the given DSN string.
func (ds *DialogService) SubmitDSN(dsn string) {
	ds.result <- DialogResult{Mode: "quickconnect", DSN: dsn}
	ds.app.Quit()
}

// UseLastConfig sends a config-mode result with the previously saved path.
func (ds *DialogService) UseLastConfig(path string) {
	ds.result <- DialogResult{Mode: "config", ConfigPath: path}
	ds.app.Quit()
}

// Cancel sends a cancel result and quits the dialog app.
func (ds *DialogService) Cancel() {
	ds.result <- DialogResult{Mode: "cancel"}
	ds.app.Quit()
}
