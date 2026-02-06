package metrics

import (
	"strconv"

	"github.com/Scrin/siikabot/constants"
	"github.com/prometheus/client_golang/prometheus"
)

const metricPrefix = "siikabot_"

var defaultBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 10}

func makeCollector[C prometheus.Collector](collector C) C {
	prometheus.MustRegister(collector)
	return collector
}

// misc collectors

var webhooksHandled = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "webhooks_handled_count",
	Help: "Total number of webhook requests handled",
}, []string{"hook"}))

var eventsHandled = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "events_handled_count",
	Help: "Total number of events handled",
}, []string{"event_type", "sub_type", "encrypted"}))

var commandsHandled = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "commands_handled_count",
	Help: "Total number of chat commands handled",
}, []string{"command"}))

var webhookEventsHandled = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "webhook_events_handled_count",
	Help: "Total number of webhook events handled by type",
}, []string{"hook", "event_type"}))

func init() {
	for _, hook := range constants.AllWebhookTypes {
		webhooksHandled.WithLabelValues(string(hook))
	}
	for hook, events := range constants.WebhookEventTypes {
		for _, eventType := range events {
			webhookEventsHandled.WithLabelValues(string(hook), string(eventType))
		}
	}
	for _, cmd := range constants.AllCommands {
		commandsHandled.WithLabelValues(string(cmd))
	}
}

// RecordWebhookHandled records a webhook being handled
func RecordWebhookHandled(hook constants.WebhookType) {
	webhooksHandled.WithLabelValues(string(hook)).Inc()
}

// RecordWebhookEventHandled records a webhook event of a specific type being handled
func RecordWebhookEventHandled(hook constants.WebhookType, eventType constants.WebhookEventType) {
	webhookEventsHandled.WithLabelValues(string(hook), string(eventType)).Inc()
}

// RecordEventHandled records a Matrix event being handled
func RecordEventHandled(eventType, subType string, encrypted bool) {
	eventsHandled.WithLabelValues(eventType, subType, strconv.FormatBool(encrypted)).Inc()
}

// RecordCommandHandled records a chat command being handled
func RecordCommandHandled(command constants.Command) {
	commandsHandled.WithLabelValues(string(command)).Inc()
}
