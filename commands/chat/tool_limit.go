package chat

import (
	"slices"
	"strings"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/db"
)

// toolLimitNotice tells the model that the turn has used up its tool iterations, in the last
// request of such a turn. Without it the model only finds out from tool_choice, which not every
// provider honours, and goes on asking for tools the turn will never run.
//
// It goes in a system message of its own rather than in a tool result: tool results hold text from
// the web, which the model is told never to take instructions from.
const toolLimitNotice = "You have used every round of tool calls this turn allows, so no more tools can be called. " +
	"Answer now from what you have found so far, and say what you could not find out."

// toolLimitRequest is the last request of a turn that has used up its tool iterations, under the
// same limits as the rest of the turn's requests.
//
// It still offers the tools, with tool_choice none. Leaving them out used to make the model write
// its tool calls as text, which was then posted to the room as the answer: without tools in the
// request, the provider neither tells the model about them nor parses the calls it makes. Offering
// them also keeps the request on the same cached prefix as the rest of the turn.
func toolLimitRequest(cfg db.ChatConfig, messages []aigateway.Message, tools []aigateway.ToolDefinition, metadata map[string]string) aigateway.ChatRequest {
	req := chatRequest(cfg, append(slices.Clip(messages), aigateway.Message{Role: "system", Content: toolLimitNotice}),
		tools, metadata)
	req.ToolChoice = "none"
	return req
}

// asksOnlyForTools says whether a response is nothing but tool calls, with no answer written
// alongside them
func asksOnlyForTools(resp *aigateway.ChatResponse) bool {
	message := resp.Choices[0].Message
	text, _ := message.Content.(string)
	return len(message.ToolCalls) > 0 && strings.TrimSpace(text) == ""
}
