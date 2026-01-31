package llmtools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/openrouter"
	strip "github.com/grokify/html-strip-tags-go"
	"github.com/rs/zerolog/log"
)

// WebToolDefinition returns the tool definition for the web content fetching tool
var WebToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "get_web_content",
		Description: "Fetch the content of a web page. HTML is converted to markdown for readability. (max 10kB output by default)",
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

// Multiplier for raw HTML pre-limit (prevents memory issues on huge pages)
const maxRawHTMLMultiplier = 10

// isHTMLContentType checks if the content type indicates HTML
func isHTMLContentType(contentType string) bool {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	return mediaType == "text/html" || mediaType == "application/xhtml+xml"
}

// isTextContentType checks if the content type indicates text
func isTextContentType(contentType string) bool {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	return strings.HasPrefix(mediaType, "text/")
}

// formatNonHTMLContent wraps non-HTML text content appropriately
func formatNonHTMLContent(content, contentType string) string {
	mediaType, _, _ := mime.ParseMediaType(contentType)

	switch mediaType {
	case "application/json":
		return "```json\n" + content + "\n```"
	case "application/xml", "text/xml":
		return "```xml\n" + content + "\n```"
	default:
		return content
	}
}

// convertHTMLToMarkdown converts HTML content to markdown
func convertHTMLToMarkdown(ctx context.Context, htmlContent string, baseURL string) (string, error) {
	// Parse the base URL to extract domain for relative link resolution
	parsedURL, err := url.Parse(baseURL)
	var domain string
	if err == nil && parsedURL.Host != "" {
		domain = parsedURL.Scheme + "://" + parsedURL.Host
	}

	// Create converter with plugins
	conv := converter.NewConverter(
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
			table.NewTablePlugin(),
		),
	)

	// Convert with domain option if available
	var markdown string
	if domain != "" {
		markdown, err = conv.ConvertString(htmlContent, converter.WithDomain(domain))
	} else {
		markdown, err = conv.ConvertString(htmlContent)
	}

	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to convert HTML to markdown")
		return "", err
	}

	return markdown, nil
}

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

	if resp.StatusCode != http.StatusOK {
		log.Error().Ctx(ctx).Int("status_code", resp.StatusCode).Str("url", args.URL).Msg("Web request returned non-OK status")
		return "", fmt.Errorf("web request returned status code %d", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")

	// Reject binary content types
	if !isTextContentType(contentType) && !isHTMLContentType(contentType) &&
		contentType != "application/json" && contentType != "application/xml" {
		mediaType, _, _ := mime.ParseMediaType(contentType)
		log.Warn().Ctx(ctx).Str("url", args.URL).Str("content_type", mediaType).Msg("Cannot process binary content")
		return "", fmt.Errorf("cannot process binary content type: %s", mediaType)
	}

	// Use pre-limit for raw content (prevents memory issues on huge pages)
	preLimit := int64(maxSize * maxRawHTMLMultiplier)
	limitedReader := io.LimitReader(resp.Body, preLimit)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", args.URL).Msg("Failed to read web response")
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	rawSize := len(body)
	var result string

	// Process content based on type
	if isHTMLContentType(contentType) {
		// Convert HTML to markdown
		markdown, err := convertHTMLToMarkdown(ctx, string(body), args.URL)
		if err != nil {
			// Fallback to basic tag stripping
			log.Warn().Ctx(ctx).Err(err).Str("url", args.URL).Msg("HTML to markdown conversion failed, falling back to tag stripping")
			result = strip.StripTags(string(body))
		} else {
			result = markdown
		}
	} else {
		// Format non-HTML content appropriately
		result = formatNonHTMLContent(string(body), contentType)
	}

	// Apply final size limit
	truncated := false
	if len(result) > maxSize {
		result = result[:maxSize]
		truncated = true
		result += "\n\n[Content truncated]"
	}

	// Log success with size information
	log.Debug().Ctx(ctx).
		Str("url", args.URL).
		Str("content_type", contentType).
		Int("raw_size", rawSize).
		Int("processed_size", len(result)).
		Int("max_size", maxSize).
		Bool("truncated", truncated).
		Msg("Successfully fetched and processed web content")

	return result, nil
}
