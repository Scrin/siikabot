package bot

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLimitRequestBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/", limitRequestBody(10), func(c *gin.Context) {
		if _, err := io.ReadAll(c.Request.Body); err != nil {
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}
		c.Status(http.StatusOK)
	})

	for body, want := range map[string]int{"short": http.StatusOK, strings.Repeat("x", 11): http.StatusRequestEntityTooLarge} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
		if recorder.Code != want {
			t.Errorf("a %d byte body gets %d, want %d", len(body), recorder.Code, want)
		}
	}
}

func TestAlertmanagerBasicAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/", AlertmanagerBasicAuthMiddleware("alertmanager", "hunter2"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for _, test := range []struct {
		user, password string
		set            bool
		want           int
	}{
		{"alertmanager", "hunter2", true, http.StatusOK},
		{"alertmanager", "hunter3", true, http.StatusUnauthorized},
		{"alertmanager", "hunter", true, http.StatusUnauthorized},
		{"someone", "hunter2", true, http.StatusUnauthorized},
		{"", "", true, http.StatusUnauthorized},
		{"", "", false, http.StatusUnauthorized},
	} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		if test.set {
			req.SetBasicAuth(test.user, test.password)
		}
		router.ServeHTTP(recorder, req)
		if recorder.Code != test.want {
			t.Errorf("%q:%q gets %d, want %d", test.user, test.password, recorder.Code, test.want)
		}
	}
}

// Other sites mustn't show the dashboard in a frame
func TestSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(securityHeaders())
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := recorder.Header().Get("Content-Security-Policy"); got != "frame-ancestors 'none'" {
		t.Errorf("Content-Security-Policy = %q", got)
	}
	if got := recorder.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q", got)
	}
}

// A request that reaches the bot directly can't claim another address with X-Forwarded-For; one through a
// proxy on a private network can
func TestClientIPTrustsOnlyPrivateProxies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if err := router.SetTrustedProxies(trustedProxies); err != nil {
		t.Fatal(err)
	}
	var clientIP string
	router.GET("/", func(c *gin.Context) { clientIP = c.ClientIP() })

	for remote, want := range map[string]string{"172.18.0.5:41000": "198.51.100.7", "203.0.113.9:41000": "203.0.113.9"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = remote
		req.Header.Set("X-Forwarded-For", "198.51.100.7")
		router.ServeHTTP(httptest.NewRecorder(), req)
		if clientIP != want {
			t.Errorf("from %s, ClientIP() = %s, want %s", remote, clientIP, want)
		}
	}
}
