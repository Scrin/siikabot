package llmtools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

// WebToolDefinition returns the tool definition for the web content fetching tool
var WebToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "get_web_content",
		Description: "Fetch the content of a web page via HTTP GET request (max 10kB by default)",
		Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"url": {
						"type": "string",
						"description": "The URL to fetch content from. Must be a valid HTTP/HTTPS URL."
					}
				},
				"required": ["url"]
			}`),
	},
	Handler:          handleWebToolCall,
	ValidityDuration: 1 * time.Minute,
}

// Default maximum size of response body to read (10kB)
const DefaultMaxWebResponseSize = 10 * 1024

// Maximum number of redirects to follow
const maxRedirects = 5

// handleWebToolCall handles web content fetching tool calls
func handleWebToolCall(ctx context.Context, arguments string) (string, error) {
	// Parse the arguments
	var args struct {
		URL string `json:"url"`
	}

	// Log the raw arguments for debugging
	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received web content tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	if args.URL == "" {
		return "", fmt.Errorf("URL is required")
	}

	// Basic URL validation
	if !strings.HasPrefix(args.URL, "http://") && !strings.HasPrefix(args.URL, "https://") {
		return "", fmt.Errorf("invalid URL: must start with http:// or https://")
	}

	// Get room ID from context
	roomID, ok := ctx.Value("room_id").(string)
	if !ok || roomID == "" {
		return "", fmt.Errorf("room ID not found in context")
	}

	// Get configured max size for the room, or use default if not set
	maxSize := DefaultMaxWebResponseSize
	if configuredSize, err := db.GetRoomChatMaxWebContentSize(ctx, roomID); err != nil && err != sql.ErrNoRows {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to get max web content size, using default")
	} else if configuredSize != nil {
		maxSize = *configuredSize
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", args.URL, nil)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", args.URL).Msg("Failed to create web request")
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set User-Agent header
	req.Header.Set("User-Agent", "Siikabot-Web-Tool/1.0")

	// Create HTTP client with redirect handling
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Check number of redirects
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}

			// Log redirect
			log.Debug().Ctx(ctx).
				Str("from", via[len(via)-1].URL.String()).
				Str("to", req.URL.String()).
				Int("redirect_count", len(via)).
				Msg("Following redirect")

			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", args.URL).Msg("Failed to fetch web content")
		return "", fmt.Errorf("failed to fetch web content: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body with size limit
	limitedReader := io.LimitReader(resp.Body, int64(maxSize))
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", args.URL).Msg("Failed to read web response")
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Error().Ctx(ctx).Int("status_code", resp.StatusCode).Str("url", args.URL).Msg("Web request returned non-OK status")
		return "", fmt.Errorf("web request returned status code %d", resp.StatusCode)
	}

	// Check if response was truncated
	contentLength := resp.ContentLength
	if contentLength > int64(maxSize) {
		log.Warn().Ctx(ctx).
			Str("url", args.URL).
			Int64("content_length", contentLength).
			Int("max_size", maxSize).
			Msg("Response truncated due to size limit")
	}

	// Log success with content length
	log.Debug().Ctx(ctx).
		Str("url", args.URL).
		Int("content_length", len(body)).
		Int("max_size", maxSize).
		Msg("Successfully fetched web content")

	// Return the content
	return string(body), nil
}
