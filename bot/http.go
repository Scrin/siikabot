package bot

import (
	"net/http"

	"github.com/Scrin/siikabot/api"
	"github.com/Scrin/siikabot/config"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

func initHTTP() {
	// Create a new mux to have better control over routing
	mux := http.NewServeMux()

	// Register specific API endpoints first (these take priority)
	mux.HandleFunc("/api/healthcheck", api.HealthCheckHandler)
	mux.HandleFunc("/api/auth/challenge", api.ChallengeHandler)
	mux.HandleFunc("/api/auth/poll", api.PollHandler)
	mux.HandleFunc("/api/auth/me", api.MeHandler)
	mux.HandleFunc("/api/auth/logout", api.LogoutHandler)
	mux.Handle("/api/reminders", api.AuthMiddleware(http.HandlerFunc(api.RemindersHandler)))
	mux.Handle("/api/rooms", api.AuthMiddleware(http.HandlerFunc(api.RoomsHandler)))
	mux.Handle("/api/grafana/templates", api.AuthMiddleware(http.HandlerFunc(api.GrafanaTemplatesHandler)))
	mux.Handle("/api/grafana/templates/", api.AuthMiddleware(http.HandlerFunc(api.GrafanaTemplateRouteHandler)))
	mux.HandleFunc("/api/metrics", api.MetricsHandler)
	mux.HandleFunc("/hooks/github", githubHandler(config.HookSecret))
	mux.Handle("/metrics", promhttp.Handler())

	// Register the catch-all static file handler for everything else
	mux.Handle("/", getStaticHandler())

	// Start the server
	go func() {
		log.Info().Msg("Starting HTTP server on :8080")
		if err := http.ListenAndServe(":8080", mux); err != nil {
			log.Error().Err(err).Msg("HTTP server failed")
		}
	}()
}
