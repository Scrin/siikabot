package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var aiGatewayCost = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "ai_gateway_cost",
	Help: "Cost of AI Gateway inference requests in USD",
}, []string{"model", "provider"}))

var aiGatewayLatency = makeCollector(prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: metricPrefix + "ai_gateway_latest_latency_seconds",
	Help: "Latest latency of AI Gateway inference requests in seconds",
}, []string{"model", "provider"}))

var aiGatewayLatencyHistogram = makeCollector(prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    metricPrefix + "ai_gateway_latency_seconds",
	Help:    "Histogram of AI Gateway inference request latencies in seconds",
	Buckets: defaultBuckets,
}, []string{"model", "provider"}))

var aiGatewayTokens = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "ai_gateway_tokens_count",
	Help: "Token counts from the AI Gateway logs",
}, []string{"model", "provider", "type"}))

var aiGatewayCacheStatus = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "ai_gateway_cache_status_count",
	Help: "Number of AI Gateway requests by cache status",
}, []string{"model", "provider", "status"}))

var aiGatewayFailures = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "ai_gateway_failures_count",
	Help: "Number of failed AI Gateway requests by HTTP status code",
}, []string{"model", "provider", "status_code"}))

// RecordAIGatewayCost records the cost of an AI Gateway request
func RecordAIGatewayCost(model, provider string, cost float64) {
	aiGatewayCost.WithLabelValues(model, provider).Add(cost)
}

// RecordAIGatewayLatency records the latency of an AI Gateway request
func RecordAIGatewayLatency(model, provider string, latencySec float64) {
	aiGatewayLatency.WithLabelValues(model, provider).Set(latencySec)
	aiGatewayLatencyHistogram.WithLabelValues(model, provider).Observe(latencySec)
}

// RecordAIGatewayTokens records token counts from the AI Gateway logs
func RecordAIGatewayTokens(model, provider string, tokensIn, tokensOut int) {
	if tokensIn > 0 {
		aiGatewayTokens.WithLabelValues(model, provider, "prompt").Add(float64(tokensIn))
	}

	if tokensOut > 0 {
		aiGatewayTokens.WithLabelValues(model, provider, "completion").Add(float64(tokensOut))
	}
}

// RecordAIGatewayCacheStatus records whether an AI Gateway request was served from cache
func RecordAIGatewayCacheStatus(model, provider string, cached bool) {
	status := "miss"
	if cached {
		status = "hit"
	}
	aiGatewayCacheStatus.WithLabelValues(model, provider, status).Inc()
}

// RecordAIGatewayFailure records a failed AI Gateway request
func RecordAIGatewayFailure(model, provider string, statusCode int) {
	aiGatewayFailures.WithLabelValues(model, provider, strconv.Itoa(statusCode)).Inc()
}
