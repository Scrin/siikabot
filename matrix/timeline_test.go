package matrix

import (
	"testing"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

var mainTimeline = timelineKey{roomID: testRoomID}

// resetTimelines starts a test with nothing recorded
func resetTimelines(t *testing.T) {
	t.Helper()
	timelines.Lock()
	timelines.latest = make(map[timelineKey]latestMessage)
	timelines.Unlock()
}

// timelineEvent is an event as it arrives in a room's timeline through sync
func timelineEvent(evtType event.Type, eventID string, parsed any) *event.Event {
	return &event.Event{
		Type:    evtType,
		RoomID:  id.RoomID(testRoomID),
		ID:      id.EventID(eventID),
		Content: event.Content{Parsed: parsed},
		Mautrix: event.MautrixInfo{EventSource: event.SourceJoin | event.SourceTimeline},
	}
}

func textMessage(eventID string, rel *event.RelatesTo) *event.Event {
	return timelineEvent(event.EventMessage, eventID, &event.MessageEventContent{MsgType: event.MsgText, Body: "hi", RelatesTo: rel})
}

func encryptedMessage(eventID string, rel *event.RelatesTo) *event.Event {
	return timelineEvent(event.EventEncrypted, eventID, &event.EncryptedEventContent{RelatesTo: rel})
}

func inThread(rootID string) *event.RelatesTo {
	return (&event.RelatesTo{}).SetThread(id.EventID(rootID), "")
}

func TestTimelineRecordsTheLatestMessage(t *testing.T) {
	resetTimelines(t)

	trackTimeline(textMessage("$first", nil))
	trackTimeline(textMessage("$second", nil))

	if isLatest(mainTimeline, "$first") {
		t.Error("an earlier message is still the latest")
	}
	if !isLatest(mainTimeline, "$second") {
		t.Error("the last message is not the latest")
	}
}

func TestTimelineIgnoresWhatDoesNotPostAMessage(t *testing.T) {
	resetTimelines(t)
	trackTimeline(textMessage("$question", nil))

	member := timelineEvent(event.StateMember, "$join", &event.MemberEventContent{Membership: event.MembershipJoin})
	stateKey := "@carol:example.com"
	member.StateKey = &stateKey

	notPosting := map[string]*event.Event{
		"an edit":            textMessage("$edit", (&event.RelatesTo{}).SetReplace("$question")),
		"an encrypted edit":  encryptedMessage("$encrypted-edit", (&event.RelatesTo{}).SetReplace("$question")),
		"a reaction":         timelineEvent(event.EventReaction, "$reaction", &event.ReactionEventContent{RelatesTo: *(&event.RelatesTo{}).SetAnnotation("$question", "👍")}),
		"an encrypted vote":  encryptedMessage("$vote", &event.RelatesTo{Type: event.RelReference, EventID: "$poll"}),
		"a redaction":        timelineEvent(event.EventRedaction, "$redaction", &event.RedactionEventContent{Redacts: "$other"}),
		"a join":             member,
		"a typing indicator": {Type: event.EphemeralEventTyping, RoomID: id.RoomID(testRoomID), Mautrix: event.MautrixInfo{EventSource: event.SourceJoin | event.SourceEphemeral}},
	}
	for name, evt := range notPosting {
		trackTimeline(evt)
		if !isLatest(mainTimeline, "$question") {
			t.Errorf("%s counted as a message", name)
			resetTimelines(t)
			trackTimeline(textMessage("$question", nil))
		}
	}
}

// A message that isn't part of the timeline, such as one in the state section of a sync, doesn't
// count
func TestTimelineIgnoresEventsFromOutsideTheTimeline(t *testing.T) {
	resetTimelines(t)
	trackTimeline(textMessage("$question", nil))

	outside := textMessage("$state", nil)
	outside.Mautrix.EventSource = event.SourceJoin | event.SourceState
	trackTimeline(outside)

	if !isLatest(mainTimeline, "$question") {
		t.Error("an event from outside the timeline counted as a message")
	}
}

func TestTimelineCountsEveryKindOfMessage(t *testing.T) {
	messages := map[string]*event.Event{
		"a notice":         timelineEvent(event.EventMessage, "$notice", &event.MessageEventContent{MsgType: event.MsgNotice, Body: "hi"}),
		"an image":         timelineEvent(event.EventMessage, "$image", &event.MessageEventContent{MsgType: event.MsgImage, Body: "cat.jpg"}),
		"a sticker":        timelineEvent(event.EventSticker, "$sticker", &event.MessageEventContent{Body: "wave"}),
		"a reply":          textMessage("$reply", (&event.RelatesTo{}).SetReplyTo("$older")),
		"an encrypted one": encryptedMessage("$encrypted", nil),
	}
	for name, evt := range messages {
		resetTimelines(t)
		trackTimeline(textMessage("$question", nil))
		trackTimeline(evt)
		if isLatest(mainTimeline, "$question") {
			t.Errorf("%s didn't count as a message", name)
		}
	}
}

func TestTimelineTracksThreadsSeparately(t *testing.T) {
	resetTimelines(t)
	thread := timelineKey{roomID: testRoomID, threadRootID: "$root"}

	trackTimeline(textMessage("$main-question", nil))
	trackTimeline(textMessage("$thread-question", inThread("$root")))
	if !isLatest(mainTimeline, "$main-question") {
		t.Error("a thread message counted in the main timeline")
	}
	if !isLatest(thread, "$thread-question") {
		t.Error("a thread message wasn't recorded in its thread")
	}

	trackTimeline(textMessage("$main-chatter", nil))
	if !isLatest(thread, "$thread-question") {
		t.Error("a main timeline message counted in the thread")
	}

	// The thread relation of an encrypted message is in the clear
	trackTimeline(encryptedMessage("$thread-chatter", inThread("$root")))
	if isLatest(thread, "$thread-question") {
		t.Error("an encrypted thread message didn't count in its thread")
	}
}

func TestTimelineTracksRoomsSeparately(t *testing.T) {
	resetTimelines(t)

	trackTimeline(textMessage("$question", nil))
	elsewhere := textMessage("$elsewhere", nil)
	elsewhere.RoomID = "!other:example.com"
	trackTimeline(elsewhere)

	if !isLatest(mainTimeline, "$question") {
		t.Error("a message in another room counted")
	}
}

// The bot's own message is recorded when it is sent, and again when its copy comes back through
// sync. A message that the server placed before it, but that the bot only saw afterwards, arrives
// before that copy, so the bot's message ends up the latest one, as it is.
func TestTimelineSyncCopyOfAnOwnMessage(t *testing.T) {
	resetTimelines(t)

	trackTimeline(textMessage("$question", nil))
	recordLatest(mainTimeline, "$answer")
	trackTimeline(textMessage("$sent-meanwhile", nil))
	trackTimeline(timelineEvent(event.EventMessage, "$answer", &event.MessageEventContent{MsgType: event.MsgNotice, Body: "sunny"}))

	if !isLatest(mainTimeline, "$answer") {
		t.Error("the bot's own message is not the latest after its copy came back")
	}
}

func TestTimelineForgetsQuietTimelines(t *testing.T) {
	resetTimelines(t)
	quiet := timelineKey{roomID: "!quiet:example.com"}

	recordLatest(quiet, "$old")
	timelines.Lock()
	timelines.latest[quiet] = latestMessage{eventID: "$old", recordedAt: time.Now().Add(-timelineMemory - time.Minute)}
	timelines.Unlock()

	if isLatest(quiet, "$old") {
		t.Error("a message older than timelineMemory is still the latest")
	}

	recordLatest(mainTimeline, "$new")
	timelines.Lock()
	_, kept := timelines.latest[quiet]
	timelines.Unlock()
	if kept {
		t.Error("a quiet timeline was not forgotten")
	}
}
