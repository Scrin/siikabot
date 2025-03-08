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

var chatAPICalls = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "chat_api_calls_count",
	Help: "Total number of chat API calls made",
}, []string{"model", "status"}))

// RecordChatAPICall records a chat API call being made
func RecordChatAPICall(model string, success bool) {
	status := "success"
	if !success {
		status = "failure"
	}
	chatAPICalls.WithLabelValues(model, status).Inc()
}

var chatCharacters = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "chat_characters_count",
	Help: "Total number of characters used in chat API calls",
}, []string{"model", "type"}))

// RecordChatCharacters records the number of characters used in a chat API call
func RecordChatCharacters(model string, inputChars, outputChars int) {
	chatCharacters.WithLabelValues(model, "input").Add(float64(inputChars))
	chatCharacters.WithLabelValues(model, "output").Add(float64(outputChars))
}

var toolCalls = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "tool_calls_count",
	Help: "Total number of tool calls made",
}, []string{"tool", "status"}))

// RecordToolCall records a tool call being made
func RecordToolCall(tool string, success bool) {
	status := "success"
	if !success {
		status = "failure"
	}
	toolCalls.WithLabelValues(tool, status).Inc()
}
