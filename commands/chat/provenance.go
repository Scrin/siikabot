package chat

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/webfetch"
)

// withFetchProvenance gives a tool context the provenance rule for web fetches and lookups, from the messages
// the turn holds so far (webfetch.Provenance). The rule applies once any of them is a result from a tool that
// isn't trusted, whether this turn called it or the history replays it. An expired result is replaced in the
// history by a marker, which holds nothing from the tool, so it doesn't count.
//
// The URLs and names the turn may use then are the ones in the room's messages and in tool results, taken in
// order. Whatever the model wrote doesn't count, and nor does a tool repeating it back later: its answers, the
// arguments of its tool calls, and the bot's own messages that the room's messages quote. The system messages
// don't count either: they hold the bot's memories, which the model wrote.
func withFetchProvenance(ctx context.Context, messages []aigateway.Message, tools []aigateway.ToolDefinition) context.Context {
	return webfetch.WithProvenance(ctx, fetchProvenance(messages, tools))
}

// fetchProvenance works out the rule for withFetchProvenance
func fetchProvenance(messages []aigateway.Message, tools []aigateway.ToolDefinition) webfetch.Provenance {
	trusted := make(map[string]bool, len(tools))
	for _, tool := range tools {
		trusted[tool.Function.Name] = tool.Trusted
	}

	// A tool result names only the call it answers, and the call names the tool
	toolNames := map[string]string{}
	restricted := false
	for _, msg := range messages {
		for _, call := range msg.ToolCalls {
			toolNames[call.ID] = call.Function.Name
		}
		if msg.Role == "tool" {
			// A tool no longer registered counts as untrusted
			name := toolNames[msg.ToolCallID]
			if !trusted[name] && messageText(msg) != expiredToolResult(name) {
				restricted = true
			}
		}
	}

	p := webfetch.NewProvenance(restricted)
	if !restricted {
		return p
	}
	for _, msg := range messages {
		switch msg.Role {
		case "assistant":
			p.Wrote(messageText(msg))
			for _, call := range msg.ToolCalls {
				p.Wrote(argumentsText(call.Function.Arguments))
			}
		case "user":
			for _, own := range msg.OwnText {
				p.Wrote(own)
			}
			p.See(messageText(msg))
		case "tool":
			p.See(messageText(msg))
		}
	}
	return p
}

// messageText returns the text of a message, without any images in it
func messageText(msg aigateway.Message) string {
	switch content := msg.Content.(type) {
	case string:
		return content
	case []aigateway.ContentPart:
		texts := make([]string, 0, len(content))
		for _, part := range content {
			texts = append(texts, part.Text)
		}
		return strings.Join(texts, "\n")
	}
	return ""
}

// argumentsText returns what the model wrote in a tool call's arguments: the arguments as written, and each
// string in them as a tool reads it, with JSON's escapes undone. A tool repeats the strings, so they have to
// be read the way the tool reads them.
func argumentsText(arguments string) string {
	texts := []string{arguments}
	var parsed any
	if err := json.Unmarshal([]byte(arguments), &parsed); err == nil {
		texts = appendStrings(texts, parsed)
	}
	return strings.Join(texts, "\n")
}

// appendStrings appends every string in a parsed JSON value
func appendStrings(texts []string, value any) []string {
	switch value := value.(type) {
	case string:
		texts = append(texts, value)
	case []any:
		for _, item := range value {
			texts = appendStrings(texts, item)
		}
	case map[string]any:
		for key, item := range value {
			texts = appendStrings(append(texts, key), item)
		}
	}
	return texts
}

// ownText returns what a user turn quotes of the bot's own messages, which the model may have written. A
// message the bot relayed from a webhook is the webhook's, not the bot's.
func ownText(turn db.UserTurn) []string {
	quotes := turn.Links
	if turn.ReplyTo != nil {
		quotes = append([]db.QuotedMessage{*turn.ReplyTo}, quotes...)
	}
	var texts []string
	for _, quote := range quotes {
		if quote.Sender == config.UserID && quote.RelayedBy == "" && quote.Body != "" {
			texts = append(texts, quote.Body)
		}
	}
	return texts
}
