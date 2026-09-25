package bot

import (
	"slices"
	"testing"
)

const (
	testBotUserID      = "@siikabot:example.com"
	testBotDisplayName = "SiikaBot"
)

func TestMentionsBotExplicitly(t *testing.T) {
	tests := []struct {
		name    string
		content map[string]any
		want    bool
	}{
		{
			name:    "no mentions field",
			content: map[string]any{"body": "hello"},
		},
		{
			name:    "bot in user_ids",
			content: map[string]any{"m.mentions": map[string]any{"user_ids": []any{testBotUserID}}},
			want:    true,
		},
		{
			name:    "another user in user_ids",
			content: map[string]any{"m.mentions": map[string]any{"user_ids": []any{"@someone:example.com"}}},
		},
		{
			name:    "bot alongside another user",
			content: map[string]any{"m.mentions": map[string]any{"user_ids": []any{"@someone:example.com", testBotUserID}}},
			want:    true,
		},
		{
			name:    "room mention only",
			content: map[string]any{"m.mentions": map[string]any{"room": true}},
		},
		{
			name:    "room mention with the bot named too",
			content: map[string]any{"m.mentions": map[string]any{"room": true, "user_ids": []any{testBotUserID}}},
			want:    true,
		},
		{
			name:    "empty mentions",
			content: map[string]any{"m.mentions": map[string]any{}},
		},
		{
			name:    "unexpected user_ids type",
			content: map[string]any{"m.mentions": map[string]any{"user_ids": testBotUserID}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mentionsBotExplicitly(tt.content, testBotUserID); got != tt.want {
				t.Errorf("mentionsBotExplicitly() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStripBotNamePrefix(t *testing.T) {
	pill := `<a href="https://matrix.to/#/` + testBotUserID + `">SiikaBot</a>`

	tests := []struct {
		name        string
		plain       string
		formatted   string
		displayName string
		want        string
		wantOK      bool
	}{
		{
			name:   "display name prefix",
			plain:  "SiikaBot hello there",
			want:   "hello there",
			wantOK: true,
		},
		{
			name:   "display name with colon",
			plain:  "siikabot: hello there",
			want:   "hello there",
			wantOK: true,
		},
		{
			name:   "display name with comma",
			plain:  "SiikaBot, hello there",
			want:   "hello there",
			wantOK: true,
		},
		{
			name:   "display name with leading at sign",
			plain:  "@SiikaBot hello there",
			want:   "hello there",
			wantOK: true,
		},
		{
			name:   "leading whitespace",
			plain:  "   SiikaBot   hello there",
			want:   "hello there",
			wantOK: true,
		},
		{
			name:   "full user id prefix",
			plain:  "@siikabot:example.com hello there",
			want:   "hello there",
			wantOK: true,
		},
		{
			name:   "name only",
			plain:  "SiikaBot",
			want:   "",
			wantOK: true,
		},
		{
			name:   "question mark right after the name",
			plain:  "SiikaBot? are you there",
			want:   "? are you there",
			wantOK: true,
		},
		{
			name:  "name in the middle of a message",
			plain: "I wonder if siikabot knows the answer",
		},
		{
			name:  "name at the end of a message",
			plain: "that sounds like a job for siikabot",
		},
		{
			name:  "longer word starting with the name",
			plain: "siikabottle of water",
		},
		{
			name:  "hostname starting with the name",
			plain: "siikabot.example.com is down again",
		},
		{
			name:      "leading pill",
			plain:     "SiikaBot: hello there",
			formatted: pill + ": hello there",
			want:      "hello there",
			wantOK:    true,
		},
		{
			name:      "leading pill whose text differs from the display name",
			plain:     "Siika the Bot: hello there",
			formatted: `<a href="https://matrix.to/#/` + testBotUserID + `">Siika the Bot</a>: hello there`,
			want:      "hello there",
			wantOK:    true,
		},
		{
			name:      "pill in the middle of a message",
			plain:     "I wonder if SiikaBot knows the answer",
			formatted: "I wonder if " + pill + " knows the answer",
		},
		{
			name:      "pill for another user",
			plain:     "Someone: hello there",
			formatted: `<a href="https://matrix.to/#/@someone:example.com">Someone</a>: hello there`,
		},
		{
			name:      "pill behind a rich reply quote",
			plain:     "> <@someone:example.com> an earlier message\n\nSiikaBot: hello there",
			formatted: "<mx-reply><blockquote>an earlier message</blockquote></mx-reply>" + pill + ": hello there",
			want:      "hello there",
			wantOK:    true,
		},
		{
			name:  "name prefix behind a rich reply quote",
			plain: "> <@someone:example.com> an earlier message\n\nSiikaBot hello there",
			// Some clients send the fallback without any formatted body at all
			want:   "hello there",
			wantOK: true,
		},
		{
			name:        "display name falls back to the user id",
			plain:       "@siikabot:example.com hello there",
			displayName: testBotUserID,
			want:        "hello there",
			wantOK:      true,
		},
		{
			name:        "no display name known",
			plain:       "SiikaBot hello there",
			displayName: "-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			displayName := testBotDisplayName
			switch tt.displayName {
			case "":
			case "-":
				displayName = ""
			default:
				displayName = tt.displayName
			}

			got, gotOK := stripBotNamePrefix(tt.plain, tt.formatted, testBotUserID, displayName)
			if gotOK != tt.wantOK {
				t.Fatalf("stripBotNamePrefix() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotOK && got != tt.want {
				t.Errorf("stripBotNamePrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}

// The localpart of the user id is deliberately not accepted as a name on its own, since it is
// often something as generic as "bot", which would make far too many messages look like ours
func TestStripBotNamePrefixIgnoresLocalpart(t *testing.T) {
	if got, ok := stripBotNamePrefix("bot, please fix the build", "", "@bot:example.com", testBotDisplayName); ok {
		t.Errorf("stripBotNamePrefix() = %q, ok = true, want ok = false", got)
	}
}

func TestMentionedUserIDs(t *testing.T) {
	content := map[string]any{"m.mentions": map[string]any{
		"user_ids": []any{"@alice:example.com", 42, testBotUserID},
	}}

	got := mentionedUserIDs(content)
	want := []string{"@alice:example.com", testBotUserID}
	if !slices.Equal(got, want) {
		t.Errorf("mentionedUserIDs() = %v, want %v", got, want)
	}

	if got := mentionedUserIDs(map[string]any{"body": "hello"}); got != nil {
		t.Errorf("mentionedUserIDs() without m.mentions = %v, want nil", got)
	}

	// m.mentions that names nobody says the message mentions nobody, which is not the same as a
	// client that doesn't send m.mentions at all
	if got := mentionedUserIDs(map[string]any{"m.mentions": map[string]any{}}); got == nil || len(got) != 0 {
		t.Errorf("mentionedUserIDs() with an empty m.mentions = %#v, want an empty list", got)
	}
}

// The bot can have a display name of its own in a room, and a message may address it by either
func TestStripBotNamePrefixAcceptsEveryBotName(t *testing.T) {
	for _, msg := range []string{"SiikaBot hello", "Siika: hello"} {
		got, ok := stripBotNamePrefix(msg, "", testBotUserID, testBotDisplayName, "Siika")
		if !ok || got != "hello" {
			t.Errorf("stripBotNamePrefix(%q) = %q, %v, want \"hello\", true", msg, got, ok)
		}
	}
}
