package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

var startTime time.Time

// Init initializes the API package
func Init() {
	startTime = time.Now()
}

// HealthCheckResponse represents the response from the healthcheck endpoint
type HealthCheckResponse struct {
	Status   string            `json:"status"`
	Uptime   string            `json:"uptime"`
	Services map[string]string `json:"services"`
}

// HealthCheckHandler returns the health status of the bot
func HealthCheckHandler(c *gin.Context) {
	c.JSON(http.StatusOK, HealthCheckResponse{
		Status: "ok",
		Uptime: time.Since(startTime).Round(time.Second).String(),
	})
}
