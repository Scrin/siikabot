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

// Failures are counted separately from chatAPICalls rather than by adding a label to it, so the
// existing success/failure counter keeps its meaning and cardinality. error_kind is a closed set
// defined in the aigateway package; nothing derived from a message body may be used as a label,
// since the metrics endpoint is public.
var chatAPIErrors = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "chat_api_errors_count",
	Help: "Total number of failed chat API calls by error kind",
}, []string{"model", "error_kind"}))

var chatAPICallDuration = makeCollector(prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    metricPrefix + "chat_api_call_duration_seconds",
	Help:    "Duration of individual chat API calls in seconds",
	Buckets: []float64{0.25, 0.5, 1, 2, 4, 8, 15, 30, 60},
}, []string{"model"}))

// Cached prompt tokens are the only direct evidence that the stable-prefix work is paying off
var chatCachedTokens = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "chat_cached_tokens_count",
	Help: "Total number of prompt tokens served from the provider's cache",
}, []string{"model"}))

var chatUncachedPromptTokens = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "chat_uncached_prompt_tokens_count",
	Help: "Total number of prompt tokens not served from the provider's cache",
}, []string{"model"}))

var toolCalls = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "tool_calls_count",
	Help: "Total number of tool calls made",
}, []string{"tool", "status"}))

var toolErrors = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "tool_errors_count",
	Help: "Total number of failed tool calls by error kind",
}, []string{"tool", "error_kind"}))

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

// Splits a turn into waiting on the model versus waiting on tools, so "slow when many tool calls
// are involved" can be attributed rather than guessed at
var chatTurnPhaseDuration = makeCollector(prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    metricPrefix + "chat_turn_phase_duration_seconds",
	Help:    "Time spent in each phase of a chat turn in seconds",
	Buckets: []float64{0.25, 0.5, 1, 2, 4, 8, 15, 30, 60, 120},
}, []string{"model", "phase"}))

// Shows which part of the prompt the tokens are actually going to, so trimming can be aimed
var chatPromptComponentTokens = makeCollector(prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    metricPrefix + "chat_prompt_component_tokens",
	Help:    "Estimated token cost of each component of a prompt",
	Buckets: []float64{64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384},
}, []string{"component"}))

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

// RecordChatAPIError records a failed chat API call under a specific error kind
func RecordChatAPIError(model, errorKind string) {
	chatAPIErrors.WithLabelValues(model, errorKind).Inc()
}

// RecordChatAPICallDuration records how long a single chat API call took
func RecordChatAPICallDuration(model string, durationSec float64) {
	chatAPICallDuration.WithLabelValues(model).Observe(durationSec)
}

// RecordChatCachedTokens records how much of a prompt was served from the provider's cache.
// Splitting the prompt into cached and uncached makes the hit rate a ratio of two counters.
func RecordChatCachedTokens(model string, cachedTokens, promptTokens int) {
	if cachedTokens > 0 {
		chatCachedTokens.WithLabelValues(model).Add(float64(cachedTokens))
	}
	if uncached := promptTokens - cachedTokens; uncached > 0 {
		chatUncachedPromptTokens.WithLabelValues(model).Add(float64(uncached))
	}
}

// RecordToolError records a failed tool call under a specific error kind
func RecordToolError(tool, errorKind string) {
	toolErrors.WithLabelValues(tool, errorKind).Inc()
}

// RecordChatTurnPhase records how long a turn spent waiting on the model versus on tools
func RecordChatTurnPhase(model, phase string, durationSec float64) {
	chatTurnPhaseDuration.WithLabelValues(model, phase).Observe(durationSec)
}

// RecordChatPromptComponent records the estimated token cost of one part of a prompt
func RecordChatPromptComponent(component string, tokens int) {
	chatPromptComponentTokens.WithLabelValues(component).Observe(float64(tokens))
}
