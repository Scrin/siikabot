package bot

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

//go:embed all:frontend/dist
var frontendFS embed.FS

// SetupStaticRoutes configures static file serving with SPA fallback for Gin
func SetupStaticRoutes(router *gin.Engine) {
	distFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		log.Warn().Err(err).Msg("Failed to access embedded frontend files, static serving will be unavailable")
		return
	}

	// Use NoRoute handler for SPA fallback
	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// Don't serve index.html for API routes that 404
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/hooks/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
			return
		}

		// Try to serve the requested file
		filePath := strings.TrimPrefix(path, "/")
		if filePath == "" {
			filePath = "index.html"
		}

		// Check if the file exists in the embedded filesystem
		if file, err := distFS.Open(filePath); err == nil {
			file.Close()
			// File exists, serve it
			c.FileFromFS(path, http.FS(distFS))
			return
		}

		// File doesn't exist, serve index.html for SPA routing
		indexContent, err := fs.ReadFile(distFS, "index.html")
		if err != nil {
			c.String(http.StatusNotFound, "Frontend not available")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexContent)
	})
}
