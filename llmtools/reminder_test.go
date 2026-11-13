package llmtools

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/commands/remind"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
)

func helsinkiTime(t *testing.T) *time.Location {
	previous := config.Timezone
	config.Timezone = "Europe/Helsinki"
	t.Cleanup(func() { config.Timezone = previous })
	return remind.Location()
}

func TestFormatReminderList(t *testing.T) {
	loc := helsinkiTime(t)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, loc)
	reminders := []db.Reminder{
		{ID: 42, RemindTime: time.Date(2026, 11, 13, 13, 30, 0, 0, loc), Message: "Call mum"},
		{ID: 7, RemindTime: time.Date(2026, 11, 20, 9, 0, 0, 0, loc), Message: "Dentist<br>bring the card"},
	}

	want := "Reminders of @alice:example.com set in this room, as of 12:00:00 on 13.11.2026:\n" +
		"- [ID: 42] at 13:30:00 on 13.11.2026 (in 1h30m0s): Call mum\n" +
		"- [ID: 7] at 09:00:00 on 20.11.2026 (in 165h0m0s): Dentist<br>bring the card\n" +
		"Reminders set in other rooms aren't shown here. They can be seen on the web dashboard."
	if got := formatReminderList("@alice:example.com", reminders, now); got != want {
		t.Errorf("formatReminderList() =\n%s\nwant\n%s", got, want)
	}
}

// An empty list mustn't read as having no reminders anywhere
func TestFormatReminderListWithNoneMentionsOtherRooms(t *testing.T) {
	loc := helsinkiTime(t)
	got := formatReminderList("@alice:example.com", nil, time.Date(2026, 11, 13, 12, 0, 0, 0, loc))
	want := "As of 12:00:00 on 13.11.2026, @alice:example.com has no reminders set in this room that are still to go off. " +
		"Reminders set in other rooms aren't shown here. They can be seen on the web dashboard."
	if got != want {
		t.Errorf("formatReminderList() = %q, want %q", got, want)
	}
}

func TestFormatReminderListSaysWhenItIsCut(t *testing.T) {
	loc := helsinkiTime(t)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, loc)
	var reminders []db.Reminder
	for i := range db.MaxListedReminders {
		reminders = append(reminders, db.Reminder{ID: int64(i), RemindTime: now.Add(time.Duration(i+1) * time.Hour), Message: "x"})
	}
	if got := formatReminderList("@alice:example.com", reminders, now); !strings.Contains(got, fmt.Sprintf("Only the first %d are listed.", db.MaxListedReminders)) {
		t.Errorf("a full list doesn't say it may be cut:\n%s", got)
	}
}

func TestDeletionResult(t *testing.T) {
	loc := helsinkiTime(t)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, loc)
	rem := db.Reminder{ID: 42, RemindTime: time.Date(2026, 11, 13, 13, 30, 0, 0, loc), Message: "Call mum"}

	tests := []struct {
		name string
		err  error
		want string
	}{
		{"deleted", nil, "Deleted reminder 42, which was due at 13:30:00 on 13.11.2026 (in 1h30m0s): Call mum\n" +
			"A notice with the command that sets it again was posted in the room, so there's no need to repeat the command."},
		{"not in this room", remind.ErrNotFound, "@alice:example.com has no reminder 42 set in this room, so nothing was deleted. " +
			"Reminders set in other rooms can only be deleted there or on the web dashboard."},
		{"already went off", remind.ErrAlreadyDelivered, "Reminder 42 already went off, so there was nothing to delete."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deletionResult("@alice:example.com", 42, rem, tt.err, now); got != tt.want {
				t.Errorf("deletionResult() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

// These fail before the tool touches the database
func TestManageRemindersRejectsIncompleteCalls(t *testing.T) {
	ctx := context.WithValue(context.WithValue(context.Background(), "room_id", "!room:example.com"), "sender", "@alice:example.com")

	tests := []struct {
		arguments string
		wantErr   string
	}{
		{`{"action": "snooze"}`, "unknown action: snooze"},
		{`{"action": "delete"}`, "reminder_id is required for the delete action"},
		{`{"action": "create", "message": "Dentist"}`, "time is required for the create action"},
		{`{"action": "create", "time": "1h"}`, "message is required for the create action"},
		{`{"action": "create", "time": "someday", "message": "Dentist"}`, "invalid date/time or duration: someday"},
	}
	for _, tt := range tests {
		t.Run(tt.arguments, func(t *testing.T) {
			_, err := handleManageRemindersToolCall(ctx, tt.arguments)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

// The room and the author come from the chat turn, so a call without them is refused rather than
// reaching for reminders anywhere
func TestManageRemindersNeedsTheTurnsRoomAndAuthor(t *testing.T) {
	if _, err := handleManageRemindersToolCall(context.Background(), `{"action": "list"}`); err == nil {
		t.Error("listing without the turn's room and author didn't fail")
	}
}
