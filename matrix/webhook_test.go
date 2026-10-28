package matrix

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Scrin/siikabot/config"
	"maunium.net/go/mautrix/event"
)

var hermes = WebhookOrigin{ID: "abc123", Name: "hermes"}

func TestWebhookMessageLayout(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		body      string
		formatted string
	}{
		{
			name:      "a single line follows the name",
			text:      "hello there",
			body:      "[hermes]: hello there",
			formatted: "<p>[hermes]: hello there</p>",
		},
		{
			name:      "a single line keeps its formatting",
			text:      "Backup **done** in 4 min",
			body:      "[hermes]: Backup **done** in 4 min",
			formatted: "<p>[hermes]: Backup <strong>done</strong> in 4 min</p>",
		},
		{
			name:      "more lines start under the name, without a paragraph's gap",
			text:      "line one\nline two",
			body:      "[hermes]\nline one\nline two",
			formatted: "<p>[hermes]<br>\nline one<br>\nline two</p>",
		},
		{
			name:      "a heading starts under the name",
			text:      "# Disk full\nhost1 at 98%",
			body:      "[hermes]\n# Disk full\nhost1 at 98%",
			formatted: "<p>[hermes]</p>\n<h1>Disk full</h1>\n\n<p>host1 at 98%</p>",
		},
		{
			name:      "a single-line heading starts under the name too",
			text:      "# Disk full",
			body:      "[hermes]\n# Disk full",
			formatted: "<p>[hermes]</p>\n<h1>Disk full</h1>",
		},
		{
			name:      "so does a single list item",
			text:      "- one thing",
			body:      "[hermes]\n- one thing",
			formatted: "<p>[hermes]</p>\n<ul>\n<li>one thing</li>\n</ul>",
		},
		{
			name:      "and a code block",
			text:      "```\nmake build\n```",
			body:      "[hermes]\n```\nmake build\n```",
			formatted: "<p>[hermes]</p>\n<pre><code>make build\n</code></pre>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := NewWebhookMessage(hermes, tt.text).content
			if msg.Body != tt.body {
				t.Errorf("body = %q, want %q", msg.Body, tt.body)
			}
			if msg.FormattedBody != tt.formatted {
				t.Errorf("formatted body = %q, want %q", msg.FormattedBody, tt.formatted)
			}
			if msg.MsgType != "m.text" || msg.Format != "org.matrix.custom.html" {
				t.Errorf("sent as %q in %q, want an html-formatted m.text", msg.MsgType, msg.Format)
			}
		})
	}
}

// Behind "[name]: ", these would be Markdown link definitions, which render as nothing
func TestWebhookMessageThatLooksLikeALinkDefinitionComesOutWhole(t *testing.T) {
	for _, text := range []string{"hello", "https://example.com/status", `deploy "ok"`} {
		msg := NewWebhookMessage(hermes, text).content
		if want := "[hermes]: " + text; msg.Body != want {
			t.Errorf("body = %q, want %q", msg.Body, want)
		}
		if !strings.HasPrefix(msg.FormattedBody, "<p>[hermes]: ") || strings.TrimSpace(stripFormatting(msg.FormattedBody)) != msg.Body {
			t.Errorf("%q is formatted as %q, want it after the name", text, msg.FormattedBody)
		}
	}
}

func TestWebhookMessageEscapesTheName(t *testing.T) {
	msg := NewWebhookMessage(WebhookOrigin{ID: "abc123", Name: "<b>CI</b> & co"}, "green").content
	if msg.Body != "[<b>CI</b> & co]: green" {
		t.Errorf("body = %q, want the name as it is", msg.Body)
	}
	if msg.FormattedBody != "<p>[&lt;b&gt;CI&lt;/b&gt; &amp; co]: green</p>" {
		t.Errorf("formatted body = %q, want the name escaped", msg.FormattedBody)
	}
}

func TestWebhookMarkdownReadsEverydayTextAsWritten(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"line breaks are kept", "first\nsecond", "<p>first<br>\nsecond</p>\n"},
		{"dollar signs aren't math", "costs $5 and $10 today", "<p>costs $5 and $10 today</p>\n"},
		{"a line opening with a colon isn't a definition", "Status\n: ok", "<p>Status<br>\n: ok</p>\n"},
		{"dashes, fractions and quotes stay as written", `run --force at 1/2 load, "ok"`, "<p>run --force at 1/2 load, &quot;ok&quot;</p>\n"},
		{"list items don't end in a line break", "- a\n- b", "<ul>\n<li>a</li>\n<li>b</li>\n</ul>\n"},
		{"inline HTML is text", "a <b>bold</b> move", "<p>a &lt;b&gt;bold&lt;/b&gt; move</p>\n"},
		{"an HTML block is text", "<div>\nhi\n</div>", "<p>&lt;div&gt;<br>\nhi<br>\n&lt;/div&gt;</p>\n"},
		{"a script is text", "<script>alert(1)</script>", "<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>\n"},
		{"URLs are links", "see https://example.com", `<p>see <a href="https://example.com">https://example.com</a></p>` + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := webhookMarkdownToHTML(tt.text); got != tt.want {
				t.Errorf("webhookMarkdownToHTML(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

// The webhooks' Markdown settings are their own: the chat's replies render as they did
func TestChatMarkdownIsUntouchedByWebhookSettings(t *testing.T) {
	if got := markdownToHTML("first\nsecond", nil); strings.Contains(got, "<br>") {
		t.Errorf("chat markdown = %q, want the line break left soft", got)
	}
	if got := markdownToHTML("a <b>bold</b> move", nil); !strings.Contains(got, "<b>bold</b>") {
		t.Errorf("chat markdown = %q, want its HTML kept", got)
	}
}

func TestWebhookMessageNamesItsWebhookInItsContent(t *testing.T) {
	msg := NewWebhookMessage(hermes, "hello")
	encoded, err := json.Marshal(msg.content)
	if err != nil {
		t.Fatal(err)
	}
	var content map[string]any
	if err := json.Unmarshal(encoded, &content); err != nil {
		t.Fatal(err)
	}
	origin, _ := content[webhookOriginKey].(map[string]any)
	if origin["id"] != "abc123" || origin["name"] != "hermes" {
		t.Errorf("content = %s, want the webhook under %s", encoded, webhookOriginKey)
	}
	if msg.Size() != len(encoded) {
		t.Errorf("Size() = %d, want the %d bytes of the content", msg.Size(), len(encoded))
	}
}

func TestOwnMessagesCarryNoWebhookOrigin(t *testing.T) {
	encoded, err := json.Marshal(simpleMessage{MsgType: "m.text", Body: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), webhookOriginKey) {
		t.Errorf("content = %s, want no webhook origin on the bot's own message", encoded)
	}
}

func TestRelayedByTrustsOnlyTheBot(t *testing.T) {
	saved := config.UserID
	config.UserID = "@siikabot:example.com"
	defer func() { config.UserID = saved }()

	relayed := map[string]any{
		"msgtype":        "m.text",
		"body":           "[hermes]: hello",
		webhookOriginKey: map[string]any{"id": "abc123", "name": "hermes"},
	}
	fromBot := func(content map[string]any) *Message {
		evt := messageEvent(content)
		evt.Sender = "@siikabot:example.com"
		return messageFromEvent(testRoomID, evt)
	}

	if got := fromBot(relayed).RelayedBy(); got != "hermes" {
		t.Errorf("the bot's relayed message RelayedBy() = %q, want hermes", got)
	}
	if got := fromBot(map[string]any{"msgtype": "m.text", "body": "my own words"}).RelayedBy(); got != "" {
		t.Errorf("the bot's own message RelayedBy() = %q, want none", got)
	}
	if got := messageFromEvent(testRoomID, messageEvent(relayed)).RelayedBy(); got != "" {
		t.Errorf("someone else's message with a forged mark RelayedBy() = %q, want none", got)
	}
	notAMessage := &event.Event{ID: "$state", Sender: "@siikabot:example.com", Type: event.StateTopic}
	if got := messageFromEvent(testRoomID, notAMessage).RelayedBy(); got != "" {
		t.Errorf("an event that isn't a message RelayedBy() = %q, want none", got)
	}
}
