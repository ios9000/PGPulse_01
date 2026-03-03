package api

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/ios9000/PGPulse_01/web"
)

// staticHandler serves the embedded SPA frontend.
// API routes (/api/) are handled separately and take priority.
// All other routes serve the SPA with index.html fallback.
func (s *APIServer) staticHandler() http.Handler {
	distFS, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		panic("failed to create sub filesystem: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Try to serve the exact file.
		if path != "/" && !strings.HasSuffix(path, "/") {
			if f, err := distFS.Open(strings.TrimPrefix(path, "/")); err == nil {
				_ = f.Close()
				// Set cache headers for hashed assets.
				if strings.Contains(path, "/assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// SPA fallback: serve index.html for all non-file routes.
		// This lets the frontend router handle client-side routing.
		w.Header().Set("Cache-Control", "no-cache")
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
