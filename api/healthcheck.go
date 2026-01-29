package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
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
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	response := HealthCheckResponse{
		Status: "ok",
		Uptime: time.Since(startTime).Round(time.Second).String(),
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	// Encode and send response
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to encode healthcheck response")
	}
}
