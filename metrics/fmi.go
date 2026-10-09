package metrics

import "github.com/prometheus/client_golang/prometheus"

var fmiRequests = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "fmi_requests_count",
	Help: "Total number of FMI data tool requests, by kind of data and outcome",
}, []string{"kind", "status"}))

var fmiRequestDuration = makeCollector(prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    metricPrefix + "fmi_request_duration_seconds",
	Help:    "How long an FMI data tool request took, by kind of data",
	Buckets: defaultBuckets,
}, []string{"kind"}))

// InitializeFMIKind creates the series for a kind of FMI data, so it shows up at zero before its first request
func InitializeFMIKind(kind string) {
	fmiRequests.WithLabelValues(kind, "success")
	fmiRequests.WithLabelValues(kind, "failure")
	fmiRequestDuration.WithLabelValues(kind)
}

// RecordFMIRequest records an FMI data tool request
func RecordFMIRequest(kind string, success bool, durationSec float64) {
	status := "success"
	if !success {
		status = "failure"
	}
	fmiRequests.WithLabelValues(kind, status).Inc()
	fmiRequestDuration.WithLabelValues(kind).Observe(durationSec)
}
