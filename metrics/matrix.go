package metrics

import (
	"github.com/Scrin/siikabot/constants"
	"github.com/prometheus/client_golang/prometheus"
)

var matrixMessagesSent = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "matrix_messages_sent_count",
	Help: "Total number of Matrix messages sent",
}, []string{"status"}))

var matrixMessageLatency = makeCollector(prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    metricPrefix + "matrix_message_latency_seconds",
	Help:    "Histogram of Matrix message send latencies in seconds",
	Buckets: defaultBuckets,
}))

var matrixRateLimitRetries = makeCollector(prometheus.NewCounter(prometheus.CounterOpts{
	Name: metricPrefix + "matrix_rate_limit_retries_count",
	Help: "Total number of rate limit retries when sending Matrix messages",
}))

var matrixOutboundQueueDepth = makeCollector(prometheus.NewGauge(prometheus.GaugeOpts{
	Name: metricPrefix + "matrix_outbound_queue_depth",
	Help: "Current number of events in the outbound event queue",
}))

func init() {
	for _, status := range constants.AllMatrixSendStatuses {
		matrixMessagesSent.WithLabelValues(string(status))
	}
}

// RecordMatrixMessageSent records a Matrix message being sent with the given status
func RecordMatrixMessageSent(status constants.MatrixSendStatus) {
	matrixMessagesSent.WithLabelValues(string(status)).Inc()
}

// RecordMatrixMessageLatency records the latency of sending a Matrix message
func RecordMatrixMessageLatency(latencySec float64) {
	if latencySec > 0 {
		matrixMessageLatency.Observe(latencySec)
	}
}

// RecordMatrixRateLimitRetry records a rate limit retry when sending a Matrix message
func RecordMatrixRateLimitRetry() {
	matrixRateLimitRetries.Inc()
}

// SetMatrixOutboundQueueDepth sets the current depth of the outbound event queue
func SetMatrixOutboundQueueDepth(depth int) {
	matrixOutboundQueueDepth.Set(float64(depth))
}
