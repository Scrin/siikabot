package chat

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
)

// Several people may be talking to the bot in the same room, so every user message reaches the
// model with a header saying who wrote it and when. The "Conversation format" section of the system
// prompt explains the format to the model.
//
// The header is written by the bot, never the sender. Display names are cleaned of anything that
// could make them look like part of it, and lines in a message that look like a header are escaped.

// headerTimeLayout is how message times are shown to the model, in the bot's timezone
const headerTimeLayout = "2006-01-02 15:04"

// currentTimeLayout is how the current time is shown: the header format plus the weekday and
// seconds, so the model compares like with like
const currentTimeLayout = "Monday 2006-01-02 15:04:05 MST"

// maxNameLength caps a display name as shown to the model, in characters
const maxNameLength = 64

// maxQuoteLength caps the quoted message kept with a turn, in characters
const maxQuoteLength = 2000

// displayLocation is the bot's timezone, loaded once
var displayLocation = sync.OnceValue(func() *time.Location {
	loc, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
})

// localTime renders a time the way headers show it
func localTime(t time.Time) string {
	return t.In(displayLocation()).Format(headerTimeLayout)
}

// renderUserTurn renders a user message as the model sees it: a header saying who wrote it and
// when, the messages it refers to, then the message itself, with its own image if it is one.
//
// The same function renders the current turn and every replayed one, so a message looks the same
// once it becomes history and the prompt prefix stays cacheable. The one difference is the images,
// which are attached to the current turn only: attached says whose images are, by event ID, and is
// nil for a replayed turn.
func renderUserTurn(turn db.UserTurn, attached map[string]bool) string {
	var b strings.Builder

	b.WriteString("[" + person(turn.SenderName, turn.UserID) + " · " + localTime(turn.SentAt))
	if n := turn.UnseenBefore; n > 0 {
		fmt.Fprintf(&b, " · %d unseen room %s before this", n, plural(n, "message", "messages"))
	}
	if len(turn.Mentions) > 0 {
		names := make([]string, len(turn.Mentions))
		for i, mention := range turn.Mentions {
			names[i] = person(mention.Name, mention.UserID)
		}
		b.WriteString(" · mentions " + strings.Join(names, ", "))
	}
	b.WriteString("]\n")

	if turn.ReplyTo != nil {
		b.WriteString(renderQuote(*turn.ReplyTo, db.RelationReply, attached[turn.ReplyTo.EventID]))
	}
	for _, link := range turn.Links {
		b.WriteString(renderQuote(link, db.RelationLink, attached[link.EventID]))
	}

	if turn.HasImage {
		if attached[turn.EventID] {
			b.WriteString("[with an image]\n")
		} else {
			b.WriteString("[with an image, not attached]\n")
		}
	}

	b.WriteString(escapeHeaderLike(turn.Message))
	return b.String()
}

// renderQuote renders a message a turn refers to, the one it replies to or one it links to,
// between the header and the turn's own text
func renderQuote(quote db.QuotedMessage, relation string, imageAttached bool) string {
	from := person(quote.SenderName, quote.Sender)
	when := localTime(quote.SentAt)
	refersTo := "replying to"
	if relation == db.RelationLink {
		refersTo = "linking to"
	}

	switch quote.Kind {
	case db.QuoteDeleted:
		if relation == db.RelationLink {
			return "[linking to a message that was deleted]\n"
		}
		return "[the replied-to message was deleted]\n"
	case db.QuoteImage:
		line := "[" + refersTo + " an image from " + from + " · " + when
		if !imageAttached {
			line += ", not attached"
		}
		return line + "]\n" + quoteLines(quote.Body)
	}

	switch {
	case quote.Sender == config.UserID:
		from = "your message"
	case relation == db.RelationLink:
		from = "a message from " + from
	}
	return "[" + refersTo + " " + from + " · " + when + "]\n" + quoteLines(quote.Body)
}

// quoteLines renders quoted text, each line marked as a quote. An image without a caption has none.
func quoteLines(text string) string {
	if text == "" {
		return ""
	}
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		b.WriteString("> " + line + "\n")
	}
	return b.String()
}

// person renders someone for the model: their name and user ID, or only the user ID when they have
// no name of their own
func person(name, userID string) string {
	if name == "" || name == userID {
		return userID
	}
	return name + " (" + userID + ")"
}

// cleanName turns a display name into something safe to show the model: a single line, without
// the characters that make up the header syntax, and not too long. Members choose their own
// display names, so they are untrusted input.
func cleanName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsControl(r) || unicode.IsSpace(r):
			b.WriteRune(' ')
		case unicode.Is(unicode.Cf, r), strings.ContainsRune("[]()·", r):
			// Invisible formatting characters and header syntax are dropped
		default:
			b.WriteRune(r)
		}
	}

	cleaned := strings.Join(strings.Fields(b.String()), " ")
	if utf8.RuneCountInString(cleaned) > maxNameLength {
		cleaned = string([]rune(cleaned)[:maxNameLength]) + "…"
	}
	return cleaned
}

// headerLike matches a line that could pass for a header
var headerLike = regexp.MustCompile(`^\s*\[.*\]\s*$`)

// escapeHeaderLike escapes the lines of a message that look like a header, so that a message can't
// pass its text off as coming from someone else
func escapeHeaderLike(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if headerLike.MatchString(line) {
			lines[i] = `\` + line
		}
	}
	return strings.Join(lines, "\n")
}

// imitatedHeader matches a line written in the header's own format: a name followed by " · " and
// the time, in brackets
var imitatedHeader = regexp.MustCompile(`^\s*\[[^\]\n]* · \d{4}-\d{2}-\d{2} \d{2}:\d{2}[^\]\n]*\]\s*$`)

// stripImitatedHeader removes a header the model has put at the start of its answer. Every user
// message it sees starts with one, and a model that copies the pattern would otherwise post it to
// the room. An answer that is nothing but a header is left as it is, rather than sent empty.
func stripImitatedHeader(answer string) string {
	first, rest, _ := strings.Cut(answer, "\n")
	if !imitatedHeader.MatchString(first) {
		return answer
	}
	rest = strings.TrimLeft(rest, "\n")
	if strings.TrimSpace(rest) == "" {
		return answer
	}
	return rest
}

// capQuote shortens a quoted message to maxQuoteLength characters, marking the cut
func capQuote(body string) string {
	if utf8.RuneCountInString(body) <= maxQuoteLength {
		return body
	}
	return string([]rune(body)[:maxQuoteLength]) + " …[truncated]"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
