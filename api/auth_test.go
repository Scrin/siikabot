package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Scrin/siikabot/auth"
	"github.com/gin-gonic/gin"
)

// poll posts a poll body to PollHandler and returns the status code and response
func poll(t *testing.T, body string) (int, PollResponse) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/auth/poll", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	PollHandler(c)

	var response PollResponse
	_ = json.Unmarshal(recorder.Body.Bytes(), &response)
	return recorder.Code, response
}

func pollBody(challenge, pollSecret string) string {
	body, _ := json.Marshal(PollRequest{Challenge: challenge, PollSecret: pollSecret})
	return string(body)
}

func TestPollHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth.Init()
	ctx := context.Background()
	challenge, pollSecret, _, err := auth.GenerateChallenge(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if code, response := poll(t, pollBody(challenge, pollSecret)); code != http.StatusOK || response.Status != "pending" {
		t.Errorf("before !auth: %d %+v, want pending", code, response)
	}
	if code, _ := poll(t, pollBody(challenge, "wrong")); code != http.StatusForbidden {
		t.Errorf("with the wrong poll secret: %d, want 403", code)
	}
	for _, body := range []string{"", "{}", `{"challenge":"` + challenge + `"}`, "not json"} {
		if code, _ := poll(t, body); code != http.StatusBadRequest {
			t.Errorf("poll with body %q: %d, want 400", body, code)
		}
	}

	if err := auth.CompleteChallenge(ctx, challenge, "@alice:example.com", func() (string, error) { return "alice-token", nil }); err != nil {
		t.Fatal(err)
	}
	if code, _ := poll(t, pollBody(challenge, "wrong")); code != http.StatusForbidden {
		t.Errorf("with the wrong poll secret after !auth: %d, want 403", code)
	}
	code, response := poll(t, pollBody(challenge, pollSecret))
	if code != http.StatusOK || response.Status != "authenticated" || response.Token != "alice-token" ||
		response.UserID != "@alice:example.com" {
		t.Errorf("after !auth: %d %+v, want Alice's token", code, response)
	}
}
