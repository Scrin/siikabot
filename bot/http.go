package bot

import (
	"time"

	"github.com/Scrin/siikabot/api"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/metrics"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func initHTTP() {
	// Set Gin to release mode for production
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()

	// Global middleware. otelgin goes on before anything else so every route is traced, including
	// /metrics and the healthcheck — Tempo already handles far more than this bot can produce, and a
	// filter is one more place for a route to end up silently untraced.
	//
	// It also extracts the inbound traceparent, which is what makes a webhook span a child of the
	// nginx span in front of it rather than the root of an unrelated trace.
	router.Use(otelgin.Middleware("siikabot"))
	router.Use(gin.Recovery())
	router.Use(requestLoggingMiddleware())

	// API routes group
	apiGroup := router.Group("/api")
	{
		// Public endpoints
		apiGroup.GET("/healthcheck", api.HealthCheckHandler)
		apiGroup.GET("/metrics", api.MetricsHandler)

		// Auth endpoints (mixed public/protected)
		authGroup := apiGroup.Group("/auth")
		{
			authGroup.POST("/challenge", api.ChallengeHandler)
			authGroup.GET("/poll", api.PollHandler)
			authGroup.GET("/me", api.MeHandler)
			authGroup.POST("/logout", api.LogoutHandler)
		}

		// Protected endpoints (require authentication)
		protectedGroup := apiGroup.Group("")
		protectedGroup.Use(api.AuthMiddleware())
		{
			protectedGroup.GET("/reminders", api.RemindersHandler)
			protectedGroup.GET("/rooms", api.RoomsHandler)
			protectedGroup.GET("/rooms/:roomId/members", api.RoomMembersHandler)
			protectedGroup.GET("/memories", api.MemoriesHandler)
			protectedGroup.DELETE("/memories", api.DeleteAllMemoriesHandler)
			protectedGroup.DELETE("/memories/:id", api.DeleteMemoryHandler)

			// Admin routes (require admin authorization)
			adminGroup := protectedGroup.Group("/admin")
			adminGroup.Use(api.AdminAuthMiddleware())
			{
				adminGroup.GET("/rooms", api.AdminRoomsHandler)
				adminGroup.GET("/rooms/:roomId/members", api.AdminRoomMembersHandler)
				adminGroup.GET("/chat-usage", api.ChatUsageHandler)
			}
		}
	}

	// Webhook routes group
	hooksGroup := router.Group("/hooks")
	{
		// GitHub webhook with HMAC signature verification
		githubGroup := hooksGroup.Group("/github")
		githubGroup.Use(GithubSignatureMiddleware(config.HookSecret))
		githubGroup.POST("", GithubWebhookHandler)

		// Alertmanager webhook with Basic Auth
		alertmanagerGroup := hooksGroup.Group("/alertmanager")
		alertmanagerGroup.Use(AlertmanagerBasicAuthMiddleware(config.AlertmanagerUser, config.AlertmanagerPassword))
		alertmanagerGroup.POST("", AlertmanagerWebhookHandler)
	}

	// Prometheus metrics endpoint
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Static file serving with SPA fallback (must be last)
	SetupStaticRoutes(router)

	// Start the server
	go func() {
		log.Info().Msg("Starting HTTP server on :8080")
		if err := router.Run(":8080"); err != nil {
			log.Error().Err(err).Msg("HTTP server failed")
		}
	}()
}

// requestLoggingMiddleware logs HTTP requests and records metrics using zerolog
func requestLoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		rawPath := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		log.Debug().
			Int("status", status).
			Str("method", c.Request.Method).
			Str("path", rawPath).
			Str("query", query).
			Dur("latency", latency).
			Str("client_ip", c.ClientIP()).
			Msg("HTTP request")

		// Use route template for metrics to avoid high-cardinality labels
		metricsPath := c.FullPath()
		if metricsPath == "" {
			metricsPath = "unmatched"
		}
		metrics.RecordHTTPRequest(c.Request.Method, metricsPath, status, latency.Seconds())
	}
}
