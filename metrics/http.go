package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var httpRequestsTotal = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "http_requests_total",
	Help: "Total number of HTTP requests handled",
}, []string{"method", "path", "status_code"}))

var httpRequestLatency = makeCollector(prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    metricPrefix + "http_request_latency_seconds",
	Help:    "Histogram of HTTP request latencies in seconds",
	Buckets: defaultBuckets,
}, []string{"method", "path"}))

// RecordHTTPRequest records an HTTP request being handled
func RecordHTTPRequest(method, path string, statusCode int, latencySec float64) {
	httpRequestsTotal.WithLabelValues(method, path, strconv.Itoa(statusCode)).Inc()
	if latencySec > 0 {
		httpRequestLatency.WithLabelValues(method, path).Observe(latencySec)
	}
}
