package matrix

import "strings"

// A rich reply used to open with a quote of the message it replies to: a fallback for clients that
// can't render replies. The spec has since dropped it, but some clients still send it. The quote is
// someone else's message, so anything that wants a message's own words strips it first.

// StripReplyFallback removes the rich reply fallback from a plain text body: the run of "> " quoted
// lines a replying client prepends, and the blank lines that separate it from the actual message
func StripReplyFallback(body string) string {
	lines := strings.Split(body, "\n")
	i := 0
	for i < len(lines) && strings.HasPrefix(lines[i], "> ") {
		i++
	}
	if i == 0 {
		return body
	}
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	return strings.Join(lines[i:], "\n")
}

// StripMxReply removes the <mx-reply> block that a rich reply prepends to the formatted body
func StripMxReply(formattedBody string) string {
	const closingTag = "</mx-reply>"
	if idx := strings.Index(formattedBody, closingTag); idx >= 0 {
		return formattedBody[idx+len(closingTag):]
	}
	return formattedBody
}
