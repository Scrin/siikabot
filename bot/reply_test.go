package bot

import (
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

const (
	testThreadRoot = id.EventID("$root")
	testLatest     = id.EventID("$latest")
	testRepliedTo  = id.EventID("$replied")
)

// threadMessage builds the relation a thread message carries: the thread root, plus a reply
// relation that is either a fallback or an explicit reply
func threadMessage(replyTo id.EventID, isFallingBack bool) *event.RelatesTo {
	rel := &event.RelatesTo{Type: event.RelThread, EventID: testThreadRoot, IsFallingBack: isFallingBack}
	if replyTo != "" {
		rel.InReplyTo = &event.InReplyTo{EventID: replyTo}
	}
	return rel
}

func TestReplyTarget(t *testing.T) {
	tests := []struct {
		name string
		rel  *event.RelatesTo
		// firstReply is what the server says about whether the message is the thread's first reply
		firstReply bool
		want       id.EventID
		// wantAsked is whether the server had to be asked at all
		wantAsked bool
	}{
		{
			name: "no relation",
			rel:  nil,
		},
		{
			name: "plain reply",
			rel:  &event.RelatesTo{InReplyTo: &event.InReplyTo{EventID: testRepliedTo}},
			want: testRepliedTo,
		},
		{
			name: "thread fallback yields no reply target",
			rel:  threadMessage(testLatest, true),
		},
		{
			name: "explicit reply inside a thread yields its target",
			rel:  threadMessage(testRepliedTo, false),
			want: testRepliedTo,
		},
		{
			name: "explicit reply to the root inside a thread",
			rel:  threadMessage(testThreadRoot, false),
			want: testThreadRoot,
		},
		{
			name:       "first message of a thread yields the root",
			rel:        threadMessage(testThreadRoot, true),
			firstReply: true,
			want:       testThreadRoot,
			wantAsked:  true,
		},
		{
			name:       "fallback to the root that is not the thread's first reply",
			rel:        threadMessage(testThreadRoot, true),
			firstReply: false,
			wantAsked:  true,
		},
		{
			name:       "thread message without a fallback is checked too",
			rel:        threadMessage("", false),
			firstReply: true,
			want:       testThreadRoot,
			wantAsked:  true,
		},
		{
			name: "edit",
			rel:  &event.RelatesTo{Type: event.RelReplace, EventID: "$original"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asked := false
			got := replyTarget(tt.rel, func(root id.EventID) bool {
				asked = true
				if root != testThreadRoot {
					t.Errorf("asked about root %q, want %q", root, testThreadRoot)
				}
				return tt.firstReply
			})

			if got != tt.want {
				t.Errorf("replyTarget() = %q, want %q", got, tt.want)
			}
			if asked != tt.wantAsked {
				t.Errorf("server asked = %v, want %v", asked, tt.wantAsked)
			}
		})
	}
}

func TestOwnText(t *testing.T) {
	reply := &event.RelatesTo{InReplyTo: &event.InReplyTo{EventID: testRepliedTo}}

	tests := []struct {
		name          string
		body          string
		formatted     string
		rel           *event.RelatesTo
		wantBody      string
		wantFormatted string
	}{
		{
			name:          "reply loses the quote of the replied-to message",
			body:          "> <@carol:example.com> an earlier message\n\nwhat do you think?",
			formatted:     "<mx-reply><blockquote>an earlier message</blockquote></mx-reply>what do you <i>think</i>?",
			rel:           reply,
			wantBody:      "what do you think?",
			wantFormatted: "what do you <i>think</i>?",
		},
		{
			name:          "reply without a fallback is unchanged",
			body:          "what do you think?",
			rel:           reply,
			wantBody:      "what do you think?",
			wantFormatted: "",
		},
		{
			name:          "message that is not a reply keeps a quote of its own",
			body:          "> a line I am quoting myself\n\nthoughts?",
			rel:           nil,
			wantBody:      "> a line I am quoting myself\n\nthoughts?",
			wantFormatted: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBody, gotFormatted := ownText(tt.body, tt.formatted, tt.rel)
			if gotBody != tt.wantBody {
				t.Errorf("body = %q, want %q", gotBody, tt.wantBody)
			}
			if gotFormatted != tt.wantFormatted {
				t.Errorf("formatted = %q, want %q", gotFormatted, tt.wantFormatted)
			}
		})
	}
}
