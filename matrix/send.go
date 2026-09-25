package matrix

import (
	"context"
	"html"
	"strings"
	"time"

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
	return sendMessage(ctx, roomID, formattedMessage("m.text", message, nil), nil)
}

// SendFormattedMessageWithDebugData queues a html-formatted message to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendFormattedMessageWithDebugData(ctx context.Context, roomID string, message string, debugData map[string]any) <-chan string {
	return sendMessage(ctx, roomID, formattedMessage("m.text", message, debugData), nil)
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
	return sendMessage(ctx, roomID, formattedMessage("m.notice", notice, nil), nil)
}

// SendFormattedNotice queues a html-formatted notice to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendFormattedNoticeWithDebugData(ctx context.Context, roomID string, notice string, debugData map[string]any) <-chan string {
	return sendMessage(ctx, roomID, formattedMessage("m.notice", notice, debugData), nil)
}

// SendMarkdownFormattedMessage converts markdown text to HTML and queues the formatted message to be sent.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendMarkdownFormattedMessage(ctx context.Context, roomID string, markdownText string) <-chan string {
	htmlOutput := markdownToHTML(markdownText, nil)
	return SendFormattedMessage(ctx, roomID, htmlOutput)
}

// SendMarkdownFormattedMessageWithDebugData converts markdown text to HTML and queues the formatted message to be sent.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendMarkdownFormattedMessageWithDebugData(ctx context.Context, roomID string, markdownText string, debugData map[string]any) <-chan string {
	htmlOutput := markdownToHTML(markdownText, nil)
	return SendFormattedMessageWithDebugData(ctx, roomID, htmlOutput, debugData)
}

// SendMarkdownFormattedNotice converts markdown text to HTML and queues the formatted notice to be sent.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendMarkdownFormattedNotice(ctx context.Context, roomID string, markdownText string) <-chan string {
	htmlOutput := markdownToHTML(markdownText, nil)
	return SendFormattedNotice(ctx, roomID, htmlOutput)
}

// SendMarkdownFormattedNotice converts markdown text to HTML and queues the formatted notice to be sent.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendMarkdownFormattedNoticeWithDebugData(ctx context.Context, roomID string, markdownText string, debugData map[string]any) <-chan string {
	htmlOutput := markdownToHTML(markdownText, nil)
	return SendFormattedNoticeWithDebugData(ctx, roomID, htmlOutput, debugData)
}

// Target says where a message goes, and whether it is still wanted when its turn to be sent comes.
//
// Both are decided just before the message is sent rather than when it is queued, since the queue
// can hold it for a while. A message answering another one goes out plainly if that message is
// still the latest one in its timeline, and as a reply to it otherwise (see timeline.go).
type Target struct {
	// ThreadRootID sends the message into that thread. Empty for the main timeline.
	ThreadRootID string
	// InReplyTo is the message this one answers, in the same timeline
	InReplyTo string
	// Cancelled is asked just before the message is sent, and again before any retry. If it
	// reports true, the message is dropped and the returned channel receives an empty event ID.
	Cancelled func() bool
	// Timeout, when set, gives the message up if it hasn't gone out this long after it was queued,
	// in place of the usual defaultSendTimeout. Someone waiting for the message sets it to how long
	// they wait, so that it never goes out after they have given up on it.
	Timeout time.Duration
}

// cancelled reports whether a message for the target is no longer wanted. A message without a
// target is always wanted.
func (t *Target) cancelled() bool {
	return t != nil && t.Cancelled != nil && t.Cancelled()
}

// timeline is the timeline a message for the target is posted in
func (t *Target) timeline(roomID string) timelineKey {
	key := timelineKey{roomID: roomID}
	if t != nil {
		key.threadRootID = t.ThreadRootID
	}
	return key
}

// relatesTo returns the relation that places a message at the target in the room, nil for a plain
// message in the main timeline. Whether the message is sent as a reply depends on what has been
// posted since the one it answers, so this is called just before it is sent.
func (t Target) relatesTo(roomID string) *event.RelatesTo {
	movedOn := t.InReplyTo != "" && !isLatest(t.timeline(roomID), t.InReplyTo)

	if t.ThreadRootID == "" {
		if !movedOn {
			return nil
		}
		return (&event.RelatesTo{}).SetReplyTo(id.EventID(t.InReplyTo))
	}

	// In a thread the message always carries a reply relation. Where nothing came in between, it
	// is only the fallback shown by clients without thread support, pointing at the latest message
	// in the thread, and the message shows as a plain one; otherwise it is a reply proper.
	rel := (&event.RelatesTo{}).SetThread(id.EventID(t.ThreadRootID), id.EventID(t.InReplyTo))
	if movedOn {
		rel.IsFallingBack = false
	}
	return rel
}

// SendMessageTo queues a message with debug data to be sent to the target, and returns
// immediately.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendMessageTo(ctx context.Context, roomID string, target Target, message string, debugData map[string]any) <-chan string {
	return sendMessage(ctx, roomID, simpleMessage{MsgType: "m.text", Body: message, DebugData: debugData}, &target)
}

// SendFormattedNoticeTo queues a html-formatted notice to be sent to the target, and returns
// immediately.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendFormattedNoticeTo(ctx context.Context, roomID string, target Target, notice string) <-chan string {
	return sendMessage(ctx, roomID, formattedMessage("m.notice", notice, nil), &target)
}

// SendMarkdownFormattedNoticeTo converts markdown text to HTML and queues the formatted notice,
// with debug data, to be sent to the target. Where the text writes out the user ID of one of the
// users in pills, the notice shows a pill for them, with the name pills gives.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendMarkdownFormattedNoticeTo(ctx context.Context, roomID string, target Target, markdownText string, pills map[string]string, debugData map[string]any) <-chan string {
	htmlOutput := markdownToHTML(markdownText, pills)
	return sendMessage(ctx, roomID, formattedMessage("m.notice", htmlOutput, debugData), &target)
}

// formattedMessage builds an html-formatted message, with a plain text body for clients that
// don't render html
func formattedMessage(msgType, html string, debugData map[string]any) simpleMessage {
	return simpleMessage{
		MsgType:       msgType,
		Body:          stripFormatting(html),
		Format:        "org.matrix.custom.html",
		FormattedBody: html,
		DebugData:     debugData,
	}
}

// sendMessage queues a message, placed at the target if there is one and in the main timeline
// otherwise
func sendMessage(ctx context.Context, roomID string, message simpleMessage, target *Target) <-chan string {
	done := make(chan string, 1)
	// The context travels with the event so the worker goroutine that actually talks to the
	// homeserver can continue the caller's trace rather than starting an unrelated one
	outboundEvents <- newOutboundEvent(ctx, roomID, message, target, done)
	metrics.SetMatrixOutboundQueueDepth(len(outboundEvents))
	return done
}

// markdownToHTML converts markdown text to HTML, showing a pill where the text writes out the user
// ID of one of the users in pills
func markdownToHTML(markdownText string, pills map[string]string) string {
	// Create markdown parser with extensions
	extensions := mdparser.CommonExtensions | mdparser.NoEmptyLineBeforeBlock
	parser := mdparser.NewWithExtensions(extensions)

	// Parse the markdown text
	md := []byte(markdownText)
	parsedMd := parser.Parse(md)

	// Put in once the text is parsed, so that code and links, which the parser tells apart, are
	// left as they were written
	if len(pills) > 0 {
		insertPills(parsedMd, pills)
	}

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
