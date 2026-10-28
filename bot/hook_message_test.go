package bot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/webhooks"
	"github.com/gin-gonic/gin"
)

const testWebhookToken = "correct-horse-battery-staple"

// messageHookRouter routes requests to a webhook with the given ID and token through the auth
// middleware, to a handler that answers with the message the request brings
func messageHookRouter(webhookID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	lookUp := func(ctx context.Context, id string) (db.Webhook, bool, error) {
		if id != webhookID {
			return db.Webhook{}, false, nil
		}
		return db.Webhook{ID: id, Name: "hermes", TokenHash: webhooks.HashToken(testWebhookToken)}, true, nil
	}
	router := gin.New()
	router.POST("/hooks/message/:id", MessageHookAuthMiddleware(lookUp), func(c *gin.Context) {
		message, status, err := readHookMessage(c)
		if err != nil {
			c.String(status, err.Error())
			return
		}
		c.String(http.StatusOK, c.MustGet(messageHookWebhookKey).(db.Webhook).Name+": "+message)
	})
	return router
}

func postToHook(router *gin.Engine, path, authorization, contentType, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestMessageHookTurnsAwayEveryBadTokenTheSameWay(t *testing.T) {
	router := messageHookRouter("abc123")
	tests := []struct {
		name          string
		path          string
		authorization string
	}{
		{"no header", "/hooks/message/abc123", ""},
		{"another scheme", "/hooks/message/abc123", "Basic " + testWebhookToken},
		{"no token", "/hooks/message/abc123", "Bearer "},
		{"wrong token", "/hooks/message/abc123", "Bearer correct-horse-battery-stapler"},
		{"unknown webhook", "/hooks/message/nope", "Bearer " + testWebhookToken},
	}

	var first string
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postToHook(router, tt.path, tt.authorization, "text/plain", "hello")
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if rec.Header().Get("WWW-Authenticate") == "" {
				t.Error("no WWW-Authenticate header")
			}
			if first == "" {
				first = rec.Body.String()
			} else if rec.Body.String() != first {
				t.Errorf("answered %q, unlike the others' %q", rec.Body.String(), first)
			}
		})
	}
}

func TestMessageHookLetsTheRightTokenThrough(t *testing.T) {
	router := messageHookRouter("abc123")
	for _, scheme := range []string{"Bearer", "bearer"} {
		rec := postToHook(router, "/hooks/message/abc123", scheme+" "+testWebhookToken, "text/plain", "hello there\n")
		if rec.Code != http.StatusOK || rec.Body.String() != "hermes: hello there" {
			t.Errorf("with %q: %d %q, want the message through", scheme, rec.Code, rec.Body.String())
		}
	}
}

func TestMessageHookFailsWhenTheWebhookCantBeLookedUp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/hooks/message/:id", MessageHookAuthMiddleware(func(context.Context, string) (db.Webhook, bool, error) {
		return db.Webhook{}, false, errors.New("database down")
	}), func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := postToHook(router, "/hooks/message/abc123", "Bearer "+testWebhookToken, "text/plain", "hello")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestMessageHookReadsTheMessage(t *testing.T) {
	router := messageHookRouter("abc123")
	tests := []struct {
		name        string
		contentType string
		body        string
		status      int
		want        string
	}{
		{"plain text", "text/plain", "hello", http.StatusOK, "hermes: hello"},
		{"JSON", "application/json", `{"message": "hello"}`, http.StatusOK, "hermes: hello"},
		{"as large as allowed", "text/plain", strings.Repeat("a", webhooks.MaxBodyBytes), http.StatusOK, "hermes: " + strings.Repeat("a", webhooks.MaxBodyBytes)},
		{"too large", "text/plain", strings.Repeat("a", webhooks.MaxBodyBytes+1), http.StatusRequestEntityTooLarge, webhooks.ErrMessageTooLarge.Error()},
		{"empty", "text/plain", "\n", http.StatusBadRequest, webhooks.ErrEmptyMessage.Error()},
		{"broken JSON", "application/json", `{"message":`, http.StatusBadRequest, webhooks.ErrInvalidJSON.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postToHook(router, "/hooks/message/abc123", "Bearer "+testWebhookToken, tt.contentType, tt.body)
			if rec.Code != tt.status || rec.Body.String() != tt.want {
				body := rec.Body.String()
				if len(body) > 80 {
					body = body[:80] + "…"
				}
				t.Errorf("%d %q, want %d", rec.Code, body, tt.status)
			}
		})
	}
}

// The token never reaches the handler's error answers, whatever goes wrong
func TestMessageHookErrorsAreJSON(t *testing.T) {
	router := messageHookRouter("abc123")
	rec := postToHook(router, "/hooks/message/abc123", "Bearer wrong-token-that-is-long", "text/plain", "hello")
	var answer map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil || answer["error"] == "" {
		t.Errorf("answered %q, want a JSON error", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "wrong-token") {
		t.Errorf("answered %q, which repeats the token", rec.Body.String())
	}
}
