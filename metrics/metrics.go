package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	webhooksHandled *prometheus.CounterVec
	eventsHandled   *prometheus.CounterVec
	commandsHandled *prometheus.CounterVec
)

func init() {
	metricPrefix := "siikabot_"
	webhooksHandled = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricPrefix + "webhooks_handled_count",
		Help: "Total number of webhook requests handled",
	}, []string{"hook"})
	eventsHandled = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricPrefix + "events_handled_count",
		Help: "Total number of events handled",
	}, []string{"event_type", "msg_type"})
	commandsHandled = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricPrefix + "commands_handled_count",
		Help: "Total number of chat commands handled",
	}, []string{"command"})

	prometheus.MustRegister(webhooksHandled)
	prometheus.MustRegister(eventsHandled)
	prometheus.MustRegister(commandsHandled)
}

// RecordWebhookHandled records a webhook being handled
func RecordWebhookHandled(hook string) {
	webhooksHandled.WithLabelValues(hook).Inc()
}

// RecordEventHandled records a Matrix event being handled
func RecordEventHandled(eventType, msgType string) {
	eventsHandled.WithLabelValues(eventType, msgType).Inc()
}

// RecordCommandHandled records a chat command being handled
func RecordCommandHandled(command string) {
	commandsHandled.WithLabelValues(command).Inc()
}
