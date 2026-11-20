package bot

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// githubRequest sends a request through the signature middleware to a handler that reads the body, and returns
// the status and how much of the body was read
func githubRequest(t *testing.T, body io.Reader, signature string) (int, int64) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	counter := &countingReader{r: body}
	router := gin.New()
	router.POST("/hooks/github", GithubSignatureMiddleware("secret"), func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		c.Status(http.StatusOK)
	})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hooks/github", counter)
	req.Header.Set("X-Hub-Signature", signature)
	router.ServeHTTP(recorder, req)
	return recorder.Code, counter.n
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	c.n += int64(n)
	return n, err
}

// zeros is an endless body
type zeros struct{}

func (zeros) Read(b []byte) (int, error) {
	clear(b)
	return len(b), nil
}

func sign(body string) string {
	mac := hmac.New(sha1.New, []byte("secret"))
	mac.Write([]byte(body))
	return "sha1=" + hex.EncodeToString(mac.Sum(nil))
}

// The body is read before the signature can be checked, so anyone could otherwise send one that fills the
// memory
func TestGithubSignatureMiddlewareReadsOnlySoMuch(t *testing.T) {
	code, read := githubRequest(t, zeros{}, "sha1=0000000000000000000000000000000000000000")
	if code != http.StatusRequestEntityTooLarge {
		t.Errorf("an endless body gets %d, want 413", code)
	}
	if read > maxGithubBodyBytes+64*1024 {
		t.Errorf("%d bytes of the body were read, want at most about %d", read, maxGithubBodyBytes)
	}
}

func TestGithubSignatureMiddleware(t *testing.T) {
	body := `{"zen":"Keep it logically awesome."}`
	if code, _ := githubRequest(t, strings.NewReader(body), sign(body)); code != http.StatusOK {
		t.Errorf("a signed request gets %d, want 200", code)
	}
	if code, _ := githubRequest(t, strings.NewReader(body+" "), sign(body)); code != http.StatusUnauthorized {
		t.Errorf("a request whose body doesn't match the signature gets %d, want 401", code)
	}
	if code, _ := githubRequest(t, strings.NewReader(body), ""); code != http.StatusUnauthorized {
		t.Errorf("an unsigned request gets %d, want 401", code)
	}
}

// Anyone can open a pull request on a public repository, and the title, like everything else in a payload,
// ends up in the notice's HTML
func TestGithubNoticesEscapeThePayload(t *testing.T) {
	evil := `<a href="https://evil.example">x</a><img src="mxc://evil/x">"`
	payload := GithubPayload{Action: evil, Ref: "refs/heads/" + evil, BeforeCommit: "a", AfterCommit: "b", Compare: evil}
	payload.Repository.FullName = evil
	payload.Repository.HtmlUrl = evil
	payload.Sender.Login = evil
	payload.Pusher.Name = evil
	payload.PullRequest.HtmlUrl = evil
	payload.PullRequest.Title = evil
	payload.Commits = append(payload.Commits, struct {
		ID       string   `json:"id"`
		Message  string   `json:"message"`
		Added    []string `json:"added"`
		Modified []string `json:"modified"`
		Removed  []string `json:"removed"`
		Author   struct {
			Name string `json:"name"`
		} `json:"author"`
	}{ID: "<b>", Message: evil})
	payload.Commits[0].Author.Name = evil

	for name, notice := range map[string]string{
		"config":       githubHookConfigNotice(payload),
		"pull_request": githubPullRequestNotice(payload),
		"push":         githubPushNotice(payload),
	} {
		if strings.Contains(notice, "evil.example\"") || strings.Contains(notice, "<img") || strings.Contains(notice, "<b>") ||
			strings.Contains(notice, `<a href="https://evil`) {
			t.Errorf("the %s notice lets the payload's HTML through: %s", name, notice)
		}
		if !strings.Contains(notice, "&lt;a href=&#34;https://evil.example&#34;&gt;") {
			t.Errorf("the %s notice doesn't show the payload's text escaped: %s", name, notice)
		}
	}
}
