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
)

type GithubPush struct {
	Ref          string `json:"ref"`
	BeforeCommit string `json:"before"`
	AfterCommit  string `json:"after"`
	Compare      string `json:"compare"`
	Pusher       struct {
		Name string `json:"name"`
	} `json:"pusher"`
	Repository struct {
		Name string `json:"name"`
	} `json:"repository"`
	Commits []struct {
		Sha     string `json:"sha"`
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
		} `json:"author"`
	} `json:"commits"`
}

func sendGithubMsg(push GithubPush, roomID string) {
	nullCommit := "0000000000000000000000000000000000000000"
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

	output := []string{"[<font color=\"#0000FC\">" + push.Repository.Name + "</font>] " +
		"<font color=\"#9C009C\">" + push.Pusher.Name + "</font> pushed " + strconv.Itoa(len(push.Commits)) + " commits " +
		"to <font color=\"#7F0000\">" + branch + "</font> " + push.Compare}

	for _, commit := range push.Commits {
		output = append(output, "<font color=\"#D2D2D2\">"+commit.Sha[0:7]+"</font> "+
			"<font color=\"#9C009C\">"+commit.Author.Name+"</font>: "+commit.Message)
	}

	client.SendFormattedMessage(roomID, strings.Join(output, "<br />"))
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
	return func(w http.ResponseWriter, req *http.Request) {

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
		msg := GithubPush{}
		err = json.Unmarshal(body, &msg)
		if err != nil {
			fmt.Fprintf(w, "%v", err)
			return
		}
		sendGithubMsg(msg, roomID)
	}
}
