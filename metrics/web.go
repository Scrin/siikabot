package metrics

import "github.com/prometheus/client_golang/prometheus"

var webFetches = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "web_fetch_count",
	Help: "Total number of web page fetches, by how the page was fetched and how the fetch went",
}, []string{"tier", "outcome"}))

var webFetchDuration = makeCollector(prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    metricPrefix + "web_fetch_duration_seconds",
	Help:    "How long a web page fetch took, by how the page was fetched",
	Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10},
}, []string{"tier"}))

// InitializeWebFetchTier creates the series for a way of fetching pages, so they show up at zero before the
// first fetch
func InitializeWebFetchTier(tier string, outcomes []string) {
	for _, outcome := range outcomes {
		webFetches.WithLabelValues(tier, outcome)
	}
	webFetchDuration.WithLabelValues(tier)
}

// RecordWebFetch records a web page fetch
func RecordWebFetch(tier, outcome string, durationSec float64) {
	webFetches.WithLabelValues(tier, outcome).Inc()
	webFetchDuration.WithLabelValues(tier).Observe(durationSec)
}
