package chat

import (
	"context"
	"testing"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
)

const testRoomID = "!room:example.com"

func TestReferencedMessageWithoutAReference(t *testing.T) {
	quote, image, notes := referencedMessage(context.Background(), Trigger{RoomID: testRoomID, Body: "hi"}, testRoom("", alice, botSelf))

	if quote != nil || image != "" || notes != nil {
		t.Errorf("referencedMessage() = %v, %q, %v, want nothing", quote, image, notes)
	}
}

func TestReferencedMessageQuotesTheReferencedText(t *testing.T) {
	sentAt := time.Date(2026, 9, 25, 13, 58, 0, 0, time.UTC)
	trigger := Trigger{
		RoomID:         testRoomID,
		Body:           "what do you think about this?",
		ReplyToEventID: "$carol",
		ReplyTo: &matrix.Message{
			RoomID:    testRoomID,
			EventID:   "$carol",
			Sender:    "@carol:example.com",
			Timestamp: sentAt,
			MsgType:   "m.text",
			Body:      "pineapple belongs on pizza",
		},
	}

	quote, image, notes := referencedMessage(context.Background(), trigger, testRoom("", alice, carol, botSelf))

	want := db.QuotedMessage{
		EventID: "$carol", Sender: "@carol:example.com", SenderName: "Carol",
		SentAt: sentAt, Kind: db.QuoteText, Body: "pineapple belongs on pizza",
	}
	if quote == nil || *quote != want {
		t.Errorf("quote = %#v, want %#v", quote, want)
	}
	if image != "" || notes != nil {
		t.Errorf("image = %q, notes = %v, want neither", image, notes)
	}
}

// A reference that can't be read is still worth mentioning: the model should know the message
// replies to something, rather than answer as if it stood alone. Nothing is quoted, since there
// is nothing to quote.
func TestReferencedMessageThatCouldNotBeRead(t *testing.T) {
	tests := []struct {
		name    string
		replyTo *matrix.Message
	}{
		{
			name:    "fetch failed",
			replyTo: nil,
		},
		{
			name:    "content could not be decrypted",
			replyTo: &matrix.Message{RoomID: testRoomID, EventID: "$secret", Sender: "@carol:example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trigger := Trigger{RoomID: testRoomID, Body: "and this?", ReplyToEventID: "$secret", ReplyTo: tt.replyTo}

			quote, image, notes := referencedMessage(context.Background(), trigger, testRoom("", alice, carol, botSelf))

			if quote != nil || image != "" {
				t.Errorf("quote = %v, image = %q, want neither", quote, image)
			}
			if len(notes) != 1 || notes[0] != unreadableReplyNote {
				t.Errorf("notes = %v, want the unreadable reply note", notes)
			}
		})
	}
}

func TestMentionsOf(t *testing.T) {
	room := testRoom("", alice, bob, carol, botSelf)
	carolPill := `<a href="https://matrix.to/#/@carol:example.com">Carol</a>`
	bobPill := `<a href="https://matrix.to/#/@bob:example.com">Bob</a>`

	tests := []struct {
		name    string
		trigger Trigger
		want    []string
	}{
		{
			name:    "m.mentions is the record, the bot left out",
			trigger: Trigger{Mentions: []string{testBotUserID, "@bob:example.com", "@bob:example.com"}},
			want:    []string{"Bob"},
		},
		{
			name:    "pills stand in for a client without m.mentions",
			trigger: Trigger{FormattedBody: "ask " + bobPill + " and " + carolPill},
			want:    []string{"Bob", "Carol"},
		},
		{
			name:    "m.mentions that names nobody means nobody",
			trigger: Trigger{Mentions: []string{}, FormattedBody: bobPill},
		},
		{
			name: "the replied-to sender added for notification is not a mention",
			trigger: Trigger{
				Mentions: []string{"@carol:example.com"},
				ReplyTo:  &matrix.Message{Sender: "@carol:example.com"},
			},
		},
		{
			name: "the replied-to sender pilled as well is",
			trigger: Trigger{
				Mentions:      []string{"@carol:example.com"},
				FormattedBody: carolPill + " is wrong",
				ReplyTo:       &matrix.Message{Sender: "@carol:example.com"},
			},
			want: []string{"Carol"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mentionsOf(context.Background(), tt.trigger, room)
			var names []string
			for _, mention := range got {
				names = append(names, mention.Name)
			}
			if len(names) != len(tt.want) {
				t.Fatalf("mentionsOf() = %v, want %v", names, tt.want)
			}
			for i := range names {
				if names[i] != tt.want[i] {
					t.Errorf("mentionsOf() = %v, want %v", names, tt.want)
				}
			}
		})
	}
}
