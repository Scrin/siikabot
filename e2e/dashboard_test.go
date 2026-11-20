//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"
)

// The dashboard shows a room, its members and its stats only to those who have joined it, and stops when they
// leave
func TestDashboardShowsOnlyJoinedRooms(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice, bob := e.User(t, "alice"), e.User(t, "bob")
	shared := e.Room(t, alice, nil)
	bobsOwn := e.Room(t, bob, nil)
	aliceDashboard := e.Login(t, alice, shared)
	bobDashboard := e.Login(t, bob, bobsOwn)

	roomsOf := func(d *Dashboard) map[string]bool {
		var rooms struct {
			Rooms []struct {
				RoomID string `json:"room_id"`
			} `json:"rooms"`
		}
		if code := d.Call(t, http.MethodGet, "/api/rooms", nil, &rooms); code != http.StatusOK {
			t.Fatalf("listing the rooms gave %d", code)
		}
		ids := map[string]bool{}
		for _, r := range rooms.Rooms {
			ids[r.RoomID] = true
		}
		return ids
	}
	if !roomsOf(aliceDashboard)[shared.ID.String()] {
		t.Error("Alice's dashboard doesn't list her room")
	}
	if roomsOf(bobDashboard)[shared.ID.String()] {
		t.Error("Bob's dashboard lists Alice's room")
	}
	members := "/api/rooms/" + shared.ID.String() + "/members"
	for _, path := range []string{members, "/api/rooms/" + shared.ID.String() + "/stats"} {
		if code := bobDashboard.Call(t, http.MethodGet, path, nil, nil); code != http.StatusForbidden {
			t.Errorf("Bob gets %d for %s, want 403", code, path)
		}
	}
	if code := aliceDashboard.Call(t, http.MethodGet, members, nil, nil); code != http.StatusOK {
		t.Errorf("Alice gets %d for her room's members", code)
	}

	// Once she leaves, the bot sees it in its sync, and she loses the room
	if _, err := alice.LeaveRoom(context.Background(), shared.ID); err != nil {
		t.Fatal(err)
	}
	eventually(t, "Alice to lose the room", func() bool {
		return aliceDashboard.Call(t, http.MethodGet, members, nil, nil) == http.StatusForbidden
	})
}
