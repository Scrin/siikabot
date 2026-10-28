package matrix

import (
	"context"
	"encoding/json"
	"html"
	"io"
	"strings"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	mdhtml "github.com/gomarkdown/markdown/html"
	mdparser "github.com/gomarkdown/markdown/parser"
)

// A webhook relays messages into a room. They are sent by the bot, but their words are an outside
// service's, so each one says in its content which webhook it came from, and opens with the
// webhook's name in brackets.

// WebhookOrigin is the webhook a message was relayed from
type WebhookOrigin struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// webhookOriginKey is where in its content a relayed message names its webhook
const webhookOriginKey = "fi.2kgwf.webhook"

// WebhookMessage is a message relayed from a webhook, ready to be sent
type WebhookMessage struct {
	content simpleMessage
}

// NewWebhookMessage builds the message a webhook relays: its Markdown text under the webhook's name.
//
// A message that is a single line of text reads "[name]: text". Anything else, more than one line
// or a single line that is a heading, a list item or another block, has the name on a line of its
// own above it, so that the message's first line starts a line as it was written to.
//
// The name doesn't go through Markdown: "[name]: text" is how Markdown defines a link, and for many
// a message, such as a single word or a URL, would render as nothing at all.
func NewWebhookMessage(origin WebhookOrigin, markdownText string) WebhookMessage {
	rendered := strings.TrimRight(webhookMarkdownToHTML(markdownText), "\n")
	name := "[" + origin.Name + "]"
	escapedName := html.EscapeString(name)

	var body, formatted string
	rest, opensWithParagraph := strings.CutPrefix(rendered, "<p>")
	switch {
	case opensWithParagraph && !strings.ContainsAny(markdownText, "\r\n"):
		body = name + ": " + markdownText
		formatted = "<p>" + escapedName + ": " + rest
	case opensWithParagraph:
		// Inside the paragraph rather than above it, so that no paragraph's gap opens under the name
		body = name + "\n" + markdownText
		formatted = "<p>" + escapedName + "<br>\n" + rest
	default:
		body = name + "\n" + markdownText
		formatted = "<p>" + escapedName + "</p>\n" + rendered
	}

	return WebhookMessage{content: simpleMessage{
		MsgType:       "m.text",
		Body:          body,
		Format:        "org.matrix.custom.html",
		FormattedBody: formatted,
		WebhookOrigin: &origin,
	}}
}

// Size is how many bytes the message's content takes, before it is encrypted
func (m WebhookMessage) Size() int {
	encoded, err := json.Marshal(m.content)
	if err != nil {
		return 0
	}
	return len(encoded)
}

// SendWebhookMessage queues a message relayed from a webhook, and returns immediately. The message
// is given up if it hasn't gone out within timeout, so it never goes out after whoever sent it has
// stopped waiting.
//
// The returned channel will provide the event ID of the message after the message has been sent,
// or an empty one if it wasn't
func SendWebhookMessage(ctx context.Context, roomID string, msg WebhookMessage, timeout time.Duration) <-chan string {
	return sendMessage(ctx, roomID, msg.content, &Target{Timeout: timeout})
}

// RelayedBy returns the name of the webhook the message was relayed from, or an empty string for
// any other message. Only the bot relays messages, so a webhook's mark on anyone else's message is
// ignored.
func (m *Message) RelayedBy() string {
	if m.Sender != config.UserID {
		return ""
	}
	origin, _ := m.content[webhookOriginKey].(map[string]any)
	name, _ := origin["name"].(string)
	return name
}

// webhookMarkdownExtensions are the Markdown a webhook message may use. Unlike in the chat's replies,
// every line break is kept, since what is sent with curl is often lines of output. It also leaves out
// the extensions that misread everyday text: MathJax takes the text between two dollar signs, as in
// "$5 and $10", for math, and a line opening with ": " turns the line above it into a definition list.
const webhookMarkdownExtensions = (mdparser.CommonExtensions | mdparser.NoEmptyLineBeforeBlock | mdparser.HardLineBreak) &^
	(mdparser.MathJax | mdparser.DefinitionLists)

// webhookMarkdownToHTML converts the Markdown of a webhook message to HTML. HTML written into the
// message is shown as text, so that its formatting comes from Markdown alone.
//
// Nor is the text made typographic as the chat's replies are: output such as --force, 1/2 or "ok"
// reads as it was written, rather than with a dash, a fraction or curly quotes.
func webhookMarkdownToHTML(markdownText string) string {
	parser := mdparser.NewWithExtensions(webhookMarkdownExtensions)
	renderer := mdhtml.NewRenderer(mdhtml.RendererOptions{
		Flags:          mdhtml.FlagsNone,
		RenderNodeHook: renderRawHTMLAsText,
	})
	rendered := string(markdown.Render(parser.Parse([]byte(markdownText)), renderer))

	// The line break that ends a list item's line is kept too, though it breaks nothing. The text
	// can't hold one of its own, since its HTML comes out escaped.
	return strings.ReplaceAll(rendered, "<br>\n</li>", "</li>")
}

// renderRawHTMLAsText renders the HTML in a webhook message as the text it was written as
func renderRawHTMLAsText(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
	switch node := node.(type) {
	case *ast.HTMLSpan:
		io.WriteString(w, html.EscapeString(string(node.Literal)))
		return ast.GoToNext, true
	case *ast.HTMLBlock:
		lines := strings.Split(strings.TrimRight(string(node.Literal), "\n"), "\n")
		for i, line := range lines {
			lines[i] = html.EscapeString(line)
		}
		io.WriteString(w, "<p>"+strings.Join(lines, "<br>\n")+"</p>\n")
		return ast.GoToNext, true
	}
	return ast.GoToNext, false
}
