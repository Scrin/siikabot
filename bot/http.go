package bot

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func initHTTP(hookSecret string) {
	http.HandleFunc("/hooks/github", githubHandler(hookSecret))
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(":8080", nil)
}
