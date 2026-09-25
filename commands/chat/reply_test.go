package chat

import (
	"context"
	"strings"
	"testing"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/matrix"
)

const testRoomID = "!room:example.com"

func TestRelatedMessageWithoutAReference(t *testing.T) {
	var messages []aigateway.Message
	got := processRelatedMessage(context.Background(), Trigger{RoomID: testRoomID, Body: "hi"}, &messages)

	if got != "" || len(messages) != 0 {
		t.Errorf("processRelatedMessage() = %q with %d messages, want nothing added", got, len(messages))
	}
}

func TestRelatedMessageIncludesTheReferencedText(t *testing.T) {
	trigger := Trigger{
		RoomID:         testRoomID,
		Body:           "what do you think about this?",
		ReplyToEventID: "$carol",
		ReplyTo: &matrix.Message{
			RoomID:  testRoomID,
			EventID: "$carol",
			Sender:  "@carol:example.com",
			MsgType: "m.text",
			Body:    "pineapple belongs on pizza",
		},
	}

	var messages []aigateway.Message
	processRelatedMessage(context.Background(), trigger, &messages)

	if len(messages) != 1 {
		t.Fatalf("got %d messages, want the reply context alone", len(messages))
	}
	if content, _ := messages[0].Content.(string); !strings.Contains(content, "pineapple belongs on pizza") {
		t.Errorf("reply context = %q, want the referenced message in it", content)
	}
}

// A reference that can't be read is still worth mentioning: the model should know the message
// replies to something, rather than answer as if it stood alone
func TestRelatedMessageThatCouldNotBeRead(t *testing.T) {
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

			var messages []aigateway.Message
			got := processRelatedMessage(context.Background(), trigger, &messages)

			if got != "" {
				t.Errorf("processRelatedMessage() = %q, want no image", got)
			}
			if len(messages) != 1 {
				t.Fatalf("got %d messages, want one note", len(messages))
			}
			if content, _ := messages[0].Content.(string); !strings.Contains(content, "couldn't retrieve") {
				t.Errorf("note = %q, want it to say the message couldn't be retrieved", content)
			}
		})
	}
}
