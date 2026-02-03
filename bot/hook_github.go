package bot

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/metrics"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
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
	log.Debug().
		Str("room_id", roomID).
		Str("repository", payload.Repository.FullName).
		Str("hook_type", payload.Hook.Type).
		Msg("Processing GitHub webhook")

	if payload.Hook.Type == "Repository" {
		sendGithubHookConfig(payload, roomID)
	} else if payload.Pusher.Name != "" {
		sendGithubPush(payload, roomID)
	} else if payload.PullRequest.HtmlUrl != "" {
		sendGithubPullrequest(payload, roomID)
	} else {
		log.Warn().
			Str("room_id", roomID).
			Str("repository", payload.Repository.FullName).
			Msg("Unknown GitHub webhook type received")
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

// GithubSignatureMiddleware creates Gin middleware that verifies GitHub webhook signatures
func GithubSignatureMiddleware(hookSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		signature := c.GetHeader("x-hub-signature")
		if signature == "" {
			log.Warn().Msg("GitHub webhook received without signature")
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// Read the body for signature verification
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			log.Error().Err(err).Msg("Failed to read GitHub webhook request body")
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}

		// Verify signature
		if !verifySignature([]byte(hookSecret), signature, body) {
			log.Warn().Str("signature", signature).Msg("Invalid GitHub webhook signature")
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// Restore body for handler to use
		c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
		c.Next()
	}
}

// GithubWebhookHandler handles GitHub webhook requests
func GithubWebhookHandler(c *gin.Context) {
	metrics.RecordWebhookHandled("github")

	roomID := c.Query("room_id")
	if roomID == "" {
		log.Warn().Msg("GitHub webhook received without room_id")
		c.Status(http.StatusBadRequest)
		return
	}

	var payload GithubPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		log.Error().Err(err).Str("room_id", roomID).Msg("Failed to parse GitHub webhook payload")
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	log.Debug().
		Str("room_id", roomID).
		Str("repository", payload.Repository.FullName).
		Str("sender", payload.Sender.Login).
		Msg("Processing GitHub webhook request")

	sendGithubMsg(payload, roomID)
	c.Status(http.StatusOK)
}
