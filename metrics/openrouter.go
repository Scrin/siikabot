package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var openrouterCost = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "openrouter_cost",
	Help: "Cost of OpenRouter API calls in USD",
}, []string{"model", "provider"}))

var openrouterLatency = makeCollector(prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: metricPrefix + "openrouter_latest_latency_seconds",
	Help: "Latest latency of OpenRouter API calls in seconds",
}, []string{"model", "provider", "type"}))

var openrouterLatencyHistogram = makeCollector(prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    metricPrefix + "openrouter_latency_seconds",
	Help:    "Histogram of OpenRouter API call latencies in seconds",
	Buckets: defaultBuckets,
}, []string{"model", "provider", "type"}))

var openrouterTokens = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "openrouter_tokens_count",
	Help: "Token counts from OpenRouter generation API",
}, []string{"model", "provider", "type"}))

// RecordOpenRouterStats records statistics from the OpenRouter generation API
func RecordOpenRouterStats(model, provider string, latency, generationTime, moderationLatency float64, cost float64) {
	openrouterCost.WithLabelValues(model, provider).Add(cost)

	if latency > 0 {
		openrouterLatency.WithLabelValues(model, provider, "total").Set(latency)
		openrouterLatencyHistogram.WithLabelValues(model, provider, "total").Observe(latency)
	}

	if generationTime > 0 {
		openrouterLatency.WithLabelValues(model, provider, "generation").Set(generationTime)
		openrouterLatencyHistogram.WithLabelValues(model, provider, "generation").Observe(generationTime)
	}

	if moderationLatency > 0 {
		openrouterLatency.WithLabelValues(model, provider, "moderation").Set(moderationLatency)
		openrouterLatencyHistogram.WithLabelValues(model, provider, "moderation").Observe(moderationLatency)
	}
}

// RecordOpenRouterTokens records token counts from the OpenRouter generation API
func RecordOpenRouterTokens(model, provider string, promptTokens, completionTokens int) {
	if promptTokens > 0 {
		openrouterTokens.WithLabelValues(model, provider, "prompt").Add(float64(promptTokens))
	}

	if completionTokens > 0 {
		openrouterTokens.WithLabelValues(model, provider, "completion").Add(float64(completionTokens))
	}
}
