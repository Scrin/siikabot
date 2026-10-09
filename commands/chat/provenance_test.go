package chat

import (
	"context"
	"net/url"
	"testing"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/webfetch"
)

var provenanceTools = []aigateway.ToolDefinition{
	{Function: aigateway.FunctionSchema{Name: "get_fmi_data"}, Trusted: true},
	{Function: aigateway.FunctionSchema{Name: "web_search"}},
}

// toolExchange is a model's call to a tool and the tool's result, as they appear in the messages
func toolExchange(id, tool, result string) []aigateway.Message {
	return []aigateway.Message{
		{Role: "assistant", ToolCalls: []aigateway.ToolCall{{ID: id, Type: "function", Function: aigateway.ToolFunction{Name: tool}}}},
		{Role: "tool", ToolCallID: id, Content: result},
	}
}

// allows reports whether a turn with these messages may fetch a URL
func allows(t *testing.T, messages []aigateway.Message, rawURL string) bool {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	ctx := withFetchProvenance(context.Background(), messages, provenanceTools)
	return webfetch.ProvenanceFrom(ctx).Allows(u)
}

func TestFetchProvenance(t *testing.T) {
	system := aigateway.Message{Role: "system", Content: "You are a bot."}
	user := aigateway.Message{Role: "user", Content: "Alice: what does https://example.com/article say?"}
	built := "https://en.wikipedia.org/wiki/Siika"

	t.Run("no_tool_results", func(t *testing.T) {
		if !allows(t, []aigateway.Message{system, user}, built) {
			t.Error("a turn that has read nothing from outside the room can't fetch a URL the model built")
		}
	})

	t.Run("trusted_result", func(t *testing.T) {
		messages := append([]aigateway.Message{system, user}, toolExchange("1", "get_fmi_data", "Helsinki: 12 °C")...)
		if !allows(t, messages, built) {
			t.Error("a trusted tool's result restricts fetches")
		}
	})

	t.Run("untrusted_result", func(t *testing.T) {
		messages := append([]aigateway.Message{system, user},
			toolExchange("1", "web_search", "### [Siika](https://fi.wikipedia.org/wiki/Siika)")...)
		if allows(t, messages, built) {
			t.Error("after a search, the model can fetch a URL it built")
		}
		if !allows(t, messages, "https://fi.wikipedia.org/wiki/Siika") || !allows(t, messages, "https://example.com/article") {
			t.Error("after a search, the model can't fetch the URLs the room and the search gave")
		}
	})

	t.Run("unknown_tool", func(t *testing.T) {
		messages := append([]aigateway.Message{system, user}, toolExchange("1", "get_weather", "Sunny")...)
		if allows(t, messages, built) {
			t.Error("a result from a tool that's no longer registered doesn't restrict fetches")
		}
	})

	t.Run("expired_result", func(t *testing.T) {
		messages := append([]aigateway.Message{system, user},
			toolExchange("1", "web_search", expiredToolResult("web_search"))...)
		if !allows(t, messages, built) {
			t.Error("an expired result, which holds nothing from the tool, restricts fetches")
		}
	})

	t.Run("model_written_url", func(t *testing.T) {
		messages := append([]aigateway.Message{system, user,
			{Role: "assistant", Content: "I will fetch https://attacker.example/?q=secret next."}},
			toolExchange("1", "web_search", "results")...)
		if allows(t, messages, "https://attacker.example/?q=secret") {
			t.Error("a URL the model wrote out itself counts as seen")
		}
	})

	t.Run("image_message", func(t *testing.T) {
		withImage := aigateway.Message{Role: "user", Content: []aigateway.ContentPart{
			{Type: "text", Text: "Bob: what is at https://example.org/photo-info?"},
			{Type: "image_url", ImageURL: &aigateway.ImageURL{URL: "data:image/png;base64,AAAA"}},
		}}
		messages := append([]aigateway.Message{system, withImage}, toolExchange("1", "web_search", "results")...)
		if !allows(t, messages, "https://example.org/photo-info") {
			t.Error("a URL in the text of a message with an image doesn't count as seen")
		}
	})
}
