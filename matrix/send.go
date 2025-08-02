package matrix

import (
	"html"
	"strings"

	"github.com/gomarkdown/markdown"
	mdhtml "github.com/gomarkdown/markdown/html"
	mdparser "github.com/gomarkdown/markdown/parser"
	strip "github.com/grokify/html-strip-tags-go"
)

// SendMessage queues a message to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendMessage(roomID string, message string) <-chan string {
	return sendMessage(roomID, simpleMessage{"m.text", message, "", "", nil}, true)
}

// SendMessageWithDebugData queues a message to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendMessageWithDebugData(roomID string, message string, debugData map[string]any) <-chan string {
	return sendMessage(roomID, simpleMessage{"m.text", message, "", "", debugData}, true)
}

// SendFormattedMessage queues a html-formatted message to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendFormattedMessage(roomID string, message string) <-chan string {
	return sendMessage(roomID, simpleMessage{"m.text", stripFormatting(message), "org.matrix.custom.html", message, nil}, true)
}

// SendFormattedMessageWithDebugData queues a html-formatted message to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendFormattedMessageWithDebugData(roomID string, message string, debugData map[string]any) <-chan string {
	return sendMessage(roomID, simpleMessage{"m.text", stripFormatting(message), "org.matrix.custom.html", message, debugData}, true)
}

// SendNotice queues a notice to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendNotice(roomID string, notice string) <-chan string {
	return sendMessage(roomID, simpleMessage{"m.notice", notice, "", "", nil}, true)
}

// SendNoticeWithDebugData queues a notice to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendNoticeWithDebugData(roomID string, notice string, debugData map[string]any) <-chan string {
	return sendMessage(roomID, simpleMessage{"m.notice", notice, "", "", debugData}, true)
}

// SendFormattedNotice queues a html-formatted notice to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendFormattedNotice(roomID string, notice string) <-chan string {
	return sendMessage(roomID, simpleMessage{"m.notice", stripFormatting(notice), "org.matrix.custom.html", notice, nil}, true)
}

// SendFormattedNotice queues a html-formatted notice to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendFormattedNoticeWithDebugData(roomID string, notice string, debugData map[string]any) <-chan string {
	return sendMessage(roomID, simpleMessage{"m.notice", stripFormatting(notice), "org.matrix.custom.html", notice, debugData}, true)
}

// SendMarkdownFormattedMessage converts markdown text to HTML and queues the formatted message to be sent.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendMarkdownFormattedMessage(roomID string, markdownText string) <-chan string {
	htmlOutput := markdownToHTML(markdownText)
	return SendFormattedMessage(roomID, htmlOutput)
}

// SendMarkdownFormattedMessageWithDebugData converts markdown text to HTML and queues the formatted message to be sent.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendMarkdownFormattedMessageWithDebugData(roomID string, markdownText string, debugData map[string]any) <-chan string {
	htmlOutput := markdownToHTML(markdownText)
	return SendFormattedMessageWithDebugData(roomID, htmlOutput, debugData)
}

// SendMarkdownFormattedNotice converts markdown text to HTML and queues the formatted notice to be sent.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendMarkdownFormattedNotice(roomID string, markdownText string) <-chan string {
	htmlOutput := markdownToHTML(markdownText)
	return SendFormattedNotice(roomID, htmlOutput)
}

// SendMarkdownFormattedNotice converts markdown text to HTML and queues the formatted notice to be sent.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendMarkdownFormattedNoticeWithDebugData(roomID string, markdownText string, debugData map[string]any) <-chan string {
	htmlOutput := markdownToHTML(markdownText)
	return SendFormattedNoticeWithDebugData(roomID, htmlOutput, debugData)
}

func sendMessage(roomID string, message any, retryOnFailure bool) <-chan string {
	done := make(chan string, 1)
	outboundEvents <- outboundEvent{roomID, "m.room.message", message, retryOnFailure, done}
	return done
}

// markdownToHTML converts markdown text to HTML
func markdownToHTML(markdownText string) string {
	// Create markdown parser with extensions
	extensions := mdparser.CommonExtensions | mdparser.NoEmptyLineBeforeBlock
	parser := mdparser.NewWithExtensions(extensions)

	// Parse the markdown text
	md := []byte(markdownText)
	parsedMd := parser.Parse(md)

	// Create HTML renderer with extensions
	htmlFlags := mdhtml.CommonFlags
	opts := mdhtml.RendererOptions{Flags: htmlFlags}
	renderer := mdhtml.NewRenderer(opts)

	// Convert to HTML
	return string(markdown.Render(parsedMd, renderer))
}

func stripFormatting(s string) string {
	// paragraph and header tags are on their own lines
	s = strings.Replace(s, "<p>", "\n", -1)
	s = strings.Replace(s, "<h1>", "\n", -1)
	s = strings.Replace(s, "<h2>", "\n", -1)
	s = strings.Replace(s, "<h3>", "\n", -1)
	s = strings.Replace(s, "<h4>", "\n", -1)
	s = strings.Replace(s, "<h5>", "\n", -1)
	s = strings.Replace(s, "<h6>", "\n", -1)
	s = strings.Replace(s, "</p>", "\n", -1)
	s = strings.Replace(s, "</h1>", "\n", -1)
	s = strings.Replace(s, "</h2>", "\n", -1)
	s = strings.Replace(s, "</h3>", "\n", -1)
	s = strings.Replace(s, "</h4>", "\n", -1)
	s = strings.Replace(s, "</h5>", "\n", -1)
	s = strings.Replace(s, "</h6>", "\n", -1)
	// beginning of every list element means beginning of a new line, break the line at the end of the list
	s = strings.Replace(s, "<li>", "\n - ", -1)
	s = strings.Replace(s, "</ul>", "\n", -1)
	// table cells have a space between them and row end ends the line
	s = strings.Replace(s, "</td>", " ", -1)
	s = strings.Replace(s, "</tr>", "\n", -1)
	// duh
	s = strings.Replace(s, "<br>", "\n", -1)
	s = strings.Replace(s, "<br/>", "\n", -1)
	s = strings.Replace(s, "<br />", "\n", -1)
	return strip.StripTags(html.UnescapeString(s))
}
