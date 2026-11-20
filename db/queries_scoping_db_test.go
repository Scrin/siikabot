package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"maunium.net/go/mautrix/event"
	mid "maunium.net/go/mautrix/id"
)

// Who may see and delete what: reminders, people's memories, and the members of rooms

func addReminder(t *testing.T, userID, roomID, message string, in time.Duration) int64 {
	t.Helper()
	id, err := AddReminder(context.Background(), Reminder{RemindTime: time.Now().Add(in), UserID: userID, RoomID: roomID, Message: message})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// The chat deletes a reminder only for its author and only in the room it was set in. The dashboard deletes
// people's own in any room.
func TestDeleteReminderScope(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()
	id := addReminder(t, alice, roomA, "dentist", time.Hour)

	for _, scope := range []ReminderScope{
		{UserID: bob, RoomID: roomA},   // someone else, in the room
		{UserID: bob},                  // someone else, on the dashboard
		{UserID: alice, RoomID: roomB}, // its author, in another room
	} {
		if deleted, err := DeleteReminder(ctx, scope, id); err != nil || deleted != nil {
			t.Errorf("DeleteReminder(%+v) = %+v, %v, want nothing deleted", scope, deleted, err)
		}
	}
	deleted, err := DeleteReminder(ctx, ReminderScope{UserID: alice, RoomID: roomA}, id)
	if err != nil || deleted == nil || deleted.Message != "dentist" {
		t.Fatalf("DeleteReminder() by its author in its room = %+v, %v", deleted, err)
	}
	if again, _ := DeleteReminder(ctx, ReminderScope{UserID: alice}, id); again != nil {
		t.Error("a reminder was deleted twice")
	}

	other := addReminder(t, alice, roomB, "sauna", time.Hour)
	if deleted, _ := DeleteReminder(ctx, ReminderScope{UserID: alice}, other); deleted == nil {
		t.Error("its author can't delete a reminder on the dashboard")
	}
}

// The chat lists only its author's upcoming reminders in the room, soonest first
func TestGetUpcomingRemindersIn(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()
	addReminder(t, alice, roomA, "later", 2*time.Hour)
	addReminder(t, alice, roomA, "sooner", time.Hour)
	addReminder(t, alice, roomA, "past", -time.Hour)
	addReminder(t, alice, roomB, "other room", time.Hour)
	addReminder(t, bob, roomA, "bob's", time.Hour)

	reminders, err := GetUpcomingRemindersIn(ctx, alice, roomA)
	if err != nil {
		t.Fatal(err)
	}
	if len(reminders) != 2 || reminders[0].Message != "sooner" || reminders[1].Message != "later" {
		t.Errorf("GetUpcomingRemindersIn() = %+v, want Alice's two upcoming ones in the room, soonest first", reminders)
	}
	if all, _ := GetRemindersByUserID(ctx, alice); len(all) != 3 {
		t.Errorf("GetRemindersByUserID() = %d reminders, want Alice's 3 upcoming ones in every room", len(all))
	}
}

// A memory saved in a group room is used there and in DMs, one saved in a DM only in DMs
func TestMemoryViews(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()
	inA, inB, dm := MemoryView{RoomID: roomA}, MemoryView{RoomID: roomB}, MemoryView{RoomID: "!dm:example.com", IsDM: true}
	for _, saved := range []struct {
		memory string
		view   MemoryView
	}{{"likes cats", inA}, {"plays chess", inB}, {"private thing", dm}} {
		if err := SaveMemory(ctx, alice, saved.memory, saved.view); err != nil {
			t.Fatal(err)
		}
	}
	_ = SaveMemory(ctx, bob, "bob's", inA)

	memories := func(view MemoryView) []string {
		got, err := GetMemoriesIn(ctx, alice, view)
		if err != nil {
			t.Fatal(err)
		}
		var texts []string
		for _, m := range got {
			texts = append(texts, m.Memory)
		}
		return texts
	}
	if got := memories(inA); len(got) != 1 || got[0] != "likes cats" {
		t.Errorf("in room A Alice's memories are %q", got)
	}
	if got := memories(dm); len(got) != 3 {
		t.Errorf("in a DM Alice's memories are %q, want all 3", got)
	}

	all, _ := GetUserMemories(ctx, alice)
	var private int64
	for _, m := range all {
		if m.RoomID == nil {
			private = m.ID
		}
	}
	if ok, _ := DeleteMemoryIn(ctx, alice, private, inA); ok {
		t.Error("a memory saved in a DM was deleted from a group room")
	}
	if n, _ := DeleteAllMemoriesIn(ctx, alice, inB); n != 1 {
		t.Errorf("clearing Alice's memories in room B deleted %d, want 1", n)
	}
	if err := DeleteMemory(ctx, bob, private); err != nil {
		t.Fatal(err)
	}
	if got := memories(dm); len(got) != 2 {
		t.Errorf("after Bob tried to delete Alice's memory, she has %q", got)
	}
	if n, _ := DeleteAllMemories(ctx, alice); n != 2 {
		t.Errorf("DeleteAllMemories() deleted %d, want 2", n)
	}
	if bobs, _ := GetUserMemories(ctx, bob); len(bobs) != 1 {
		t.Error("clearing Alice's memories touched Bob's")
	}
}

func member(roomID, userID string, membership event.Membership, name string, since time.Time) RoomMember {
	var displayName *string
	if name != "" {
		displayName = &name
	}
	return RoomMember{RoomID: roomID, UserID: userID, Membership: membership, DisplayName: displayName, Since: since}
}

// The dashboard shows a room's members and stats only to those who have joined it, so who has joined has to be
// right
func TestRoomMembership(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()
	t1, t2 := chatT0, chatT0.Add(time.Hour)

	_ = SetRoomMembership(ctx, member(roomA, alice, event.MembershipJoin, "Alice", t1))
	_ = SetRoomMembership(ctx, member(roomA, bob, event.MembershipInvite, "", t1))
	_ = SetRoomMembership(ctx, member(roomB, alice, event.MembershipJoin, "", t1))

	if rooms, _ := FindJoinedRooms(ctx, alice); len(rooms) != 2 {
		t.Errorf("Alice has joined %q, want both rooms", rooms)
	}
	if rooms, _ := FindJoinedRooms(ctx, bob); len(rooms) != 0 {
		t.Errorf("Bob, only invited, has joined %q", rooms)
	}
	if shared, _ := FindSharedRooms(ctx, mid.UserID(bob)); len(shared) != 1 {
		t.Errorf("the rooms Bob's devices get keys for are %q, want the one he's invited to", shared)
	}

	// A new display name keeps when the membership began; a new membership starts it over
	_ = SetRoomMembership(ctx, member(roomA, alice, event.MembershipJoin, "Alice B", t2))
	_ = SetRoomMembership(ctx, member(roomA, bob, event.MembershipJoin, "Bob", t2))
	if m, ok, _ := GetRoomMember(ctx, roomA, alice); !ok || !m.Since.Equal(t1) || *m.DisplayName != "Alice B" {
		t.Errorf("after a new display name Alice's membership is %+v", m)
	}
	if m, _, _ := GetRoomMember(ctx, roomA, bob); !m.Since.Equal(t2) {
		t.Errorf("after joining, Bob's membership began %v, want when he joined", m.Since)
	}

	// Leaving, or being kicked or banned, removes someone
	_ = SetRoomMembership(ctx, member(roomA, alice, event.MembershipLeave, "", t2))
	if _, ok, _ := GetRoomMember(ctx, roomA, alice); ok {
		t.Error("Alice is still a member after leaving")
	}
	if joined, _ := GetJoinedMembers(ctx, roomA); len(joined) != 1 || joined[0].UserID != bob {
		t.Errorf("room A's joined members are %+v, want Bob", joined)
	}
}

func TestReplaceRoomMembers(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()

	suspect, err := ReplaceRoomMembers(ctx, roomA, []RoomMember{
		member(roomA, alice, event.MembershipJoin, "", chatT0),
		member(roomA, bob, event.MembershipJoin, "", chatT0),
		member(roomA, "@carol:example.com", event.MembershipLeave, "", chatT0),
	})
	if err != nil || !suspect {
		t.Errorf("replacing a room's members when none were known = %v, %v, want it flagged", suspect, err)
	}
	if suspect, _ := ReplaceRoomMembers(ctx, roomA, []RoomMember{
		member(roomA, alice, event.MembershipJoin, "", chatT0.Add(time.Hour)),
		member(roomA, bob, event.MembershipJoin, "", chatT0.Add(time.Hour)),
	}); suspect {
		t.Error("replacing a room's members with the same ones was flagged")
	}
	if m, _, _ := GetRoomMember(ctx, roomA, alice); !m.Since.Equal(chatT0) {
		t.Errorf("an unchanged membership began %v, want when it first did", m.Since)
	}
	if suspect, _ := ReplaceRoomMembers(ctx, roomA, []RoomMember{member(roomA, alice, event.MembershipJoin, "", chatT0)}); !suspect {
		t.Error("a member leaving while the bot wasn't looking wasn't flagged")
	}
	if members, _ := GetRoomMembers(ctx, mid.RoomID(roomA)); len(members) != 1 {
		t.Errorf("room A has %q, want Alice only", members)
	}
}

func TestKeepOnlyRooms(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()
	_ = SetRoomMembership(ctx, member(roomA, alice, event.MembershipJoin, "", chatT0))
	_ = SetRoomMembership(ctx, member(roomB, alice, event.MembershipJoin, "", chatT0))

	if err := KeepOnlyRooms(ctx, []string{roomB}); err != nil {
		t.Fatal(err)
	}
	if rooms, _ := FindJoinedRooms(ctx, alice); len(rooms) != 1 || rooms[0] != roomB {
		t.Errorf("after keeping room B only, Alice is in %q", rooms)
	}
	if err := KeepOnlyRooms(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if rooms, _ := FindJoinedRooms(ctx, alice); len(rooms) != 0 {
		t.Errorf("a bot in no rooms still knows Alice in %q", rooms)
	}
	if err := ForgetRoomMembers(ctx, roomA); err != nil {
		t.Fatal(err)
	}
}

func TestWebhooks(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()

	created, err := CreateWebhook(ctx, Webhook{ID: "w1", Name: "Deploys", RoomID: roomA, TokenHash: []byte("hash1")})
	if err != nil || created.Name != "Deploys" || created.LastUsedAt != nil {
		t.Fatalf("CreateWebhook() = %+v, %v", created, err)
	}
	if _, err := CreateWebhook(ctx, Webhook{ID: "w2", Name: "deploys", RoomID: roomB, TokenHash: []byte("hash2")}); !errors.Is(err, ErrWebhookNameTaken) {
		t.Errorf("a name differing only in case = %v, want it taken", err)
	}
	_, _ = CreateWebhook(ctx, Webhook{ID: "w2", Name: "alerts", RoomID: roomB, TokenHash: []byte("hash2")})

	name := "Alerts"
	if _, err := UpdateWebhook(ctx, "w1", WebhookChanges{Name: &name}); !errors.Is(err, ErrWebhookNameTaken) {
		t.Errorf("renaming to another's name = %v, want it taken", err)
	}
	if _, err := UpdateWebhook(ctx, "nope", WebhookChanges{}); !errors.Is(err, ErrWebhookNotFound) {
		t.Errorf("updating a missing webhook = %v, want not found", err)
	}
	room := roomB
	updated, err := UpdateWebhook(ctx, "w1", WebhookChanges{RoomID: &room, TokenHash: []byte("new")})
	if err != nil || updated.RoomID != roomB || string(updated.TokenHash) != "new" || updated.Name != "Deploys" {
		t.Errorf("UpdateWebhook() = %+v, %v, want only the room and token changed", updated, err)
	}

	if err := TouchWebhook(ctx, "w1"); err != nil {
		t.Fatal(err)
	}
	if webhook, ok, _ := GetWebhook(ctx, "w1"); !ok || webhook.LastUsedAt == nil {
		t.Errorf("after delivering a message the webhook is %+v", webhook)
	}
	if list, _ := ListWebhooks(ctx); len(list) != 2 || list[0].Name != "alerts" {
		t.Errorf("ListWebhooks() = %+v, want them by name regardless of case", list)
	}
	if ok, _ := DeleteWebhook(ctx, "w1"); !ok {
		t.Error("DeleteWebhook() deleted nothing")
	}
	if ok, _ := DeleteWebhook(ctx, "w1"); ok {
		t.Error("a webhook was deleted twice")
	}
	if _, ok, _ := GetWebhook(ctx, "w1"); ok {
		t.Error("a deleted webhook is still found")
	}
}
