package bot

import (
	"bytes"
	"net/http"
	"text/template"
	"time"

	"github.com/Scrin/siikabot/constants"
	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/metrics"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

type AlertmanagerPayload struct {
	Version     string            `json:"version"`
	Status      string            `json:"status"`
	Alerts      []Alert           `json:"alerts"`
	ExternalURL string            `json:"externalURL"`
	GroupLabels map[string]string `json:"groupLabels"`
}

type Alert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
}

const alertMessageTemplate = `<h4>{{ if eq .Status "resolved" }}✅{{ else }}⚠️{{ end }} {{ index .Annotations "description" }}</h4>
Started at <b>{{ .StartsAt.Format "Jan 02, 2006 15:04:05 UTC" }}</b>{{ if eq .Status "resolved" }}, ended at <b>{{ .EndsAt.Format "Jan 02, 2006 15:04:05 UTC" }}</b>{{ end }}<br>
{{- range $key, $value := .Labels }}
{{ $key }}: <b>{{ $value }}</b><br>
{{- end }}`

var alertTemplate = template.Must(template.New("alert").Parse(alertMessageTemplate))

func sendAlertmanagerMsg(payload AlertmanagerPayload, roomID string) {
	log.Debug().
		Str("room_id", roomID).
		Str("status", payload.Status).
		Int("alert_count", len(payload.Alerts)).
		Msg("Processing Alertmanager webhook")

	for _, alert := range payload.Alerts {
		metrics.RecordWebhookEventHandled(constants.WebhookAlertmanager, constants.WebhookEventType(alert.Status))
		var buf bytes.Buffer
		err := alertTemplate.Execute(&buf, alert)
		if err != nil {
			log.Error().
				Err(err).
				Str("room_id", roomID).
				Str("alert_status", alert.Status).
				Msg("Failed to execute alert template")
			continue
		}
		matrix.SendFormattedNotice(roomID, buf.String())
	}
}

// AlertmanagerBasicAuthMiddleware creates Gin middleware for HTTP Basic Auth
func AlertmanagerBasicAuthMiddleware(expectedUser, expectedPassword string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, password, ok := c.Request.BasicAuth()
		if !ok || user != expectedUser || password != expectedPassword {
			log.Warn().
				Bool("auth_provided", ok).
				Msg("Alertmanager webhook received with invalid credentials")
			c.Header("WWW-Authenticate", `Basic realm="alertmanager"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}

// AlertmanagerWebhookHandler handles Alertmanager webhook requests
func AlertmanagerWebhookHandler(c *gin.Context) {
	metrics.RecordWebhookHandled(constants.WebhookAlertmanager)

	roomID := c.Query("room_id")
	if roomID == "" {
		log.Warn().Msg("Alertmanager webhook received without room_id")
		c.Status(http.StatusBadRequest)
		return
	}

	var payload AlertmanagerPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		log.Error().
			Err(err).
			Str("room_id", roomID).
			Msg("Failed to parse Alertmanager webhook payload")
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	log.Debug().
		Str("room_id", roomID).
		Str("status", payload.Status).
		Int("alert_count", len(payload.Alerts)).
		Msg("Processing Alertmanager webhook request")

	sendAlertmanagerMsg(payload, roomID)
	c.Status(http.StatusOK)
}
