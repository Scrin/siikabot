package bot

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/Scrin/siikabot/matrix"
	"github.com/prometheus/client_golang/prometheus"
)

type GithubPayload struct {
	Action       string `json:"action"`
	Ref          string `json:"ref"`
	BeforeCommit string `json:"before"`
	AfterCommit  string `json:"after"`
	Compare      string `json:"compare"`
	Pusher       struct {
		Name string `json:"name"`
	} `json:"pusher"`
	Repository struct {
		FullName string `json:"full_name"`
		HtmlUrl  string `json:"html_url"`
	} `json:"repository"`
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
	PullRequest struct {
		HtmlUrl string `json:"html_url"`
		Title   string `json:"title"`
	} `json:"pull_request"`
	Hook struct {
		Type string `json:"type"`
	} `json:"hook"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

func sendGithubMsg(payload GithubPayload, roomID string) {
	if payload.Hook.Type == "Repository" {
		sendGithubHookConfig(payload, roomID)
	} else if payload.Pusher.Name != "" {
		sendGithubPush(payload, roomID)
	} else if payload.PullRequest.HtmlUrl != "" {
		sendGithubPullrequest(payload, roomID)
	} else {
		matrix.SendNotice(roomID, "Unknown github hook called")
	}
}

func sendGithubHookConfig(payload GithubPayload, roomID string) {
	matrix.SendFormattedNotice(roomID, "[<font color=\"#0000FC\">"+payload.Repository.FullName+"</font>] "+
		"<font color=\"#9C009C\">"+payload.Sender.Login+"</font> configured a webhook: "+payload.Repository.HtmlUrl)
}

func sendGithubPullrequest(payload GithubPayload, roomID string) {
	matrix.SendFormattedNotice(roomID, "[<font color=\"#0000FC\">"+payload.Repository.FullName+"</font>] "+
		"<font color=\"#9C009C\">"+payload.Sender.Login+"</font> <a href=\""+payload.PullRequest.HtmlUrl+"\">"+payload.Action+" a pull request:</a> "+
		"<font color=\"#7F0000\">"+payload.PullRequest.Title+"</font>")
}

func sendGithubPush(payload GithubPayload, roomID string) {
	nullCommit := "0000000000000000000000000000000000000000"
	if payload.AfterCommit == nullCommit {
		// TODO branch was deleted
		return
	} else if payload.BeforeCommit == nullCommit {
		// TODO branch was created
		return
	}

	ref := strings.Split(payload.Ref, "/")
	branch := payload.Ref
	if len(ref) == 3 {
		branch = ref[2]
	}

	output := []string{"[<font color=\"#0000FC\">" + payload.Repository.FullName + "</font>] " +
		"<font color=\"#9C009C\">" + payload.Pusher.Name + "</font> pushed <a href=\"" + payload.Compare + "\">" + strconv.Itoa(len(payload.Commits)) + " commits</a> " +
		"to <font color=\"#7F0000\">" + branch + "</font> "}

	for _, commit := range payload.Commits {
		added := strconv.Itoa(len(commit.Added))
		modified := strconv.Itoa(len(commit.Modified))
		removed := strconv.Itoa(len(commit.Removed))
		output = append(output, "<font color=\"#D2D2D2\">"+commit.ID[0:7]+"</font> "+
			"(<font color=\"#009300\">+"+added+"</font>|<font color=\"#555555\">±"+modified+"</font>|<font color=\"#FF0000\">-"+removed+"</font>) "+
			"<font color=\"#9C009C\">"+commit.Author.Name+"</font>: "+commit.Message)
	}

	matrix.SendFormattedNotice(roomID, strings.Join(output, "<br />"))
}

func verifySignature(secret []byte, signature string, body []byte) bool {
	const signaturePrefix = "sha1="
	const signatureLength = 45 // len(SignaturePrefix) + len(hex(sha1))

	if len(signature) != signatureLength || !strings.HasPrefix(signature, signaturePrefix) {
		return false
	}

	actual := make([]byte, 20)
	hex.Decode(actual, []byte(signature[5:]))

	computed := hmac.New(sha1.New, secret)
	computed.Write(body)
	return hmac.Equal([]byte(computed.Sum(nil)), actual)
}

func githubHandler(hookSecret string) func(w http.ResponseWriter, req *http.Request) {
	labels := prometheus.Labels{"hook": "github"}
	return func(w http.ResponseWriter, req *http.Request) {
		metrics.webhooksHandled.With(labels).Inc()
		signature := req.Header.Get("x-hub-signature")
		if signature == "" {
			return
		}

		body, err := ioutil.ReadAll(req.Body)
		if err != nil {
			log.Print(err)
			return
		}
		req.Body.Close()

		if !verifySignature([]byte(hookSecret), signature, body) {
			log.Print("Invalid signature")
			return
		}

		roomID := req.URL.Query().Get("room_id")
		if roomID == "" {
			return
		}
		msg := GithubPayload{}
		err = json.Unmarshal(body, &msg)
		if err != nil {
			fmt.Fprintf(w, "%v", err)
			return
		}
		sendGithubMsg(msg, roomID)
	}
}
