package bot

import (
	"bytes"
	"context"
	"crypto/subtle"
	"html/template"
	"net/http"
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

// alertView is what an alert's message shows: its heading, the detail under it, and the alert itself
type alertView struct {
	Alert
	Heading string
	Detail  string
}

// The message follows Alertmanager's usual annotations: the summary is the heading and the description
// the detail under it. An alert with no summary is headed by its description, as rules that only set a
// description expect. The link goes to where the alert came from, such as the rule's graph or the page
// of the finding behind it
const alertMessageTemplate = `<h4>{{ if eq .Status "resolved" }}✅{{ else }}⚠️{{ end }} {{ .Heading }}</h4>
{{- with .Detail }}
<p>{{ . }}</p>
{{- end }}
Started at <b>{{ .StartsAt.Format "Jan 02, 2006 15:04:05 UTC" }}</b>{{ if eq .Status "resolved" }}, ended at <b>{{ .EndsAt.Format "Jan 02, 2006 15:04:05 UTC" }}</b>{{ end }}<br>
{{- range $key, $value := .Labels }}
{{ $key }}: <b>{{ $value }}</b><br>
{{- end }}
{{- with .GeneratorURL }}
<a href="{{ . }}">Source</a>
{{- end }}`

// html/template escapes every value: annotations can hold any text, such as the log lines an alert quotes,
// which must show as text, never as markup or links of their own. A source link that isn't http or https
// is replaced with a harmless one
var alertTemplate = template.Must(template.New("alert").Parse(alertMessageTemplate))

// alertMessage renders an alert as the HTML of its message
func alertMessage(alert Alert) (string, error) {
	view := alertView{Alert: alert, Heading: alert.Annotations["summary"]}
	description := alert.Annotations["description"]
	switch {
	case view.Heading == "":
		view.Heading = description
	case description != view.Heading:
		view.Detail = description
	}
	if view.Heading == "" {
		view.Heading = alert.Labels["alertname"]
	}
	var buf bytes.Buffer
	if err := alertTemplate.Execute(&buf, view); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func sendAlertmanagerMsg(ctx context.Context, payload AlertmanagerPayload, roomID string) {
	log.Debug().
		Str("room_id", roomID).
		Str("status", payload.Status).
		Int("alert_count", len(payload.Alerts)).
		Msg("Processing Alertmanager webhook")

	for _, alert := range payload.Alerts {
		metrics.RecordWebhookEventHandled(constants.WebhookAlertmanager, constants.WebhookEventType(alert.Status))
		message, err := alertMessage(alert)
		if err != nil {
			log.Error().
				Ctx(ctx).
				Err(err).
				Str("room_id", roomID).
				Str("alert_status", alert.Status).
				Msg("Failed to execute alert template")
			continue
		}
		matrix.SendFormattedMessage(ctx, roomID, message)
	}
}

// AlertmanagerBasicAuthMiddleware creates Gin middleware for HTTP Basic Auth
func AlertmanagerBasicAuthMiddleware(expectedUser, expectedPassword string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, password, ok := c.Request.BasicAuth()
		// Compared in constant time, both of them, so that the time taken tells nothing about either
		userMatches := subtle.ConstantTimeCompare([]byte(user), []byte(expectedUser))
		passwordMatches := subtle.ConstantTimeCompare([]byte(password), []byte(expectedPassword))
		if !ok || userMatches&passwordMatches != 1 {
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

	sendAlertmanagerMsg(c.Request.Context(), payload, roomID)
	c.Status(http.StatusOK)
}
