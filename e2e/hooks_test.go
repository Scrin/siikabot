//go:build e2e

package e2e

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The webhooks, checked as the room receives their messages

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// formattedBody is a message's HTML
func formattedBody(content map[string]any) string {
	body, _ := content["formatted_body"].(string)
	return body
}

func TestMessageWebhook(t *testing.T) {
	e := setUp(t)
	room := e.Room(t, e.admin, nil)
	dashboard := e.Login(t, e.admin, room)

	token := randomString()
	var created struct {
		Webhook struct {
			ID string `json:"id"`
		} `json:"webhook"`
	}
	if code := dashboard.Call(t, http.MethodPost, "/api/admin/webhooks",
		map[string]string{"name": "Deploys " + randomString()[:6], "room_id": room.ID.String(), "token": token}, &created); code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("creating a webhook gave %d", code)
	}
	path := "/hooks/message/" + created.Webhook.ID
	text := marker(t, "deployed")

	if code := e.PostHook(t, path, []byte(text), map[string]string{"Authorization": "Bearer wrong-token-0123456789"}); code != http.StatusUnauthorized {
		t.Errorf("a wrong token gets %d, want 401", code)
	}
	last := room.Latest(t)
	if code := e.PostHook(t, path, []byte("**"+text+"**"), map[string]string{"Authorization": "Bearer " + token}); code != http.StatusOK {
		t.Fatalf("posting with the token gave %d", code)
	}
	content := room.WaitForBotMessage(t, last, func(content map[string]any) bool {
		return strings.Contains(formattedBody(content), text)
	})
	if !strings.Contains(formattedBody(content), "<strong>"+text+"</strong>") {
		t.Errorf("the message's Markdown wasn't rendered: %s", formattedBody(content))
	}
}

// An alert shows as text, never markup, whatever its annotations hold
func TestAlertmanagerWebhook(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	summary := marker(t, "summary")
	payload, _ := json.Marshal(map[string]any{
		"version": "4", "status": "firing",
		"alerts": []map[string]any{{
			"status": "firing", "labels": map[string]string{"alertname": "DiskFull"},
			"annotations": map[string]string{"summary": "<b>" + summary + "</b>", "description": "<img src=x>"},
			"startsAt":    time.Now().UTC().Format(time.RFC3339), "generatorURL": "https://prometheus.example/graph",
		}},
	})
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(alertUser+":"+alertPassword))

	if code := e.PostHook(t, "/hooks/alertmanager?room_id="+room.ID.String(), payload, map[string]string{"Content-Type": "application/json"}); code != http.StatusUnauthorized {
		t.Errorf("an alert without credentials gets %d, want 401", code)
	}
	last := room.Latest(t)
	if code := e.PostHook(t, "/hooks/alertmanager?room_id="+room.ID.String(), payload,
		map[string]string{"Content-Type": "application/json", "Authorization": auth}); code != http.StatusOK {
		t.Fatalf("posting the alert gave %d", code)
	}
	content := room.WaitForBotMessage(t, last, func(content map[string]any) bool {
		return strings.Contains(formattedBody(content), summary)
	})
	html := formattedBody(content)
	if strings.Contains(html, "<b>"+summary) || strings.Contains(html, "<img") || !strings.Contains(html, "&lt;b&gt;"+summary) {
		t.Errorf("the alert's annotations became markup: %s", html)
	}
}

// A pull request's title is anyone's to write, and shows as text (review finding L1)
func TestGithubWebhook(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	title := marker(t, "title")
	payload := mustJSON(map[string]any{
		"action":       "opened",
		"pull_request": map[string]any{"html_url": "https://github.com/x/y/pull/1", "title": `<img src="mxc://evil/x">` + title},
		"repository":   map[string]any{"full_name": "x/y", "html_url": "https://github.com/x/y"},
		"sender":       map[string]any{"login": "mallory"},
	})
	last := room.Latest(t)
	postGithub(t, e, room, payload)
	content := room.WaitForBotMessage(t, last, func(content map[string]any) bool {
		return strings.Contains(formattedBody(content), title)
	})
	if html := formattedBody(content); strings.Contains(html, "<img") || !strings.Contains(html, "&lt;img") {
		t.Errorf("the pull request's title became markup: %s", html)
	}
}

func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

// postGithub posts a payload to the GitHub webhook for a room, signed as GitHub signs it
func postGithub(t *testing.T, e *environment, room *Room, payload []byte) {
	t.Helper()
	mac := hmac.New(sha1.New, []byte(hookSecret))
	mac.Write(payload)
	if code := e.PostHook(t, "/hooks/github?room_id="+room.ID.String(), payload, map[string]string{
		"Content-Type": "application/json", "X-Hub-Signature": "sha1=" + hex.EncodeToString(mac.Sum(nil))}); code != http.StatusOK {
		t.Fatalf("posting the signed payload gave %d", code)
	}
}

// githubPush is the payload of a push of one commit
func githubPush(ref, before, after, message string) []byte {
	return mustJSON(map[string]any{
		"ref": ref, "before": before, "after": after, "compare": "https://github.com/x/y/compare/a...b",
		"pusher":     map[string]any{"name": "alice"},
		"repository": map[string]any{"full_name": "x/y", "html_url": "https://github.com/x/y"},
		"commits": []map[string]any{{"id": "abcdef1234567890", "message": message, "author": map[string]any{"name": "Alice"},
			"added": []string{"a.go"}, "modified": []string{"b.go", "c.go"}, "removed": []string{}}},
	})
}

const (
	commitBefore = "1111111111111111111111111111111111111111"
	commitAfter  = "2222222222222222222222222222222222222222"
	commitNone   = "0000000000000000000000000000000000000000"
)

// A push lists its commits, setting the webhook up says so, and any other event is reported as unknown. A push that
// creates or deletes a branch gets no notice.
func TestGithubPushAndConfig(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	withText := func(text string) func(map[string]any) bool {
		return func(content map[string]any) bool { return strings.Contains(formattedBody(content), text) }
	}

	commit := marker(t, "commit")
	last := room.Latest(t)
	postGithub(t, e, room, githubPush("refs/heads/main", commitBefore, commitAfter, commit))
	notice := formattedBody(room.WaitForBotMessage(t, last, withText(commit)))
	for _, want := range []string{"x/y", "alice", "1 commits", ">main<", "abcdef1", "Alice"} {
		if !strings.Contains(notice, want) {
			t.Errorf("the push notice has no %q: %s", want, notice)
		}
	}

	created, deleted := marker(t, "created"), marker(t, "deleted")
	last = room.Latest(t)
	postGithub(t, e, room, githubPush("refs/heads/new", commitNone, commitAfter, created))
	postGithub(t, e, room, githubPush("refs/heads/old", commitBefore, commitNone, deleted))
	// The notices go out in order, so once the next one is in, any for these would be too
	postGithub(t, e, room, mustJSON(map[string]any{"hook": map[string]any{"type": "Repository"},
		"repository": map[string]any{"full_name": "x/y", "html_url": "https://github.com/x/y"}, "sender": map[string]any{"login": "alice"}}))
	room.WaitForBotMessage(t, last, withText("configured a webhook"))
	for _, msg := range room.botMessagesAfter(t, last) {
		if body := formattedBody(decryptedRaw(msg)); strings.Contains(body, created) || strings.Contains(body, deleted) {
			t.Errorf("creating or deleting a branch got a notice: %s", body)
		}
	}

	last = room.Latest(t)
	postGithub(t, e, room, mustJSON(map[string]any{"zen": "Keep it logically awesome."}))
	room.WaitForBotMessage(t, last, bodyContains("Unknown github hook called"))

	t.Run("branch_with_a_slash", func(t *testing.T) {
		knownBug(t, "B27", "a push to a branch whose name has a slash shows the whole ref")
		commit := marker(t, "slash")
		last := room.Latest(t)
		postGithub(t, e, room, githubPush("refs/heads/feature/login", commitBefore, commitAfter, commit))
		if notice := formattedBody(room.WaitForBotMessage(t, last, withText(commit))); !strings.Contains(notice, ">feature/login<") {
			t.Errorf("the branch shows as: %s", notice)
		}
	})
}
