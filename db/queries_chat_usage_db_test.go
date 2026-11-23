package db

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Usage older than its retention leaves, the passes' with the turns', and newer usage stays
func TestCleanupOldChatUsage(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()
	for _, kind := range []string{"turn", "reflection"} {
		for _, room := range []string{"!old:example.org", "!new:example.org"} {
			if err := SaveChatUsage(ctx, ChatUsage{Kind: kind, RoomID: room, UserID: "@alice:example.org", Model: "m",
				Outcome: "ok"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := pool.Exec(ctx, "UPDATE chat_usage SET timestamp = now() - interval '91 days' WHERE room_id = '!old:example.org'"); err != nil {
		t.Fatal(err)
	}

	if n, err := CleanupOldChatUsage(ctx, 90*24*time.Hour); err != nil || n != 2 {
		t.Errorf("CleanupOldChatUsage() = %d, %v, want the 2 old rows", n, err)
	}
	var rooms []string
	rows, err := pool.Query(ctx, "SELECT room_id || ' ' || kind FROM chat_usage ORDER BY kind")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var room string
		if err := rows.Scan(&room); err != nil {
			t.Fatal(err)
		}
		rooms = append(rooms, room)
	}
	if got := strings.Join(rooms, ", "); got != "!new:example.org reflection, !new:example.org turn" {
		t.Errorf("left %s, want the new turn and pass", got)
	}
}
