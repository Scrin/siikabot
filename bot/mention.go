package bot

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Scrin/siikabot/matrix"
)

// The chat feature only reacts to a message that is unambiguously aimed at the bot: an explicit
// m.mentions entry for the bot, or a message that opens by naming the bot. Naming the bot in the
// middle of a sentence is talking *about* it, not *to* it, and used to trigger a reply.

// mentionsBotExplicitly reports whether the m.mentions field of the event content names the bot.
//
// A room-wide mention ("room": true) deliberately does not count: an @room ping is addressed to
// everyone in the room, not a question for the bot.
func mentionsBotExplicitly(rawContent map[string]any, botUserID string) bool {
	return slices.Contains(mentionedUserIDs(rawContent), botUserID)
}

// mentionedUserIDs returns the users listed in the m.mentions field of the event content
func mentionedUserIDs(rawContent map[string]any) []string {
	mentions, ok := rawContent["m.mentions"].(map[string]any)
	if !ok {
		return nil
	}
	userIDs, ok := mentions["user_ids"].([]any)
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		if id, ok := userID.(string); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// stripBotNamePrefix reports whether the message opens by addressing the bot — by display name, by
// full user ID, or with a pill in the formatted body — and returns the message with that address
// removed. The bare localpart of the user ID is not accepted, since it is often something generic
// enough ("bot") to match messages that were never meant for us.
func stripBotNamePrefix(plainMsg, formattedMsg, botUserID, botDisplayName string) (string, bool) {
	// A replying client may prepend a quote of the message it replies to. Left in, a reply that
	// opens by naming the bot would look like it opens with the quote, and the check would miss it.
	body := strings.TrimSpace(matrix.StripReplyFallback(plainMsg))

	names := []string{botUserID}
	if botDisplayName != "" && !strings.EqualFold(botDisplayName, botUserID) {
		names = append(names, botDisplayName)
	}
	for _, name := range names {
		if rest, ok := matchNamePrefix(body, name); ok {
			return rest, true
		}
	}

	// A client that pills the mention writes the pill's text into the plain body, and that text is
	// whatever the sender's client chose to display — not necessarily the display name we resolved.
	// The pill itself already establishes that the message is for us, so trust it and strip the
	// text it carries.
	if pillText, ok := leadingBotPillText(formattedMsg, botUserID); ok {
		if rest, ok := matchNamePrefix(body, pillText); ok {
			return rest, true
		}
		return body, true
	}

	return "", false
}

// matchNamePrefix reports whether body opens with the given name, case-insensitively and with an
// optional leading "@", and returns the rest of the message with the name and any separator
// removed. The name has to end at a boundary, so "siikabottle" does not address "siikabot".
func matchNamePrefix(body, name string) (string, bool) {
	if name == "" {
		return "", false
	}
	rest := body
	if !strings.HasPrefix(name, "@") {
		rest = strings.TrimPrefix(rest, "@")
	}
	if len(rest) < len(name) || !strings.EqualFold(rest[:len(name)], name) {
		return "", false
	}
	rest = rest[len(name):]
	if r, size := utf8.DecodeRuneInString(rest); rest != "" {
		if isWordRune(r) {
			return "", false
		}
		// A separator that continues into more word characters means the name was only part of a
		// larger token: "siikabot.example.com is down" reports a host, it does not address the bot
		if strings.ContainsRune(tokenSeparators, r) {
			if next, _ := utf8.DecodeRuneInString(rest[size:]); isWordRune(next) {
				return "", false
			}
		}
	}
	// "siikabot hi", "siikabot: hi" and "siikabot, hi" all address the bot the same way
	return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(rest), ":,")), true
}

// tokenSeparators are the characters that hold a single token together — a hostname, a path, a
// user id — rather than separating words
const tokenSeparators = "-./:@\\"

// isWordRune reports whether the rune would be part of a name, meaning a name that ends right
// before it did not actually end there
func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// leadingBotPillText returns the display text of a pill for the bot at the start of the formatted
// body. A pill further into the message is not a match: that is the mid-sentence case this whole
// file exists to stop reacting to.
func leadingBotPillText(formattedMsg, botUserID string) (string, bool) {
	// Without the quote a rich reply prepends, so a pill at the start of the actual message is not
	// hidden behind the quoted one
	body := matrix.StripMxReply(formattedMsg)
	linkIdx := strings.Index(body, "https://matrix.to/#/"+botUserID)
	if linkIdx < 0 {
		return "", false
	}
	// Markup before the pill is fine — the <a> tag the pill lives in, at least — but text the
	// sender typed is not, because then the pill is not what the message opens with.
	if strings.TrimSpace(stripTags(body[:linkIdx])) != "" {
		return "", false
	}
	tagEnd := strings.Index(body[linkIdx:], ">")
	if tagEnd < 0 {
		return "", true
	}
	text := body[linkIdx+tagEnd+1:]
	if closeIdx := strings.Index(text, "</a>"); closeIdx >= 0 {
		text = text[:closeIdx]
	}
	return strings.TrimSpace(stripTags(text)), true
}

// stripTags removes HTML tags from a fragment, leaving the text a reader would actually see
func stripTags(fragment string) string {
	var text strings.Builder
	inTag := false
	for _, r := range fragment {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			text.WriteRune(r)
		}
	}
	return text.String()
}
