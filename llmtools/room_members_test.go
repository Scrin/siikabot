package llmtools

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

var testMembers = []RoomMember{
	{UserID: "@alice:example.com", Name: "Alice Smith"},
	{UserID: "@bob:example.com", Name: "Bob"},
	{UserID: "@smithy:example.com", Name: "@smithy:example.com"},
	{UserID: "@siikabot:example.com", Name: "Siikabot", IsBot: true},
}

func TestFindMembers(t *testing.T) {
	got := findMembers(testMembers, "SMITH")
	want := "Members of this room matching \"smith\":\n- Alice Smith (@alice:example.com)\n- @smithy:example.com"
	if got != want {
		t.Errorf("findMembers() =\n%s\nwant\n%s", got, want)
	}

	if got := findMembers(testMembers, "siika"); !strings.Contains(got, "- Siikabot (@siikabot:example.com) (you)") {
		t.Errorf("the bot isn't marked: %s", got)
	}
	if got := findMembers(testMembers, "bob:example"); !strings.Contains(got, "- Bob (@bob:example.com)") {
		t.Errorf("a user ID didn't match: %s", got)
	}
	if got := findMembers(testMembers, "carol"); got != `No member of this room matches "carol".` {
		t.Errorf("no match reads %q", got)
	}
}

func TestFindMembersCapsTheList(t *testing.T) {
	var members []RoomMember
	for i := range 25 {
		members = append(members, RoomMember{UserID: fmt.Sprintf("@member%02d:example.com", i), Name: fmt.Sprintf("Member %02d", i)})
	}

	got := findMembers(members, "member")
	if lines := strings.Count(got, "\n- "); lines != maxMemberMatches {
		t.Errorf("findMembers() listed %d members, want %d", lines, maxMemberMatches)
	}
	if !strings.HasSuffix(got, "(and 5 more: ask with a longer query to narrow it down)") {
		t.Errorf("findMembers() doesn't say how many more match:\n%s", got)
	}
}

func TestRoomMembersToolCall(t *testing.T) {
	ctx := context.WithValue(context.Background(), "room_members", testMembers)

	got, err := handleRoomMembersToolCall(ctx, `{"query": "bob"}`)
	if err != nil || !strings.Contains(got, "- Bob (@bob:example.com)") {
		t.Errorf("handleRoomMembersToolCall() = %q, %v", got, err)
	}

	if _, err := handleRoomMembersToolCall(ctx, `{"query": "  "}`); err == nil {
		t.Error("an empty query was accepted")
	}
	if _, err := handleRoomMembersToolCall(ctx, `not json`); err == nil {
		t.Error("arguments that aren't JSON were accepted")
	}
	// Without the members of the turn's room, there is nothing to look anyone up in
	if _, err := handleRoomMembersToolCall(context.Background(), `{"query": "bob"}`); err == nil {
		t.Error("a lookup without the room's members succeeded")
	}
}
