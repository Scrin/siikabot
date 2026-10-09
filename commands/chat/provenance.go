package chat

import (
	"context"
	"strings"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/webfetch"
)

// withFetchProvenance gives a tool context the provenance rule for web fetches, from the messages the turn
// holds so far (webfetch.Provenance). The rule applies once any of them is a result from a tool that isn't
// trusted, whether this turn called it or the history replays it. An expired result is replaced in the
// history by a marker, which holds nothing from the tool, so it doesn't count.
//
// The URLs the turn may fetch then are the ones in the room's messages, the bot's own notes and tool results.
// The model's own messages don't count: content could get it to write a URL out first and fetch it after.
func withFetchProvenance(ctx context.Context, messages []aigateway.Message, tools []aigateway.ToolDefinition) context.Context {
	trusted := make(map[string]bool, len(tools))
	for _, tool := range tools {
		trusted[tool.Function.Name] = tool.Trusted
	}

	// A tool result names only the call it answers, and the call names the tool
	toolNames := map[string]string{}
	restricted := false
	texts := make([]string, 0, len(messages))
	for _, msg := range messages {
		text := messageText(msg)
		switch msg.Role {
		case "assistant":
			for _, call := range msg.ToolCalls {
				toolNames[call.ID] = call.Function.Name
			}
			continue
		case "tool":
			// A tool no longer registered counts as untrusted
			name := toolNames[msg.ToolCallID]
			if !trusted[name] && text != expiredToolResult(name) {
				restricted = true
			}
		}
		texts = append(texts, text)
	}
	return webfetch.WithProvenance(ctx, webfetch.NewProvenance(restricted, texts...))
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
