package matrix

import (
	"encoding/json"
	"strings"
	"testing"
)

// Bob asks, nobody else writes, and the answer appears right below the question
func TestTargetAnswersPlainlyWhileTheQuestionIsLatest(t *testing.T) {
	resetTimelines(t)
	trackTimeline(textMessage("$question", nil))

	if rel := (Target{InReplyTo: "$question"}).relatesTo(testRoomID); rel != nil {
		t.Errorf("relatesTo() = %#v, want a plain message", rel)
	}
}

// Alice writes before the answer is out, so the answer shows which question it belongs to
func TestTargetRepliesOnceTheConversationMovedOn(t *testing.T) {
	resetTimelines(t)
	trackTimeline(textMessage("$question", nil))
	trackTimeline(textMessage("$chatter", nil))

	rel := Target{InReplyTo: "$question"}.relatesTo(testRoomID)
	if rel.GetNonFallbackReplyTo() != "$question" {
		t.Fatalf("relatesTo() = %#v, want a reply to the question", rel)
	}
	if rel.Type != "" {
		t.Errorf("relatesTo() = %#v, want a reply in the main timeline", rel)
	}
}

// The bot's own messages count too, like a tool's notice posted during the turn
func TestTargetRepliesAfterTheBotsOwnMessage(t *testing.T) {
	resetTimelines(t)
	trackTimeline(textMessage("$question", nil))
	recordLatest(mainTimeline, "$reminder-notice")

	if rel := (Target{InReplyTo: "$question"}).relatesTo(testRoomID); rel.GetNonFallbackReplyTo() != "$question" {
		t.Errorf("relatesTo() = %#v, want a reply to the question", rel)
	}
}

// A question the bot has no record of, or no longer remembers, may have been followed by anything
func TestTargetRepliesWhenTheQuestionIsNotRecorded(t *testing.T) {
	resetTimelines(t)

	if rel := (Target{InReplyTo: "$question"}).relatesTo(testRoomID); rel.GetNonFallbackReplyTo() != "$question" {
		t.Errorf("relatesTo() = %#v, want a reply to the question", rel)
	}
}

// Two questions queued back to back: each comes between the other one and its answer
func TestTargetTwoQueuedQuestionsBothGetReplies(t *testing.T) {
	resetTimelines(t)
	trackTimeline(textMessage("$bob", nil))
	trackTimeline(textMessage("$alice", nil))

	if rel := (Target{InReplyTo: "$bob"}).relatesTo(testRoomID); rel.GetNonFallbackReplyTo() != "$bob" {
		t.Errorf("the first answer: relatesTo() = %#v, want a reply to the first question", rel)
	}
	recordLatest(mainTimeline, "$answer-to-bob")

	if rel := (Target{InReplyTo: "$alice"}).relatesTo(testRoomID); rel.GetNonFallbackReplyTo() != "$alice" {
		t.Errorf("the second answer: relatesTo() = %#v, want a reply to the second question", rel)
	}
}

// In a thread, the answer is a plain thread message: its reply relation is only the fallback shown
// by clients without thread support, pointing at the latest message in the thread
func TestTargetInAThreadWhileTheQuestionIsLatest(t *testing.T) {
	resetTimelines(t)
	trackTimeline(textMessage("$question", inThread("$root")))
	// The main timeline has nothing to do with the thread
	trackTimeline(textMessage("$main-chatter", nil))

	rel := Target{ThreadRootID: "$root", InReplyTo: "$question"}.relatesTo(testRoomID)
	if rel.GetThreadParent() != "$root" {
		t.Fatalf("relatesTo() = %#v, want a thread relation to the root", rel)
	}
	if !rel.IsFallingBack || rel.GetReplyTo() != "$question" {
		t.Errorf("relatesTo() = %#v, want a fallback reply to the question", rel)
	}
	if rel.GetNonFallbackReplyTo() != "" {
		t.Error("the fallback reads as an explicit reply")
	}
}

func TestTargetInAThreadOnceItMovedOn(t *testing.T) {
	resetTimelines(t)
	trackTimeline(textMessage("$question", inThread("$root")))
	trackTimeline(textMessage("$thread-chatter", inThread("$root")))

	rel := Target{ThreadRootID: "$root", InReplyTo: "$question"}.relatesTo(testRoomID)
	if rel.GetThreadParent() != "$root" {
		t.Fatalf("relatesTo() = %#v, want a thread relation to the root", rel)
	}
	if rel.GetNonFallbackReplyTo() != "$question" {
		t.Errorf("relatesTo() = %#v, want a visible reply to the question", rel)
	}
}

func TestTargetedMessageCarriesTheRelation(t *testing.T) {
	resetTimelines(t)
	trackTimeline(textMessage("$question", inThread("$root")))
	trackTimeline(textMessage("$main-question", nil))
	trackTimeline(textMessage("$older", nil))

	encode := func(target *Target) string {
		t.Helper()
		evt := outboundEvent{RoomID: testRoomID, Content: formattedMessage("m.notice", "<b>sunny</b>", nil), target: target}
		encoded, err := json.Marshal(evt.content())
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}

	inThread := encode(&Target{ThreadRootID: "$root", InReplyTo: "$question"})
	for _, want := range []string{`"rel_type":"m.thread"`, `"event_id":"$root"`, `"is_falling_back":true`, `"body":"sunny"`} {
		if !strings.Contains(inThread, want) {
			t.Errorf("encoded thread message %s lacks %s", inThread, want)
		}
	}

	reply := encode(&Target{InReplyTo: "$main-question"})
	if !strings.Contains(reply, `"m.relates_to":{"m.in_reply_to":{"event_id":"$main-question"}}`) {
		t.Errorf("encoded reply %s lacks a plain reply relation", reply)
	}

	for name, target := range map[string]*Target{"an untargeted message": nil, "a plain answer": {InReplyTo: "$older"}} {
		if encoded := encode(target); strings.Contains(encoded, "m.relates_to") {
			t.Errorf("%s carries a relation: %s", name, encoded)
		}
	}
}

// The bot's own message is recorded in the timeline it was sent to
func TestTargetTimeline(t *testing.T) {
	if got := (*Target)(nil).timeline(testRoomID); got != mainTimeline {
		t.Errorf("an untargeted message goes to %v, want the main timeline", got)
	}
	if got := (&Target{ThreadRootID: "$root", InReplyTo: "$question"}).timeline(testRoomID); got.threadRootID != "$root" || got.roomID != testRoomID {
		t.Errorf("a thread message goes to %v, want the thread", got)
	}
}

func TestTargetCancelled(t *testing.T) {
	if (*Target)(nil).cancelled() {
		t.Error("an untargeted message is cancelled")
	}
	if (&Target{InReplyTo: "$question"}).cancelled() {
		t.Error("a message without a cancel check is cancelled")
	}
	if !(&Target{Cancelled: func() bool { return true }}).cancelled() {
		t.Error("a cancelled message is still wanted")
	}
}
