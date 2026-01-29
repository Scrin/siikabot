package bot

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
)

//go:embed all:frontend/dist
var frontendFS embed.FS

// getStaticHandler returns a handler that serves static files from the embedded frontend
// It falls back to index.html for client-side routing
func getStaticHandler() http.Handler {
	// Get the dist subdirectory from the embedded filesystem
	distFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		log.Warn().Err(err).Msg("Failed to access embedded frontend files, static serving will be unavailable")
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Frontend not available", http.StatusNotFound)
		})
	}

	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// For paths that might be client-side routes, try serving the file first
		// If it doesn't exist, serve index.html for SPA routing
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// Check if the file exists
		if _, err := fs.Stat(distFS, path); err != nil {
			// File doesn't exist, serve index.html for client-side routing
			r.URL.Path = "/"
		}

		fileServer.ServeHTTP(w, r)
	})
}
