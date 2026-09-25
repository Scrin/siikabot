package matrix

import (
	"context"
	"html"
	"strings"

	"github.com/Scrin/siikabot/metrics"
	"github.com/gomarkdown/markdown"
	mdhtml "github.com/gomarkdown/markdown/html"
	mdparser "github.com/gomarkdown/markdown/parser"
	strip "github.com/grokify/html-strip-tags-go"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// SendMessage queues a message to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendMessage(ctx context.Context, roomID string, message string) <-chan string {
	return sendMessage(ctx, roomID, simpleMessage{MsgType: "m.text", Body: message}, nil)
}

// SendMessageWithDebugData queues a message to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendMessageWithDebugData(ctx context.Context, roomID string, message string, debugData map[string]any) <-chan string {
	return sendMessage(ctx, roomID, simpleMessage{MsgType: "m.text", Body: message, DebugData: debugData}, nil)
}

// SendFormattedMessage queues a html-formatted message to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendFormattedMessage(ctx context.Context, roomID string, message string) <-chan string {
	return sendMessage(ctx, roomID, formattedMessage("m.text", message, nil, nil), nil)
}

// SendFormattedMessageWithDebugData queues a html-formatted message to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendFormattedMessageWithDebugData(ctx context.Context, roomID string, message string, debugData map[string]any) <-chan string {
	return sendMessage(ctx, roomID, formattedMessage("m.text", message, debugData, nil), nil)
}

// SendNotice queues a notice to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendNotice(ctx context.Context, roomID string, notice string) <-chan string {
	return sendMessage(ctx, roomID, simpleMessage{MsgType: "m.notice", Body: notice}, nil)
}

// SendNoticeWithDebugData queues a notice to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendNoticeWithDebugData(ctx context.Context, roomID string, notice string, debugData map[string]any) <-chan string {
	return sendMessage(ctx, roomID, simpleMessage{MsgType: "m.notice", Body: notice, DebugData: debugData}, nil)
}

// SendFormattedNotice queues a html-formatted notice to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendFormattedNotice(ctx context.Context, roomID string, notice string) <-chan string {
	return sendMessage(ctx, roomID, formattedMessage("m.notice", notice, nil, nil), nil)
}

// SendFormattedNotice queues a html-formatted notice to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendFormattedNoticeWithDebugData(ctx context.Context, roomID string, notice string, debugData map[string]any) <-chan string {
	return sendMessage(ctx, roomID, formattedMessage("m.notice", notice, debugData, nil), nil)
}

// SendMarkdownFormattedMessage converts markdown text to HTML and queues the formatted message to be sent.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendMarkdownFormattedMessage(ctx context.Context, roomID string, markdownText string) <-chan string {
	htmlOutput := markdownToHTML(markdownText)
	return SendFormattedMessage(ctx, roomID, htmlOutput)
}

// SendMarkdownFormattedMessageWithDebugData converts markdown text to HTML and queues the formatted message to be sent.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendMarkdownFormattedMessageWithDebugData(ctx context.Context, roomID string, markdownText string, debugData map[string]any) <-chan string {
	htmlOutput := markdownToHTML(markdownText)
	return SendFormattedMessageWithDebugData(ctx, roomID, htmlOutput, debugData)
}

// SendMarkdownFormattedNotice converts markdown text to HTML and queues the formatted notice to be sent.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendMarkdownFormattedNotice(ctx context.Context, roomID string, markdownText string) <-chan string {
	htmlOutput := markdownToHTML(markdownText)
	return SendFormattedNotice(ctx, roomID, htmlOutput)
}

// SendMarkdownFormattedNotice converts markdown text to HTML and queues the formatted notice to be sent.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendMarkdownFormattedNoticeWithDebugData(ctx context.Context, roomID string, markdownText string, debugData map[string]any) <-chan string {
	htmlOutput := markdownToHTML(markdownText)
	return SendFormattedNoticeWithDebugData(ctx, roomID, htmlOutput, debugData)
}

// Target says where a message goes, and whether it is still wanted when its turn to be sent comes.
// The zero value sends it to the main timeline unconditionally, like the plain Send functions.
type Target struct {
	// ThreadRootID sends the message into that thread, following up on the thread message
	// InReplyTo. Empty for the main timeline.
	ThreadRootID string
	InReplyTo    string
	// Cancelled is asked just before the message is sent. If it reports true, the message is
	// dropped and the returned channel receives an empty event ID.
	Cancelled func() bool
}

// relatesTo returns the relation that places a message at the target, nil for the main timeline
func (t Target) relatesTo() *event.RelatesTo {
	if t.ThreadRootID == "" {
		return nil
	}
	// A plain message in the thread. The reply relation it carries is only the fallback shown by
	// clients without thread support, pointing at the message this one follows up on.
	return (&event.RelatesTo{}).SetThread(id.EventID(t.ThreadRootID), id.EventID(t.InReplyTo))
}

// SendMessageTo queues a message with debug data to be sent to the target, and returns
// immediately.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendMessageTo(ctx context.Context, roomID string, target Target, message string, debugData map[string]any) <-chan string {
	msg := simpleMessage{MsgType: "m.text", Body: message, DebugData: debugData, RelatesTo: target.relatesTo()}
	return sendMessage(ctx, roomID, msg, target.Cancelled)
}

// SendFormattedNoticeTo queues a html-formatted notice to be sent to the target, and returns
// immediately.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendFormattedNoticeTo(ctx context.Context, roomID string, target Target, notice string) <-chan string {
	return sendMessage(ctx, roomID, formattedMessage("m.notice", notice, nil, target.relatesTo()), target.Cancelled)
}

// SendMarkdownFormattedNoticeTo converts markdown text to HTML and queues the formatted notice,
// with debug data, to be sent to the target
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendMarkdownFormattedNoticeTo(ctx context.Context, roomID string, target Target, markdownText string, debugData map[string]any) <-chan string {
	htmlOutput := markdownToHTML(markdownText)
	return sendMessage(ctx, roomID, formattedMessage("m.notice", htmlOutput, debugData, target.relatesTo()), target.Cancelled)
}

// formattedMessage builds an html-formatted message, with a plain text body for clients that
// don't render html
func formattedMessage(msgType, html string, debugData map[string]any, relatesTo *event.RelatesTo) simpleMessage {
	return simpleMessage{
		MsgType:       msgType,
		Body:          stripFormatting(html),
		Format:        "org.matrix.custom.html",
		FormattedBody: html,
		RelatesTo:     relatesTo,
		DebugData:     debugData,
	}
}

func sendMessage(ctx context.Context, roomID string, message any, cancelled func() bool) <-chan string {
	done := make(chan string, 1)
	// The context travels with the event so the worker goroutine that actually talks to the
	// homeserver can continue the caller's trace rather than starting an unrelated one
	outboundEvents <- outboundEvent{
		ctx:            ctx,
		RoomID:         roomID,
		EventType:      "m.room.message",
		Content:        message,
		RetryOnFailure: true,
		done:           done,
		cancelled:      cancelled,
	}
	metrics.SetMatrixOutboundQueueDepth(len(outboundEvents))
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
