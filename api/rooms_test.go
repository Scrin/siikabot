package api

import (
	"encoding/json"
	"testing"

	"github.com/Scrin/siikabot/db"
)

func TestMembersResponseShowsDisplayNames(t *testing.T) {
	alice := "Alice"
	response := membersResponse([]db.RoomMember{
		{RoomID: "!room:example.com", UserID: "@alice:example.com", Membership: "join", DisplayName: &alice},
		{RoomID: "!room:example.com", UserID: "@nameless:example.com", Membership: "join"},
	})

	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"members":[{"user_id":"@alice:example.com","display_name":"Alice"},{"user_id":"@nameless:example.com"}]}`
	if string(encoded) != want {
		t.Errorf("members encode as %s, want %s", encoded, want)
	}
}

// A room nobody else is in yet still lists as a room with no members, not as a missing list
func TestMembersResponseWithoutMembers(t *testing.T) {
	encoded, err := json.Marshal(membersResponse(nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"members":[]}` {
		t.Errorf("no members encode as %s", encoded)
	}
}

func TestRoomsResponseNamesEachRoom(t *testing.T) {
	names := map[string]string{"!named:example.com": "Siika HQ"}
	response := roomsResponse([]string{"!named:example.com", "!unnamed:example.com"}, func(roomID string) string {
		return names[roomID]
	})

	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"rooms":[{"room_id":"!named:example.com","room_name":"Siika HQ"},{"room_id":"!unnamed:example.com"}]}`
	if string(encoded) != want {
		t.Errorf("rooms encode as %s, want %s", encoded, want)
	}
}
