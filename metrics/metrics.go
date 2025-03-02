package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

const metricPrefix = "siikabot_"

func makeCollector[C prometheus.Collector](collector C) C {
	prometheus.MustRegister(collector)
	return collector
}

var webhooksHandled = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "webhooks_handled_count",
	Help: "Total number of webhook requests handled",
}, []string{"hook"}))

// RecordWebhookHandled records a webhook being handled
func RecordWebhookHandled(hook string) {
	webhooksHandled.WithLabelValues(hook).Inc()
}

var eventsHandled = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "events_handled_count",
	Help: "Total number of events handled",
}, []string{"event_type", "sub_type", "encrypted"}))

// RecordEventHandled records a Matrix event being handled
func RecordEventHandled(eventType, subType string, encrypted bool) {
	eventsHandled.WithLabelValues(eventType, subType, strconv.FormatBool(encrypted)).Inc()
}

var commandsHandled = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "commands_handled_count",
	Help: "Total number of chat commands handled",
}, []string{"command"}))

// RecordCommandHandled records a chat command being handled
func RecordCommandHandled(command string) {
	commandsHandled.WithLabelValues(command).Inc()
}
