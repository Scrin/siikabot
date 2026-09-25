package matrix

import (
	"strings"
	"testing"
)

func TestPillUserIDs(t *testing.T) {
	formatted := `hey <a href="https://matrix.to/#/@bob:example.com">Bob</a> and ` +
		`<a href="https://matrix.to/#/%40carol%3Aexample.com">Carol</a>, see ` +
		`<a href="https://matrix.to/#/!room:example.com/$event">this</a>`

	got := PillUserIDs(formatted)
	if len(got) != 2 || got[0] != "@bob:example.com" || got[1] != "@carol:example.com" {
		t.Errorf("PillUserIDs() = %v, want Bob and Carol only", got)
	}
}

var pillNames = map[string]string{
	"@bob:example.com":         "Bob",
	"@carol:example.org:8448":  "Carol",
	"@dan_the-man:example.com": "Dan",
}

const bobPill = `<a href="https://matrix.to/#/@bob:example.com">Bob</a>`

func TestInsertPills(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     string
	}{
		{"in a sentence", "ask @bob:example.com about it", "ask " + bobPill + " about it"},
		{"with punctuation after", "thanks, @bob:example.com.", "thanks, " + bobPill + "."},
		{"addressed", "@bob:example.com: it's sunny", bobPill + ": it"},
		{"in parentheses", "(@bob:example.com)", "(" + bobPill + ")"},
		{"possessive", "@bob:example.com's idea", bobPill + "&rsquo;s idea"},
		{"in bold", "**@bob:example.com**", "<strong>" + bobPill + "</strong>"},
		{"in a list", "- @bob:example.com\n- someone else", "<li>" + bobPill + "</li>"},
		{"two of them", "@bob:example.com and @carol:example.org:8448",
			bobPill + ` and <a href="https://matrix.to/#/@carol:example.org:8448">Carol</a>`},
		{"with every localpart character", "hi @dan_the-man:example.com", `hi <a href="https://matrix.to/#/@dan_the-man:example.com">Dan</a>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := markdownToHTML(tt.markdown, pillNames); !strings.Contains(got, tt.want) {
				t.Errorf("markdownToHTML(%q) = %q, want it to contain %q", tt.markdown, got, tt.want)
			}
		})
	}
}

// What isn't plainly someone's user ID in the text stays as it was written
func TestInsertPillsLeavesTheRestAlone(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
	}{
		{"someone who isn't a member", "ask @dave:example.com"},
		{"inline code", "run `invite @bob:example.com`"},
		{"a code block", "```\ninvite @bob:example.com\n```"},
		{"a link", "[his profile](https://matrix.to/#/@bob:example.com)"},
		{"a link's text", "[@bob:example.com](https://example.com)"},
		{"a bare link", "https://matrix.to/#/@bob:example.com"},
		{"an email-like address", "mail bob@bob:example.com"},
		{"a longer token", "@bob:example.com_old"},
		{"a longer server name", "@bob:example.com.au"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			with, without := markdownToHTML(tt.markdown, pillNames), markdownToHTML(tt.markdown, nil)
			if with != without {
				t.Errorf("markdownToHTML(%q) = %q, want it unchanged: %q", tt.markdown, with, without)
			}
		})
	}
}

// A display name is chosen by its member, so it is shown as text and nothing else
func TestInsertPillsEscapesTheName(t *testing.T) {
	got := markdownToHTML("hi @bob:example.com", map[string]string{"@bob:example.com": `<img src=x onerror=alert(1)> & co`})
	if strings.Contains(got, "<img") || !strings.Contains(got, "&lt;img src=x onerror=alert(1)&gt; &amp; co</a>") {
		t.Errorf("markdownToHTML() = %q, want the name escaped", got)
	}
}

// Clients that show no formatting read the name where the user ID was
func TestPlainBodyShowsTheName(t *testing.T) {
	msg := formattedMessage("m.notice", markdownToHTML("ask @bob:example.com about it", pillNames), nil)
	if strings.TrimSpace(msg.Body) != "ask Bob about it" {
		t.Errorf("body = %q, want the name in place of the user ID", msg.Body)
	}
}
