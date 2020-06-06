package bot

import (
	"net/http"
)

func initHTTP(hookSecret string) {
	http.HandleFunc("/hooks/gitlab", gitlabHandler(hookSecret))
	go http.ListenAndServe(":8080", nil)
}
