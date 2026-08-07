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

// The context window is sized with a cheap heuristic rather than a real tokeniser. This tracks the
// ratio of the token count the API reported to the count that was estimated, so the error stays
// visible: 1.0 is exact, above 1.0 means the estimate runs low and the window is bigger than intended.
var chatTokenEstimateDrift = makeCollector(prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    metricPrefix + "chat_token_estimate_drift_ratio",
	Help:    "Ratio of reported prompt tokens to estimated prompt tokens",
	Buckets: []float64{0.5, 0.7, 0.85, 0.95, 1.05, 1.15, 1.3, 1.5, 2},
}, []string{"model"}))

var chatContextWindowTokens = makeCollector(prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    metricPrefix + "chat_context_window_tokens",
	Help:    "Estimated token size of the replayed context window",
	Buckets: []float64{256, 512, 1024, 2048, 4096, 8192, 16384, 32768},
}, []string{"model"}))

var chatContextAnchorAdvances = makeCollector(prometheus.NewCounter(prometheus.CounterOpts{
	Name: metricPrefix + "chat_context_anchor_advances_count",
	Help: "Number of times a room's context window was trimmed back to the low mark",
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

// RecordTokenEstimateDrift records how far the prompt token estimate was from the reported count
func RecordTokenEstimateDrift(model string, ratio float64) {
	chatTokenEstimateDrift.WithLabelValues(model).Observe(ratio)
}

// RecordChatContextWindowTokens records the estimated size of the replayed context window
func RecordChatContextWindowTokens(model string, tokens int) {
	chatContextWindowTokens.WithLabelValues(model).Observe(float64(tokens))
}

// RecordChatContextAnchorAdvance records a context window being trimmed back to the low mark
func RecordChatContextAnchorAdvance() {
	chatContextAnchorAdvances.Inc()
}
