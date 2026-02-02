package llmtools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

// WikipediaToolDefinition returns the tool definition for the Wikipedia tool
var WikipediaToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "wikipedia_summary",
		Description: "Get a summary of a Wikipedia article. Returns the article title, a brief summary, and the URL. Useful for quick factual lookups.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {
					"type": "string",
					"description": "The topic or article title to look up (e.g., 'Albert Einstein', 'Python programming language', 'Eiffel Tower')"
				},
				"language": {
					"type": "string",
					"description": "Wikipedia language code (default: 'en'). Examples: 'fi' for Finnish, 'de' for German, 'sv' for Swedish, 'ja' for Japanese."
				}
			},
			"required": ["query"]
		}`),
	},
	Handler:          handleWikipediaToolCall,
	ValidityDuration: 1 * time.Hour,
}

// wikipediaSummaryResponse represents the Wikipedia REST API summary response
type wikipediaSummaryResponse struct {
	Type         string `json:"type"`
	Title        string `json:"title"`
	DisplayTitle string `json:"displaytitle"`
	Extract      string `json:"extract"`
	Description  string `json:"description"`
	ContentURLs  struct {
		Desktop struct {
			Page string `json:"page"`
		} `json:"desktop"`
	} `json:"content_urls"`
}

// handleWikipediaToolCall handles Wikipedia summary tool calls
func handleWikipediaToolCall(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Query    string `json:"query"`
		Language string `json:"language"`
	}

	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received Wikipedia tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse Wikipedia tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	if args.Query == "" {
		return "", fmt.Errorf("query is required")
	}

	// Default to English Wikipedia
	lang := strings.ToLower(strings.TrimSpace(args.Language))
	if lang == "" {
		lang = "en"
	}

	// Validate language code (basic check)
	if len(lang) < 2 || len(lang) > 3 {
		return "", fmt.Errorf("invalid language code '%s', use 2-3 letter codes like 'en', 'fi', 'de'", lang)
	}

	log.Debug().Ctx(ctx).Str("query", args.Query).Str("language", lang).Msg("Fetching Wikipedia summary")

	summary, err := fetchWikipediaSummary(ctx, args.Query, lang)
	if err != nil {
		return "", err
	}

	return formatWikipediaSummary(summary, lang), nil
}

// fetchWikipediaSummary fetches a summary from the Wikipedia REST API
func fetchWikipediaSummary(ctx context.Context, query, lang string) (*wikipediaSummaryResponse, error) {
	// URL-encode the query (replace spaces with underscores first, as Wikipedia prefers)
	title := strings.ReplaceAll(strings.TrimSpace(query), " ", "_")
	encodedTitle := url.PathEscape(title)

	apiURL := fmt.Sprintf("https://%s.wikipedia.org/api/rest_v1/page/summary/%s", lang, encodedTitle)

	log.Debug().Ctx(ctx).Str("url", apiURL).Msg("Fetching Wikipedia summary")

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Msg("Failed to create Wikipedia API request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set a user agent as Wikipedia API recommends
	req.Header.Set("User-Agent", "SiikaBot/1.0 (Matrix bot; contact: github.com/Scrin/siikabot)")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Msg("Failed to fetch Wikipedia summary")
		return nil, fmt.Errorf("failed to fetch Wikipedia summary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		log.Debug().Ctx(ctx).Str("query", query).Str("language", lang).Msg("Wikipedia article not found")
		return nil, fmt.Errorf("no Wikipedia article found for '%s' (language: %s). Try a different search term or check spelling", query, lang)
	}

	if resp.StatusCode != http.StatusOK {
		log.Error().Ctx(ctx).Int("status_code", resp.StatusCode).Str("url", apiURL).Msg("Wikipedia API returned non-OK status")
		return nil, fmt.Errorf("Wikipedia API returned status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Msg("Failed to read Wikipedia API response")
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var summary wikipediaSummaryResponse
	if err := json.Unmarshal(body, &summary); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to parse Wikipedia JSON response")
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Check if it's a disambiguation page
	if summary.Type == "disambiguation" {
		return nil, fmt.Errorf("'%s' is a disambiguation page with multiple meanings. Please be more specific (e.g., '%s (band)' or '%s (film)')", query, query, query)
	}

	log.Debug().Ctx(ctx).Str("title", summary.Title).Int("extract_length", len(summary.Extract)).Msg("Successfully fetched Wikipedia summary")

	return &summary, nil
}

// formatWikipediaSummary formats the Wikipedia summary for display
func formatWikipediaSummary(summary *wikipediaSummaryResponse, lang string) string {
	if summary == nil {
		return "No summary available."
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("**%s**\n\n", summary.Title))

	if summary.Description != "" {
		sb.WriteString(fmt.Sprintf("*%s*\n\n", summary.Description))
	}

	if summary.Extract != "" {
		sb.WriteString(summary.Extract)
		sb.WriteString("\n\n")
	}

	if summary.ContentURLs.Desktop.Page != "" {
		sb.WriteString(fmt.Sprintf("Read more: %s\n", summary.ContentURLs.Desktop.Page))
	}

	return sb.String()
}
