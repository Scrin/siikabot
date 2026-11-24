//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The API's contract with the dashboard. Each endpoint the dashboard calls has a fixture: a real response, saved
// as a TypeScript file that gives it the type the dashboard expects (web_frontend/src/api/contract). tsc checks
// that the type fits the response, extra fields included, and this test checks that the responses still have the
// fixtures' shape. A Go change to a response fails here; saving the fixtures again then fails tsc until types.ts
// follows.

// contractDir is where the fixtures are, from the e2e package's directory
const contractDir = "../web_frontend/src/api/contract"

// updateFixturesEnv saves the fixtures whose shape the responses no longer have, and the missing ones
const updateFixturesEnv = "SIIKABOT_E2E_UPDATE_FIXTURES"

// contract checks responses against their fixtures
type contract struct {
	t       *testing.T
	checked []string
}

// check checks a response against its fixture: name is the fixture's, endpoint what gave the response, tsType the
// type in types.ts it has, and maps the fields that are maps from keys to values rather than objects with fields
func (c *contract) check(name, endpoint, tsType string, response []byte, maps ...string) {
	c.t.Helper()
	c.checked = append(c.checked, name)
	var live any
	if err := json.Unmarshal(response, &live); err != nil {
		c.t.Fatalf("%s: the response isn't JSON: %v: %s", endpoint, err, response)
	}
	isMap := map[string]bool{}
	for _, m := range maps {
		isMap[m] = true
	}

	path := filepath.Join(contractDir, name+".ts")
	saved, err := readFixture(path)
	var differences []string
	switch {
	case os.IsNotExist(err):
		differences = []string{"there's no fixture"}
	case err != nil:
		c.t.Fatalf("%s: %v", path, err)
	default:
		differences = compareShapes("", shapeOf(live, isMap), shapeOf(saved, isMap))
	}
	if len(differences) == 0 {
		return
	}
	if os.Getenv(updateFixturesEnv) == "" {
		c.t.Errorf("%s no longer has the shape of its fixture %s:\n  %s\nRun the test with %s=1 to save it again, and "+
			"bring %s in web_frontend/src/api/types.ts in line until tsc passes.",
			endpoint, path, strings.Join(differences, "\n  "), updateFixturesEnv, tsType)
		return
	}
	if err := writeFixture(path, name, endpoint, tsType, response); err != nil {
		c.t.Fatal(err)
	}
	c.t.Logf("saved %s: %s", path, strings.Join(differences, "; "))
}

// finish checks that every fixture belongs to a response this test checks
func (c *contract) finish() {
	c.t.Helper()
	files, err := filepath.Glob(filepath.Join(contractDir, "*.ts"))
	if err != nil {
		c.t.Fatal(err)
	}
	for _, file := range files {
		if name := strings.TrimSuffix(filepath.Base(file), ".ts"); !slices.Contains(c.checked, name) {
			if os.Getenv(updateFixturesEnv) == "" {
				c.t.Errorf("no response is checked against %s; run the test with %s=1 to remove it", file, updateFixturesEnv)
			} else if err := os.Remove(file); err != nil {
				c.t.Fatal(err)
			}
		}
	}
}

// fixtureStart is what comes before the response in a fixture
const fixtureStart = "\nexport const "

func writeFixture(path, name, endpoint, tsType string, response []byte) error {
	var indented bytes.Buffer
	if err := json.Indent(&indented, response, "", "  "); err != nil {
		return err
	}
	content := fmt.Sprintf(`// A real response of %s, saved by the end-to-end test TestAPIContract. tsc checks that %s fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// %s=1 to save it again.
import type { %s } from '../types'
%s%s: %s = %s
`, endpoint, tsType, updateFixturesEnv, tsType, fixtureStart, camelCase(name), tsType, indented.String())
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func readFixture(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(data)
	start := strings.Index(content, fixtureStart)
	if start < 0 {
		return nil, fmt.Errorf("no %q in it", strings.TrimSpace(fixtureStart))
	}
	assignment := strings.Index(content[start:], " = ")
	if assignment < 0 {
		return nil, fmt.Errorf("no response in it")
	}
	var saved any
	if err := json.Unmarshal([]byte(content[start+assignment+3:]), &saved); err != nil {
		return nil, fmt.Errorf("its response isn't JSON: %w", err)
	}
	return saved, nil
}

// camelCase turns a fixture's name into its constant's
func camelCase(name string) string {
	parts := strings.Split(name, "_")
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}

// shape is what a JSON value is made of, without its values: the kinds of its values, the fields of its objects,
// and the shape of every element of its arrays and maps together
type shape struct {
	kind   string // null, boolean, number, string, array, map, object, or mixed
	fields map[string]*shape
	elem   *shape // nil for an empty array or map
}

func shapeOf(v any, maps map[string]bool) *shape {
	switch v := v.(type) {
	case nil:
		return &shape{kind: "null"}
	case bool:
		return &shape{kind: "boolean"}
	case float64:
		return &shape{kind: "number"}
	case string:
		return &shape{kind: "string"}
	case []any:
		s := &shape{kind: "array"}
		for _, e := range v {
			s.elem = mergeShapes(s.elem, shapeOf(e, maps))
		}
		return s
	case map[string]any:
		s := &shape{kind: "object", fields: map[string]*shape{}}
		for key, value := range v {
			if m, ok := value.(map[string]any); ok && maps[key] {
				field := &shape{kind: "map"}
				for _, e := range m {
					field.elem = mergeShapes(field.elem, shapeOf(e, maps))
				}
				s.fields[key] = field
				continue
			}
			s.fields[key] = shapeOf(value, maps)
		}
		return s
	}
	panic(fmt.Sprintf("not a JSON value: %T", v))
}

// mergeShapes is the shape of values that may have either shape. A null merges into anything, as JSON's nulls
// stand for values that aren't there.
func mergeShapes(a, b *shape) *shape {
	switch {
	case a == nil || a.kind == "null":
		if b == nil {
			return a
		}
		return b
	case b == nil || b.kind == "null":
		return a
	case a.kind != b.kind:
		return &shape{kind: "mixed"}
	}
	merged := &shape{kind: a.kind, elem: mergeShapes(a.elem, b.elem)}
	if a.kind == "object" {
		merged.fields = map[string]*shape{}
		for key, field := range a.fields {
			merged.fields[key] = mergeShapes(field, b.fields[key])
		}
		for key, field := range b.fields {
			if _, ok := a.fields[key]; !ok {
				merged.fields[key] = field
			}
		}
	}
	return merged
}

// compareShapes lists how a response's shape differs from its fixture's. A null or an empty array matches
// anything, since one response can't show every value a field may have.
func compareShapes(path string, live, saved *shape) []string {
	if live == nil || saved == nil || live.kind == "null" || saved.kind == "null" {
		return nil
	}
	where := path
	if where == "" {
		where = "the response"
	}
	if live.kind != saved.kind {
		return []string{fmt.Sprintf("%s is a %s, and a %s in the fixture", where, live.kind, saved.kind)}
	}
	switch live.kind {
	case "array":
		return compareShapes(path+"[]", live.elem, saved.elem)
	case "map":
		return compareShapes(path+"{}", live.elem, saved.elem)
	case "object":
		var differences []string
		keys := map[string]bool{}
		for key := range live.fields {
			keys[key] = true
		}
		for key := range saved.fields {
			keys[key] = true
		}
		sorted := make([]string, 0, len(keys))
		for key := range keys {
			sorted = append(sorted, key)
		}
		sort.Strings(sorted)
		for _, key := range sorted {
			field := strings.TrimPrefix(path+"."+key, ".")
			liveField, inLive := live.fields[key]
			savedField, inSaved := saved.fields[key]
			switch {
			case !inSaved:
				differences = append(differences, field+" is new: the fixture doesn't have it")
			case !inLive:
				differences = append(differences, field+" is gone: only the fixture has it")
			default:
				differences = append(differences, compareShapes(field, liveField, savedField)...)
			}
		}
		return differences
	}
	return nil
}

// TestAPIContract checks every response the dashboard reads against its fixture, with data in every list. It logs
// the admin in, so it doesn't run in parallel.
func TestAPIContract(t *testing.T) {
	e := setUp(t)
	admin := e.Login(t, e.admin, e.Room(t, e.admin, nil))
	w := newDashboardWorld(t, e, admin)
	c := &contract{t: t}
	call := func(d *Dashboard, method, path string, body any, want int) []byte {
		t.Helper()
		var response json.RawMessage
		token := ""
		if d != nil {
			token = d.Token
		}
		if code := e.api(t, method, path, token, body, &response); code != want {
			t.Fatalf("%s %s gave %d, want %d: %s", method, path, code, want, response)
		}
		return response
	}

	// Alice logs in the way the browser does, so the login's responses are checked too
	challengeResponse := call(nil, http.MethodPost, "/api/auth/challenge", nil, http.StatusOK)
	c.check("challenge", "POST /api/auth/challenge", "ChallengeResponse", challengeResponse)
	var challenge struct {
		Challenge  string `json:"challenge"`
		PollSecret string `json:"poll_secret"`
	}
	if err := json.Unmarshal(challengeResponse, &challenge); err != nil {
		t.Fatal(err)
	}
	poll := func() []byte {
		return call(nil, http.MethodPost, "/api/auth/poll",
			map[string]string{"challenge": challenge.Challenge, "poll_secret": challenge.PollSecret}, http.StatusOK)
	}
	c.check("poll_pending", "POST /api/auth/poll", "PollResponse", poll())
	w.dm.WaitForBotMessage(t, w.alice.Say(t, w.dm, "!auth "+challenge.Challenge), bodyContains("Authenticated for web access"))
	var authenticated []byte
	var login struct {
		Status string `json:"status"`
		Token  string `json:"token"`
	}
	eventually(t, "the login to complete", func() bool {
		authenticated = poll()
		return json.Unmarshal(authenticated, &login) == nil && login.Status == "authenticated"
	})
	c.check("poll_authenticated", "POST /api/auth/poll", "PollResponse", authenticated)
	alice := &Dashboard{env: e, Token: login.Token}
	room := w.room.ID.String()

	// The counts are written as the messages are handled, and the usage at the end of each turn
	eventually(t, "the room's messages to be counted", func() bool {
		var stats struct {
			People struct {
				Messages int `json:"messages"`
			} `json:"people"`
		}
		_ = json.Unmarshal(call(alice, http.MethodGet, "/api/rooms/"+room+"/stats", nil, http.StatusOK), &stats)
		return stats.People.Messages >= 3
	})
	eventually(t, "the failed turn's usage", func() bool {
		var usage struct {
			Totals struct {
				Failures int `json:"failures"`
			} `json:"totals"`
		}
		_ = json.Unmarshal(call(admin, http.MethodGet, "/api/admin/chat-usage", nil, http.StatusOK), &usage)
		return usage.Totals.Failures > 0
	})

	c.check("healthcheck", "GET /api/healthcheck", "HealthCheckResponse", call(nil, http.MethodGet, "/api/healthcheck", nil, http.StatusOK))
	c.check("metrics", "GET /api/metrics", "MetricsResponse", call(nil, http.MethodGet, "/api/metrics", nil, http.StatusOK))
	c.check("me", "GET /api/auth/me", "MeResponse", call(alice, http.MethodGet, "/api/auth/me", nil, http.StatusOK))
	c.check("unauthorized", "GET /api/auth/me without a session", "AuthErrorResponse",
		call(nil, http.MethodGet, "/api/auth/me", nil, http.StatusUnauthorized))

	c.check("reminders", "GET /api/reminders", "RemindersResponse", call(alice, http.MethodGet, "/api/reminders", nil, http.StatusOK))
	c.check("rooms", "GET /api/rooms", "RoomsResponse", call(alice, http.MethodGet, "/api/rooms", nil, http.StatusOK))
	c.check("room_members", "GET /api/rooms/:roomId/members", "RoomMembersResponse",
		call(alice, http.MethodGet, "/api/rooms/"+room+"/members", nil, http.StatusOK))
	c.check("room_stats", "GET /api/rooms/:roomId/stats", "RoomStatsResponse",
		call(alice, http.MethodGet, "/api/rooms/"+room+"/stats", nil, http.StatusOK))
	c.check("memories", "GET /api/memories", "MemoriesResponse", call(alice, http.MethodGet, "/api/memories", nil, http.StatusOK))

	c.check("admin_rooms", "GET /api/admin/rooms", "RoomsResponse", call(admin, http.MethodGet, "/api/admin/rooms", nil, http.StatusOK))
	c.check("admin_room_members", "GET /api/admin/rooms/:roomId/members", "RoomMembersResponse",
		call(admin, http.MethodGet, "/api/admin/rooms/"+room+"/members", nil, http.StatusOK))
	c.check("admin_room_stats", "GET /api/admin/rooms/:roomId/stats", "RoomStatsResponse",
		call(admin, http.MethodGet, "/api/admin/rooms/"+room+"/stats", nil, http.StatusOK))
	c.check("room_memories", "GET /api/admin/room-memories", "RoomMemoriesResponse",
		call(admin, http.MethodGet, "/api/admin/room-memories", nil, http.StatusOK))
	c.check("reflect", "POST /api/admin/rooms/:roomId/reflect", "ReflectResponse", w.reflected)
	c.check("chat_usage", "GET /api/admin/chat-usage", "ChatUsageResponse",
		call(admin, http.MethodGet, "/api/admin/chat-usage?days=7", nil, http.StatusOK), "failures_by_outcome")
	c.check("chat_config", "GET /api/admin/chat-config", "ChatConfigResponse",
		call(admin, http.MethodGet, "/api/admin/chat-config", nil, http.StatusOK))
	c.check("message_activity", "GET /api/admin/message-activity", "MessageActivityResponse",
		call(admin, http.MethodGet, "/api/admin/message-activity?days=30", nil, http.StatusOK))
	c.check("webhooks", "GET /api/admin/webhooks", "WebhooksResponse", call(admin, http.MethodGet, "/api/admin/webhooks", nil, http.StatusOK))
	c.check("webhook_created", "POST /api/admin/webhooks", "SavedWebhookResponse", w.webhookCreated)
	c.check("webhook_updated", "PATCH /api/admin/webhooks/:id", "SavedWebhookResponse", w.webhookUpdated)

	// Last, since they delete what the others show
	c.check("memories_deleted", "DELETE /api/memories", "DeleteAllMemoriesResponse",
		call(alice, http.MethodDelete, "/api/memories", nil, http.StatusOK))
	c.check("room_memories_deleted", "DELETE /api/admin/rooms/:roomId/memories", "DeleteRoomMemoriesResponse",
		call(admin, http.MethodDelete, "/api/admin/rooms/"+room+"/memories", nil, http.StatusOK))
	c.finish()
}
