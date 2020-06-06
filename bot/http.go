package bot

import (
	"net/http"
)

func initHTTP(hookSecret string) {
	http.HandleFunc("/hooks/gitlab", gitlabHandler(hookSecret))
	http.HandleFunc("/hooks/github", githubHandler(hookSecret))
	go http.ListenAndServe(":8080", nil)
}
