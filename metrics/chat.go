package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var chatAPICalls = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "chat_api_calls_count",
	Help: "Total number of chat API calls made",
}, []string{"model", "status"}))

var chatTokens = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "chat_tokens_count",
	Help: "Total number of tokens used in chat API calls",
}, []string{"model", "type"}))

var toolCalls = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "tool_calls_count",
	Help: "Total number of tool calls made",
}, []string{"tool", "status"}))

var toolLatency = makeCollector(prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: metricPrefix + "tool_latest_latency_seconds",
	Help: "Latest latency of tool calls in seconds",
}, []string{"tool"}))

var toolLatencyHistogram = makeCollector(prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    metricPrefix + "tool_latency_seconds",
	Help:    "Histogram of tool call latencies in seconds",
	Buckets: defaultBuckets,
}, []string{"tool"}))

// RecordChatAPICall records a chat API call being made
func RecordChatAPICall(model string, success bool) {
	status := "success"
	if !success {
		status = "failure"
	}
	chatAPICalls.WithLabelValues(model, status).Inc()
}

// RecordChatTokens records the number of tokens used in a chat API call
func RecordChatTokens(model string, promptTokens, completionTokens int) {
	chatTokens.WithLabelValues(model, "prompt").Add(float64(promptTokens))
	chatTokens.WithLabelValues(model, "completion").Add(float64(completionTokens))
}

// RecordToolCall records a tool call being made
func RecordToolCall(tool string, success bool) {
	status := "success"
	if !success {
		status = "failure"
	}
	toolCalls.WithLabelValues(tool, status).Inc()
}

// RecordToolLatency records the latency of a tool call
func RecordToolLatency(tool string, latencySec float64) {
	if latencySec > 0 {
		toolLatency.WithLabelValues(tool).Set(latencySec)
		toolLatencyHistogram.WithLabelValues(tool).Observe(latencySec)
	}
}

// InitializeTool initializes the tool metrics to zero
func InitializeTool(tool string) {
	toolCalls.WithLabelValues(tool, "success")
	toolCalls.WithLabelValues(tool, "failure")
	toolLatency.WithLabelValues(tool)
	toolLatencyHistogram.WithLabelValues(tool)
}

var chatRequestDuration = makeCollector(prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    metricPrefix + "chat_request_duration_seconds",
	Help:    "End-to-end duration of chat requests in seconds",
	Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60, 120},
}, []string{"model", "has_image"}))

var chatToolIterations = makeCollector(prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    metricPrefix + "chat_tool_iterations",
	Help:    "Number of tool iterations per chat request",
	Buckets: []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
}))

var chatImagesProcessed = makeCollector(prometheus.NewCounter(prometheus.CounterOpts{
	Name: metricPrefix + "chat_images_processed_count",
	Help: "Total number of images processed in chat requests",
}))

// RecordChatRequestDuration records the end-to-end duration of a chat request
func RecordChatRequestDuration(model string, hasImage bool, durationSec float64) {
	chatRequestDuration.WithLabelValues(model, strconv.FormatBool(hasImage)).Observe(durationSec)
}

// RecordChatToolIterations records the number of tool iterations in a chat request
func RecordChatToolIterations(count int) {
	chatToolIterations.Observe(float64(count))
}

// RecordChatImageProcessed records an image being processed in a chat request
func RecordChatImageProcessed() {
	chatImagesProcessed.Inc()
}
