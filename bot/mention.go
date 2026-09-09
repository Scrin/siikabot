package bot

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// The chat feature only reacts to a message that is unambiguously aimed at the bot: an explicit
// m.mentions entry for the bot, or a message that opens by naming the bot. Naming the bot in the
// middle of a sentence is talking *about* it, not *to* it, and used to trigger a reply.

// mentionsBotExplicitly reports whether the m.mentions field of the event content names the bot.
//
// A room-wide mention ("room": true) deliberately does not count: an @room ping is addressed to
// everyone in the room, not a question for the bot.
func mentionsBotExplicitly(rawContent map[string]any, botUserID string) bool {
	mentions, ok := rawContent["m.mentions"].(map[string]any)
	if !ok {
		return false
	}
	userIDs, ok := mentions["user_ids"].([]any)
	if !ok {
		return false
	}
	for _, userID := range userIDs {
		if id, ok := userID.(string); ok && id == botUserID {
			return true
		}
	}
	return false
}

// stripBotNamePrefix reports whether the message opens by addressing the bot — by display name, by
// full user ID, or with a pill in the formatted body — and returns the message with that address
// removed. The bare localpart of the user ID is not accepted, since it is often something generic
// enough ("bot") to match messages that were never meant for us.
func stripBotNamePrefix(plainMsg, formattedMsg, botUserID, botDisplayName string) (string, bool) {
	body := strings.TrimSpace(stripReplyFallback(plainMsg))

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
	body := stripMxReply(formattedMsg)
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

// stripMxReply removes the <mx-reply> block that a rich reply prepends to the formatted body, so
// that a pill at the start of the actual message is not hidden behind the quoted one
func stripMxReply(formattedMsg string) string {
	const closingTag = "</mx-reply>"
	if idx := strings.Index(formattedMsg, closingTag); idx >= 0 {
		return formattedMsg[idx+len(closingTag):]
	}
	return formattedMsg
}

// stripReplyFallback removes the rich reply fallback from a plain text body: the run of "> " quoted
// lines a replying client prepends. Without this, a reply that opens by naming the bot looks like
// it opens with the quoted message instead and the prefix check misses it. Nothing is lost by
// dropping it here, as the chat feature fetches the replied-to event separately for context.
func stripReplyFallback(plainMsg string) string {
	lines := strings.Split(plainMsg, "\n")
	i := 0
	for i < len(lines) && strings.HasPrefix(lines[i], "> ") {
		i++
	}
	if i == 0 {
		return plainMsg
	}
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	return strings.Join(lines[i:], "\n")
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
