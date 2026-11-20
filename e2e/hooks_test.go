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
	payload, _ := json.Marshal(map[string]any{
		"action":       "opened",
		"pull_request": map[string]any{"html_url": "https://github.com/x/y/pull/1", "title": `<img src="mxc://evil/x">` + title},
		"repository":   map[string]any{"full_name": "x/y", "html_url": "https://github.com/x/y"},
		"sender":       map[string]any{"login": "mallory"},
	})
	mac := hmac.New(sha1.New, []byte(hookSecret))
	mac.Write(payload)

	last := room.Latest(t)
	if code := e.PostHook(t, "/hooks/github?room_id="+room.ID.String(), payload, map[string]string{
		"Content-Type": "application/json", "X-Hub-Signature": "sha1=" + hex.EncodeToString(mac.Sum(nil))}); code != http.StatusOK {
		t.Fatalf("posting the signed payload gave %d", code)
	}
	content := room.WaitForBotMessage(t, last, func(content map[string]any) bool {
		return strings.Contains(formattedBody(content), title)
	})
	if html := formattedBody(content); strings.Contains(html, "<img") || !strings.Contains(html, "&lt;img") {
		t.Errorf("the pull request's title became markup: %s", html)
	}
}
