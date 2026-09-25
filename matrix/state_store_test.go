package matrix

import (
	"testing"
	"time"

	"maunium.net/go/mautrix/event"
)

func TestRoomMemberReadsAMembershipEvent(t *testing.T) {
	sent := time.Date(2026, 9, 25, 14, 2, 0, 0, time.UTC)

	member := roomMember("!room:example.com", "@alice:example.com",
		&event.MemberEventContent{Membership: event.MembershipJoin, Displayname: "Alice"}, sent.UnixMilli())
	if member.UserID != "@alice:example.com" || member.Membership != event.MembershipJoin {
		t.Errorf("roomMember() = %#v, want Alice's join", member)
	}
	if member.DisplayName == nil || *member.DisplayName != "Alice" {
		t.Errorf("roomMember() has display name %v, want Alice", member.DisplayName)
	}
	if !member.Since.Equal(sent) {
		t.Errorf("roomMember() has since %v, want the event's time %v", member.Since, sent)
	}
}

func TestRoomMemberWithoutANameOrATime(t *testing.T) {
	before := time.Now()
	member := roomMember("!room:example.com", "@bob:example.com", &event.MemberEventContent{Membership: event.MembershipInvite}, 0)

	if member.DisplayName != nil {
		t.Errorf("a member without a display name has %q", *member.DisplayName)
	}
	// Taking the time as now errs towards a membership that began later, which the history
	// visibility check treats as seeing less
	if member.Since.Before(before) {
		t.Errorf("an event without a time gives since %v, want now", member.Since)
	}
}
