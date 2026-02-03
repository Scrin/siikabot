package bot

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"text/template"
	"time"

	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/metrics"
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

func alertmanagerHandler(user, password string) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		metrics.RecordWebhookHandled("alertmanager")

		reqUser, reqPassword, ok := req.BasicAuth()
		if !ok || reqUser != user || reqPassword != password {
			log.Warn().
				Bool("auth_provided", ok).
				Msg("Alertmanager webhook received with invalid credentials")
			w.Header().Set("WWW-Authenticate", `Basic realm="alertmanager"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			log.Error().Err(err).Msg("Failed to read Alertmanager webhook request body")
			return
		}
		req.Body.Close()

		roomID := req.URL.Query().Get("room_id")
		if roomID == "" {
			log.Warn().Msg("Alertmanager webhook received without room_id")
			return
		}

		payload := AlertmanagerPayload{}
		err = json.Unmarshal(body, &payload)
		if err != nil {
			log.Error().
				Err(err).
				Str("room_id", roomID).
				Str("body", string(body)).
				Msg("Failed to parse Alertmanager webhook payload")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.Debug().
			Str("room_id", roomID).
			Str("status", payload.Status).
			Int("alert_count", len(payload.Alerts)).
			Msg("Processing Alertmanager webhook request")

		sendAlertmanagerMsg(payload, roomID)
	}
}
