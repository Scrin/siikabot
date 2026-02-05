//go:build !embed

package bot

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// SetupStaticRoutes is a stub when building without embedded frontend
func SetupStaticRoutes(router *gin.Engine) {
	router.NoRoute(func(c *gin.Context) {
		c.String(http.StatusNotFound, "Frontend not embedded in this build")
	})
}
