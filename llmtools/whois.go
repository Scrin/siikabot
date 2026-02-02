package llmtools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Scrin/siikabot/openrouter"
	"github.com/likexian/whois"
	"github.com/rs/zerolog/log"
)

// WhoisToolDefinition returns the tool definition for the whois tool
var WhoisToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "whois_lookup",
		Description: "Look up WHOIS information for a domain name or IP address. Returns registration details, nameservers, and other public registration data.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {
					"type": "string",
					"description": "The domain name or IP address to look up (e.g., example.com, 8.8.8.8)"
				}
			},
			"required": ["query"]
		}`),
	},
	Handler:          handleWhoisToolCall,
	ValidityDuration: 1 * time.Hour,
}

// handleWhoisToolCall handles whois tool calls
func handleWhoisToolCall(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Query string `json:"query"`
	}

	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received whois tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse whois tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	if args.Query == "" {
		return "", fmt.Errorf("query is required")
	}

	query := strings.TrimSpace(args.Query)

	log.Debug().Ctx(ctx).Str("query", query).Msg("Performing WHOIS lookup")

	result, err := whois.Whois(query)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("query", query).Msg("Failed to perform WHOIS lookup")
		return "", fmt.Errorf("WHOIS lookup failed: %w", err)
	}

	// Truncate if the response is too long
	const maxResponseLength = 8000
	if len(result) > maxResponseLength {
		result = result[:maxResponseLength] + "\n\n[Response truncated]"
	}

	log.Debug().Ctx(ctx).Str("query", query).Int("response_length", len(result)).Msg("WHOIS lookup completed")

	return fmt.Sprintf("**WHOIS lookup for %s**\n\n```\n%s\n```", query, result), nil
}
