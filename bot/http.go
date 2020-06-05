package bot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type GitlabPush struct {
	Ref          string `json:"ref"`
	UserName     string `json:"user_name"`
	BeforeCommit string `json:"before"`
	AfterCommit  string `json:"after"`
	Project      struct {
		Name   string `json:"name"`
		WebUrl string `json:"web_url"`
	} `json:"project"`
	Commits []struct {
		ID       string   `json:"id"`
		Message  string   `json:"message"`
		Added    []string `json:"added"`
		Modified []string `json:"modified"`
		Removed  []string `json:"removed"`
		Author   struct {
			Name string `json:"name"`
		} `json:"author"`
	} `json:"commits"`
	TotalCommitsCount int `json:"total_commits_count"`
}

const nullCommit = "0000000000000000000000000000000000000000"

func sendGitlabMsg(push GitlabPush, roomID string) {

	if push.AfterCommit == nullCommit {
		// TODO branch was deleted
		return
	} else if push.BeforeCommit == nullCommit {
		// TODO branch was created
		return
	}

	ref := strings.Split(push.Ref, "/")
	branch := push.Ref
	if len(ref) == 3 {
		branch = ref[2]
	}

	before := push.BeforeCommit[0:7]
	after := push.AfterCommit[0:7]

	output := []string{"[<font color=\"#0000FC\">" + push.Project.Name + "</font>] " + push.UserName + " pushed " + strconv.Itoa(push.TotalCommitsCount) +
		" commits to <font color=\"#7F0000\">" + branch + "</font> " + push.Project.WebUrl + "/compare/" + before + "..." + after}

	for _, commit := range push.Commits {
		added := strconv.Itoa(+len(commit.Added))
		modified := strconv.Itoa(+len(commit.Modified))
		removed := strconv.Itoa(+len(commit.Removed))
		output = append(output, "<font color=\"#D2D2D2\">"+commit.ID[0:7]+"</font> "+
			"(<font color=\"#009300\">+"+added+"</font>|<font color=\"#000000\">±"+modified+"</font>|<font color=\"#FF0000\">-"+removed+"</font>) "+
			"<font color=\"#9C009C\">"+commit.Author.Name+"</font>: "+commit.Message)
	}

	client.SendFormattedMessage(roomID, strings.Join(output, "<br />"))
}

func initHTTP(hookSecret string) {
	http.HandleFunc("/hooks/gitlab", func(w http.ResponseWriter, req *http.Request) {
		token := strings.Split(req.Header.Get("X-Gitlab-Token"), " ")
		if len(token) != 2 || token[0] != hookSecret {
			return
		}
		msg := GitlabPush{}
		err := json.NewDecoder(req.Body).Decode(&msg)
		if err != nil {
			fmt.Fprintf(w, "%v", err)
			return
		}
		req.Body.Close()
		sendGitlabMsg(msg, token[1])
	})
	go http.ListenAndServe(":8080", nil)
}
