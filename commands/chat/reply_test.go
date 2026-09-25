package chat

import (
	"context"
	"testing"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
)

const testRoomID = "!room:example.com"

var quoteTime = time.Date(2026, 9, 25, 13, 58, 0, 0, time.UTC)

// textFrom is a fetched text message
func textFrom(eventID, sender, body string) *matrix.Message {
	return &matrix.Message{RoomID: testRoomID, EventID: eventID, Sender: sender, Timestamp: quoteTime, MsgType: "m.text", Body: body}
}

func TestReferencedMessagesWithoutReferences(t *testing.T) {
	refs := referencedMessages(context.Background(), Trigger{RoomID: testRoomID, Body: "hi"}, testRoom("", alice, botSelf), true)

	if refs.replyTo != nil || refs.links != nil || refs.images != nil || refs.notes != nil || len(refs.attached) != 0 {
		t.Errorf("referencedMessages() = %#v, want nothing", refs)
	}
}

func TestReferencedMessagesQuoteTheRepliedToText(t *testing.T) {
	trigger := Trigger{
		RoomID:         testRoomID,
		Body:           "what do you think about this?",
		ReplyToEventID: "$carol",
		ReplyTo:        textFrom("$carol", "@carol:example.com", "pineapple belongs on pizza"),
	}

	refs := referencedMessages(context.Background(), trigger, testRoom("", alice, carol, botSelf), true)

	want := db.QuotedMessage{
		EventID: "$carol", Sender: "@carol:example.com", SenderName: "Carol",
		SentAt: quoteTime, Kind: db.QuoteText, Body: "pineapple belongs on pizza",
	}
	if refs.replyTo == nil || *refs.replyTo != want {
		t.Errorf("replyTo = %#v, want %#v", refs.replyTo, want)
	}
	if refs.images != nil || refs.notes != nil {
		t.Errorf("images = %v, notes = %v, want neither", refs.images, refs.notes)
	}
}

// A reference that can't be read is still worth mentioning: the model should know the message
// replies to something, rather than answer as if it stood alone. Nothing is quoted, since there
// is nothing to quote.
func TestReferencedMessagesWithAReplyThatCouldNotBeRead(t *testing.T) {
	tests := []struct {
		name    string
		replyTo *matrix.Message
	}{
		{"fetch failed, or the sender couldn't see it", nil},
		{"content could not be decrypted", &matrix.Message{RoomID: testRoomID, EventID: "$secret", Sender: "@carol:example.com"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trigger := Trigger{RoomID: testRoomID, Body: "and this?", ReplyToEventID: "$secret", ReplyTo: tt.replyTo}

			refs := referencedMessages(context.Background(), trigger, testRoom("", alice, carol, botSelf), true)

			if refs.replyTo != nil || refs.images != nil {
				t.Errorf("replyTo = %v, images = %v, want neither", refs.replyTo, refs.images)
			}
			if len(refs.notes) != 1 || refs.notes[0] != unreadableReplyNote {
				t.Errorf("notes = %v, want the unreadable reply note", refs.notes)
			}
		})
	}
}

// The first message of a thread started on a stored turn has that turn in its context already
func TestReferencedMessagesWithoutTheReply(t *testing.T) {
	trigger := Trigger{
		RoomID:         testRoomID,
		Body:           "are you sure?",
		ThreadRootID:   "$answer",
		ReplyToEventID: "$answer",
		ReplyTo:        textFrom("$answer", testBotUserID, "it's sunny"),
	}

	refs := referencedMessages(context.Background(), trigger, testRoom("", alice, botSelf), false)
	if refs.replyTo != nil || refs.notes != nil {
		t.Errorf("referencedMessages() = %#v, want the root left out", refs)
	}
}

func TestReferencedMessagesQuoteLinkedMessages(t *testing.T) {
	trigger := Trigger{
		RoomID: testRoomID,
		Body:   "compare these two",
		Links: []Link{
			{EventID: "$carol", Message: textFrom("$carol", "@carol:example.com", "tabs")},
			{EventID: "$bob", Message: textFrom("$bob", "@bob:example.com", "spaces")},
		},
	}

	refs := referencedMessages(context.Background(), trigger, testRoom("", alice, bob, carol, botSelf), true)

	if len(refs.links) != 2 || refs.links[0].Body != "tabs" || refs.links[1].Body != "spaces" || refs.links[1].SenderName != "Bob" {
		t.Errorf("links = %#v, want Carol's and Bob's messages, in order", refs.links)
	}
	if refs.replyTo != nil || refs.notes != nil {
		t.Errorf("replyTo = %v, notes = %v, want neither", refs.replyTo, refs.notes)
	}
}

// However many linked messages can't be read, the model is told once
func TestReferencedMessagesWithLinksThatCouldNotBeRead(t *testing.T) {
	trigger := Trigger{
		RoomID: testRoomID,
		Body:   "what about these?",
		Links: []Link{
			{EventID: "$hidden"},
			{EventID: "$carol", Message: textFrom("$carol", "@carol:example.com", "tabs")},
			{EventID: "$secret", Message: &matrix.Message{RoomID: testRoomID, EventID: "$secret", Sender: "@bob:example.com"}},
		},
	}

	refs := referencedMessages(context.Background(), trigger, testRoom("", alice, bob, carol, botSelf), true)

	if len(refs.links) != 1 || refs.links[0].EventID != "$carol" {
		t.Errorf("links = %#v, want only Carol's message", refs.links)
	}
	if len(refs.notes) != 1 || refs.notes[0] != unreadableLinkNote {
		t.Errorf("notes = %v, want the unreadable link note once", refs.notes)
	}
}

// A quoted image keeps its caption. The image itself is attached only when it can be downloaded,
// which a message without its media details can't be.
func TestReferencedMessagesQuoteAnImageWithItsCaption(t *testing.T) {
	image := &matrix.Message{RoomID: testRoomID, EventID: "$image", Sender: "@carol:example.com", Timestamp: quoteTime, MsgType: "m.image", Body: "cat.jpg"}
	trigger := Trigger{RoomID: testRoomID, Body: "what is this?", ReplyToEventID: "$image", ReplyTo: image}

	refs := referencedMessages(context.Background(), trigger, testRoom("", alice, carol, botSelf), true)

	if refs.replyTo == nil || refs.replyTo.Kind != db.QuoteImage || refs.replyTo.Body != "" {
		t.Errorf("replyTo = %#v, want an image without a caption", refs.replyTo)
	}
	if refs.images != nil || refs.attached["$image"] {
		t.Errorf("images = %v, attached = %v, want nothing attached", refs.images, refs.attached)
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
