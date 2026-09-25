package matrix

import (
	"encoding/json"
	"strings"
	"testing"

	"maunium.net/go/mautrix/event"
)

func TestTargetInTheMainTimelineHasNoRelation(t *testing.T) {
	if rel := (Target{InReplyTo: "$question"}).relatesTo(); rel != nil {
		t.Errorf("relatesTo() = %#v, want none for the main timeline", rel)
	}
}

// An answer in a thread is a plain thread message: its reply relation is only the fallback shown
// by clients without thread support, pointing at the message it follows up on
func TestTargetInAThread(t *testing.T) {
	rel := Target{ThreadRootID: "$root", InReplyTo: "$question"}.relatesTo()

	if rel == nil || rel.Type != event.RelThread || rel.EventID != "$root" {
		t.Fatalf("relatesTo() = %#v, want a thread relation to the root", rel)
	}
	if !rel.IsFallingBack || rel.GetReplyTo() != "$question" {
		t.Errorf("relatesTo() = %#v, want a fallback reply to the question", rel)
	}
	if rel.GetNonFallbackReplyTo() != "" {
		t.Error("the fallback reads as an explicit reply")
	}
}

func TestTargetedMessageCarriesTheRelation(t *testing.T) {
	msg := formattedMessage("m.notice", "<b>sunny</b>", nil, Target{ThreadRootID: "$root", InReplyTo: "$question"}.relatesTo())

	encoded, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"m.relates_to":{`, `"rel_type":"m.thread"`, `"is_falling_back":true`, `"body":"sunny"`} {
		if !strings.Contains(string(encoded), want) {
			t.Errorf("encoded message %s lacks %s", encoded, want)
		}
	}

	plain, _ := json.Marshal(formattedMessage("m.notice", "sunny", nil, nil))
	if strings.Contains(string(plain), "m.relates_to") {
		t.Errorf("a main timeline message carries a relation: %s", plain)
	}
}
