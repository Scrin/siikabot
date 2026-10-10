package metrics

import "github.com/prometheus/client_golang/prometheus"

var sourceCodeDownloads = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "source_code_downloads_count",
	Help: "Total number of downloads of the bot's own source code, by outcome",
}, []string{"status"}))

var sourceCodeDownloadDuration = makeCollector(prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    metricPrefix + "source_code_download_duration_seconds",
	Help:    "How long a download of the bot's own source code took",
	Buckets: defaultBuckets,
}))

func init() {
	// Both outcomes show up at zero before the first download
	sourceCodeDownloads.WithLabelValues("success")
	sourceCodeDownloads.WithLabelValues("failure")
}

// RecordSourceCodeDownload records a download of the bot's own source code
func RecordSourceCodeDownload(success bool, durationSec float64) {
	status := "success"
	if !success {
		status = "failure"
	}
	sourceCodeDownloads.WithLabelValues(status).Inc()
	sourceCodeDownloadDuration.Observe(durationSec)
}
