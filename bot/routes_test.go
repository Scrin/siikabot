package bot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/testutil/dbtest"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"maunium.net/go/mautrix/event"
)

// The HTTP routes as the bot really sets them up, with sessions on a database of the test's own

const (
	routeAdmin = "@admin:example.com"
	routeAlice = "@alice:example.com"
	routeBob   = "@bob:example.com"
	routeRoom  = "!room:example.com"
)

// publicAPIRoutes are the API routes anyone may call. Everything else under /api needs a login.
var publicAPIRoutes = map[string]bool{
	"GET /api/healthcheck":     true,
	"GET /api/metrics":         true,
	"POST /api/auth/challenge": true,
	"POST /api/auth/poll":      true,
	"GET /api/auth/me":         true, // answers 401 itself without a session
	"POST /api/auth/logout":    true,
}

// routerDatabase is the connection string of the database useRouter gave the test, for reading it directly
var routerDatabase string

// useRouter builds the bot's router on a database of the test's own, with routeAdmin as the admin
func useRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	routerDatabase = dbtest.Use(t)
	previous := config.Admin
	config.Admin = routeAdmin
	t.Cleanup(func() { config.Admin = previous })
	return newRouter()
}

// session logs someone in to the dashboard and returns their token
func session(t *testing.T, userID string) string {
	t.Helper()
	token := "token-of-" + userID
	if err := db.SetWebSessionToken(context.Background(), userID, token); err != nil {
		t.Fatal(err)
	}
	return token
}

// call makes a request to the router, with a session token if token isn't empty
func call(router *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

// concrete fills a route's parameters in
func concrete(path string) string {
	return strings.NewReplacer(":roomId", routeRoom, ":id", "1").Replace(path)
}

// Every API route but the public ones needs a login. A route added without one fails here.
func TestEveryAPIRouteNeedsALogin(t *testing.T) {
	router := useRouter(t)
	seen := map[string]bool{}
	for _, route := range router.Routes() {
		key := route.Method + " " + route.Path
		seen[key] = true
		if !strings.HasPrefix(route.Path, "/api/") || publicAPIRoutes[key] {
			continue
		}
		for _, token := range []string{"", "not-a-session"} {
			if code := call(router, route.Method, concrete(route.Path), token).Code; code != http.StatusUnauthorized {
				t.Errorf("%s with token %q gets %d, want 401", key, token, code)
			}
		}
	}
	for key := range publicAPIRoutes {
		if !seen[key] {
			t.Errorf("the public route %s no longer exists; take it off the list", key)
		}
	}
}

// Every route under /api/admin is the admin's only
func TestAdminRoutesNeedTheAdmin(t *testing.T) {
	router := useRouter(t)
	alice := session(t, routeAlice)
	var adminRoutes int
	for _, route := range router.Routes() {
		if !strings.HasPrefix(route.Path, "/api/admin/") {
			continue
		}
		adminRoutes++
		if code := call(router, route.Method, concrete(route.Path), alice).Code; code != http.StatusForbidden {
			t.Errorf("%s %s for someone who isn't the admin gets %d, want 403", route.Method, route.Path, code)
		}
	}
	if adminRoutes == 0 {
		t.Fatal("no admin routes were found")
	}
	admin := session(t, routeAdmin)
	if code := call(router, http.MethodGet, "/api/admin/chat-config", admin).Code; code != http.StatusOK {
		t.Errorf("the admin gets %d for the chat configuration, want 200", code)
	}
}

// Only someone who has joined a room sees its members and stats, and loses them on leaving
func TestRoomAccessFollowsMembership(t *testing.T) {
	router := useRouter(t)
	ctx := context.Background()
	alice, bob := session(t, routeAlice), session(t, routeBob)
	join := func(userID string, membership event.Membership) {
		t.Helper()
		if err := db.SetRoomMembership(ctx, db.RoomMember{RoomID: routeRoom, UserID: userID, Membership: membership, Since: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	join(routeAlice, event.MembershipJoin)
	join(routeBob, event.MembershipInvite)

	members := "/api/rooms/" + routeRoom + "/members"
	if response := call(router, http.MethodGet, members, alice); response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), routeAlice) {
		t.Errorf("a member gets %d: %s", response.Code, response.Body)
	}
	for _, path := range []string{members, "/api/rooms/" + routeRoom + "/stats"} {
		if code := call(router, http.MethodGet, path, bob).Code; code != http.StatusForbidden {
			t.Errorf("someone only invited gets %d for %s, want 403", code, path)
		}
	}

	join(routeAlice, event.MembershipLeave)
	if code := call(router, http.MethodGet, members, alice).Code; code != http.StatusForbidden {
		t.Errorf("after leaving, Alice gets %d for the members, want 403", code)
	}
}

// People delete their own reminders and memories, never anyone else's
func TestPeopleDeleteOnlyTheirOwn(t *testing.T) {
	router := useRouter(t)
	ctx := context.Background()
	alice, bob := session(t, routeAlice), session(t, routeBob)

	reminderID, err := db.AddReminder(ctx, db.Reminder{RemindTime: time.Now().Add(time.Hour), UserID: routeAlice, RoomID: routeRoom, Message: "dentist"})
	if err != nil {
		t.Fatal(err)
	}
	reminder := "/api/reminders/" + strconv.FormatInt(reminderID, 10)
	if code := call(router, http.MethodDelete, reminder, bob).Code; code != http.StatusNotFound {
		t.Errorf("Bob deleting Alice's reminder gets %d, want 404", code)
	}
	if left, _ := db.GetRemindersByUserID(ctx, routeAlice); len(left) != 1 {
		t.Fatal("Bob deleted Alice's reminder")
	}
	if code := call(router, http.MethodDelete, reminder, alice).Code; code != http.StatusOK {
		t.Errorf("Alice deleting her reminder gets %d, want 200", code)
	}
	if left, _ := db.GetRemindersByUserID(ctx, routeAlice); len(left) != 0 {
		t.Error("Alice's reminder is still there after she deleted it")
	}

	if err := db.SaveMemory(ctx, routeAlice, "likes cats", db.MemoryView{RoomID: routeRoom}); err != nil {
		t.Fatal(err)
	}
	memories, _ := db.GetUserMemories(ctx, routeAlice)
	call(router, http.MethodDelete, "/api/memories/"+strconv.FormatInt(memories[0].ID, 10), bob)
	call(router, http.MethodDelete, "/api/memories", bob)
	if left, _ := db.GetUserMemories(ctx, routeAlice); len(left) != 1 {
		t.Error("Bob deleted Alice's memory")
	}
}

// A session unused for longer than the timeout no longer logs anyone in
func TestExpiredSessionIsRefused(t *testing.T) {
	router := useRouter(t)
	alice := session(t, routeAlice)
	if code := call(router, http.MethodGet, "/api/reminders", alice).Code; code != http.StatusOK {
		t.Fatalf("a fresh session gets %d, want 200", code)
	}

	conn, err := pgx.Connect(context.Background(), routerDatabase)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	if _, err := conn.Exec(context.Background(),
		"UPDATE user_authorizations SET web_session_last_used_at = now() - interval '31 days'"); err != nil {
		t.Fatal(err)
	}
	if code := call(router, http.MethodGet, "/api/reminders", alice).Code; code != http.StatusUnauthorized {
		t.Errorf("an expired session gets %d, want 401", code)
	}
}

// The dashboard learns who is logged in, and whether they're the admin, from /api/auth/me
func TestAuthMe(t *testing.T) {
	router := useRouter(t)
	for _, test := range []struct {
		token string
		code  int
		admin string
	}{
		{session(t, routeAdmin), http.StatusOK, `"admin":true`},
		{session(t, routeAlice), http.StatusOK, `"admin":false`},
		{"", http.StatusUnauthorized, ""},
		{"not-a-session", http.StatusUnauthorized, ""},
	} {
		response := call(router, http.MethodGet, "/api/auth/me", test.token)
		if response.Code != test.code || !strings.Contains(response.Body.String(), test.admin) {
			t.Errorf("/api/auth/me with %q gets %d: %s", test.token, response.Code, response.Body)
		}
	}
}
