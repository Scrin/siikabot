package llmtools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/webfetch"
	"github.com/rs/zerolog/log"
)

// WebToolDefinition returns the tool definition for the web content fetching tool
var WebToolDefinition = aigateway.ToolDefinition{
	Type: "function",
	Function: aigateway.FunctionSchema{
		Name: "get_web_content",
		Description: "Read a web page, plain text, JSON, XML or a PDF. For a web page, you get its main content as " +
			"markdown, with its title, site, author and publish date when known. A long page comes in parts: the end " +
			"of each part says which offset to call again with to read on. " +
			"Use URLs from the conversation or from tool results such as web_search. Don't guess URLs: search instead. " +
			"If a page can't be read, the result says why and what to do instead. A site that blocked the bot, or a " +
			"page that doesn't exist, won't change on a retry. The tool can't run JavaScript, so pages that build " +
			"their content in the browser come back nearly empty. Reddit can't be read at all.",
		Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"url": {
						"type": "string",
						"description": "The http:// or https:// URL to read"
					},
					"offset": {
						"type": "integer",
						"description": "Where to continue reading a long page from, as given at the end of the previous part. Leave out to read from the start."
					}
				},
				"required": ["url"]
			}`),
	},
	Handler:          handleWebToolCall,
	ValidityDuration: 10 * time.Minute,
}

// handleWebToolCall handles web content fetching tool calls. A page that can't be read isn't an error: the
// model gets an explanation of what went wrong instead.
func handleWebToolCall(ctx context.Context, arguments string) (string, error) {
	var args struct {
		URL    string `json:"url"`
		Offset int    `json:"offset"`
	}

	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received web content tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}
	if args.URL == "" {
		return "", fmt.Errorf("URL is required")
	}

	// The size of a part is in the chat configuration, which has no fallback values to use instead
	chatConfig, err := db.GetChatConfig(ctx)
	if err != nil {
		// Already logged
		return "", fmt.Errorf("failed to load chat configuration: %w", err)
	}

	result := webfetch.Fetch(ctx, args.URL)
	return webfetch.Render(result, args.Offset, chatConfig.MaxWebContentSize), nil
}
