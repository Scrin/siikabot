package bot

import (
	"net/http"

	"github.com/Scrin/siikabot/config"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func initHTTP() {
	http.HandleFunc("/hooks/github", githubHandler(config.HookSecret))
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(":8080", nil)
}
