//go:build desktop

package desktop

import (
	_ "embed"
	"net/http"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed dialog.html
var dialogHTML string

// dialogHandler serves the dialog HTML with template substitution.
type dialogHandler struct {
	lastConfigPath string
}

func (h *dialogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	html := dialogHTML

	// Inject last config path.
	lastClass := ""
	if h.lastConfigPath == "" {
		lastClass = "disabled"
	}
	html = strings.ReplaceAll(html, "{{LAST_CONFIG_PATH}}", h.lastConfigPath)
	html = strings.ReplaceAll(html, "{{LAST_CLASS}}", lastClass)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

// ShowConfigDialog displays a modal connection dialog and blocks until the user
// makes a choice. It creates a separate Wails application with its own event
// loop so the dialog lifecycle is independent from the main app.
func ShowConfigDialog(lastConfigPath string) (DialogResult, error) {
	handler := &dialogHandler{lastConfigPath: lastConfigPath}

	app := application.New(application.Options{
		Name: "PGPulse — Setup",
		Assets: application.AssetOptions{
			Handler: handler,
		},
	})

	svc := NewDialogService(app)
	app.RegisterService(application.NewService(svc))

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:            "dialog",
		Title:           "PGPulse — Welcome",
		Width:           600,
		Height:          400,
		MinWidth:        600,
		MinHeight:       400,
		DisableResize:    true,
		InitialPosition:  application.WindowCentered,
		URL:              "/",
		BackgroundColour: application.NewRGBA(26, 31, 46, 255),
	})

	// Run the dialog app in a goroutine; it blocks until Quit() is called.
	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run()
	}()

	// Wait for the service to send a result or the app to exit.
	select {
	case result := <-svc.result:
		// App will quit itself; wait for Run to finish.
		<-errCh
		return result, nil
	case err := <-errCh:
		// App closed without sending a result (e.g. window closed).
		select {
		case result := <-svc.result:
			return result, nil
		default:
			return DialogResult{Mode: "cancel"}, err
		}
	}
}
