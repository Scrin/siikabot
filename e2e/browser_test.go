//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// The dashboard in a browser: Playwright's tests (web_frontend/e2e), against this environment's bot

// frontendDir is the dashboard's source, from the e2e package's directory
const frontendDir = "../web_frontend"

var (
	dashboardOnce  sync.Once
	dashboardBuilt string
	dashboardError error
)

// buildDashboard builds the dashboard as the image does, into the run's directory, once a run
func (e *environment) buildDashboard(t *testing.T) string {
	t.Helper()
	dashboardOnce.Do(func() {
		dir := filepath.Join(e.dir, "dashboard")
		build := exec.Command("npx", "vite", "build", "--outDir", dir, "--emptyOutDir", "--logLevel", "warn")
		build.Dir = frontendDir
		if out, err := build.CombinedOutput(); err != nil {
			dashboardError = fmt.Errorf("building the dashboard: %w\n%s", err, out)
			return
		}
		dashboardBuilt = dir
	})
	if dashboardError != nil {
		t.Fatal(dashboardError)
	}
	return dashboardBuilt
}

// serveDashboard serves a built dashboard until the test ends, the way the bot serves the one built into it: its
// files, index.html for any other page, and the API, the webhooks and the metrics from the bot
func (e *environment) serveDashboard(t *testing.T, dir string) string {
	t.Helper()
	bot, err := url.Parse(e.apiURL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(bot)
	files := http.FileServer(http.Dir(dir))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/"), strings.HasPrefix(r.URL.Path, "/hooks/"), r.URL.Path == "/metrics":
			proxy.ServeHTTP(w, r)
		case r.URL.Path != "/" && !fileExists(filepath.Join(dir, filepath.Clean(r.URL.Path))):
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
		default:
			files.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// requirePlaywright fails a test that needs Playwright when it isn't installed. It's never skipped, as CI must run
// it.
func requirePlaywright(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("npx"); err != nil {
		t.Fatal("the browser tests need Node.js")
	}
	if _, err := os.Stat(filepath.Join(frontendDir, "node_modules", "@playwright", "test")); err != nil {
		t.Fatal("the browser tests need the dashboard's packages: run npm ci in web_frontend")
	}
}

// browserPerson is someone in the world the browser tests are given, with what it takes to send !auth as them
type browserPerson struct {
	UserID string `json:"userId"`
	Token  string `json:"token"`
	RoomID string `json:"roomId"`
}

func personIn(u *User, room *Room) browserPerson {
	return browserPerson{UserID: u.ID.String(), Token: u.AccessToken, RoomID: room.ID.String()}
}

// TestDashboardInABrowser runs the browser tests against a world of data, and checks afterwards what they changed.
// They log the admin in, so this doesn't run in parallel.
func TestDashboardInABrowser(t *testing.T) {
	e := setUp(t)
	requirePlaywright(t)
	adminRoom := e.Room(t, e.admin, nil)
	w := newDashboardWorld(t, e, e.Login(t, e.admin, adminRoom))
	dashboard := e.serveDashboard(t, e.buildDashboard(t))

	world, err := json.Marshal(map[string]any{
		"dashboard":  dashboard,
		"homeserver": e.synapse.URL,
		"alice":      personIn(w.alice, w.dm),
		"bob":        personIn(w.bob, w.room),
		"admin":      personIn(e.admin, adminRoom),
		"texts": map[string]string{
			"reminder": w.reminder, "memory": w.memory, "dmMemory": w.dmMemory, "roomMemory": w.roomMemory,
			"roomName": w.roomName, "webhookName": w.webhookName,
			"model": e.Query(t, "SELECT model FROM chat_config ORDER BY id DESC LIMIT 1")[0],
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	worldFile := filepath.Join(t.TempDir(), "world.json")
	if err := os.WriteFile(worldFile, world, 0o600); err != nil {
		t.Fatal(err)
	}
	// The traces of failed tests, kept with the logs when SIIKABOT_E2E_LOGDIR is set
	output := t.TempDir()
	if dir := os.Getenv("SIIKABOT_E2E_LOGDIR"); dir != "" {
		output = filepath.Join(dir, "browser")
	}

	playwright := exec.Command("npx", "playwright", "test")
	playwright.Dir = frontendDir
	playwright.Env = append(os.Environ(), "SIIKABOT_E2E_WORLD="+worldFile, "SIIKABOT_E2E_BROWSER_OUTPUT="+output)
	out, err := playwright.CombinedOutput()
	t.Logf("Playwright:\n%s", out)
	if err != nil {
		t.Fatalf("the browser tests failed: %v. Their traces are in %s.", err, output)
	}

	// The reminder deleted in the browser is gone from the bot too
	if left := e.Query(t, "SELECT count(*) FROM reminders WHERE message = $1", w.reminder)[0]; left != "0" {
		t.Error("the reminder deleted on the dashboard is still stored")
	}
}
