package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

// WebSearchToolDefinition returns the tool definition for the web search tool
var WebSearchToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "web_search",
		Description: "Search the web for information using Google. Use this tool to find up-to-date information about topics, definitions, or facts that you might not know about.",
		Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {
						"type": "string",
						"description": "The search query to look up on the web. Be specific and concise for better results."
					}
				},
				"required": ["query"]
			}`),
	},
	Handler: handleWebSearchToolCall,
}

// handleWebSearchToolCall handles web search tool calls
func handleWebSearchToolCall(ctx context.Context, arguments string) (string, error) {
	// Parse the arguments
	var args struct {
		Query string `json:"query"`
	}

	// Log the raw arguments for debugging
	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received web search tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	if args.Query == "" {
		return "", fmt.Errorf("search query is required")
	}

	// Sanitize and prepare the query
	query := strings.TrimSpace(args.Query)

	// Get search results from Google
	searchResults, err := performGoogleSearch(ctx, query)
	if err != nil {
		return "", err
	}

	return formatSearchResults(searchResults, query), nil
}

// GoogleSearchResponse represents the JSON response from the Google Custom Search API
type GoogleSearchResponse struct {
	Kind string `json:"kind"`
	URL  struct {
		Type     string `json:"type"`
		Template string `json:"template"`
	} `json:"url"`
	Queries struct {
		Request  []GoogleSearchRequest `json:"request"`
		NextPage []GoogleSearchRequest `json:"nextPage,omitempty"`
	} `json:"queries"`
	Context struct {
		Title string `json:"title"`
	} `json:"context"`
	SearchInformation struct {
		SearchTime            float64 `json:"searchTime"`
		FormattedSearchTime   string  `json:"formattedSearchTime"`
		TotalResults          string  `json:"totalResults"`
		FormattedTotalResults string  `json:"formattedTotalResults"`
	} `json:"searchInformation"`
	Items []GoogleSearchItem `json:"items"`
}

// GoogleSearchRequest represents a request in the Google Custom Search API response
type GoogleSearchRequest struct {
	Title          string `json:"title"`
	TotalResults   string `json:"totalResults"`
	SearchTerms    string `json:"searchTerms"`
	Count          int    `json:"count"`
	StartIndex     int    `json:"startIndex"`
	InputEncoding  string `json:"inputEncoding"`
	OutputEncoding string `json:"outputEncoding"`
	Safe           string `json:"safe"`
	Cx             string `json:"cx"`
}

// GoogleSearchItem represents an item in the Google Custom Search API response
type GoogleSearchItem struct {
	Kind             string `json:"kind"`
	Title            string `json:"title"`
	HTMLTitle        string `json:"htmlTitle"`
	Link             string `json:"link"`
	DisplayLink      string `json:"displayLink"`
	Snippet          string `json:"snippet"`
	HTMLSnippet      string `json:"htmlSnippet"`
	CacheID          string `json:"cacheId,omitempty"`
	FormattedURL     string `json:"formattedUrl"`
	HTMLFormattedURL string `json:"htmlFormattedUrl"`
	Pagemap          struct {
		CseThumbnail []struct {
			Src    string `json:"src"`
			Width  string `json:"width"`
			Height string `json:"height"`
		} `json:"cse_thumbnail,omitempty"`
		Metatags []map[string]string `json:"metatags"`
		CseImage []struct {
			Src string `json:"src"`
		} `json:"cse_image,omitempty"`
	} `json:"pagemap,omitempty"`
}

// performGoogleSearch fetches search results from the Google Custom Search API
func performGoogleSearch(ctx context.Context, query string) (*GoogleSearchResponse, error) {
	// Construct the URL for the Google Custom Search API
	baseURL := "https://www.googleapis.com/customsearch/v1"

	// Create URL with query parameters
	params := url.Values{}
	params.Add("q", query)
	params.Add("key", config.GoogleAPIKey)
	params.Add("cx", config.GoogleSearchEngineID)
	params.Add("num", "10") // Number of results to return (max 10)

	requestURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	log.Debug().Ctx(ctx).Str("url", requestURL).Str("query", query).Msg("Fetching web search results")

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", requestURL).Msg("Failed to create web search API request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set a user agent
	req.Header.Set("User-Agent", "SiikabotWebSearch/1.0")

	// Execute the request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", requestURL).Msg("Failed to fetch web search results")
		return nil, fmt.Errorf("failed to fetch web search results: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		log.Error().Ctx(ctx).Int("status_code", resp.StatusCode).Str("url", requestURL).Msg("Web search API returned non-OK status")

		// Try to read the error response body for more details
		errorBody, _ := io.ReadAll(resp.Body)
		if len(errorBody) > 0 {
			log.Error().Ctx(ctx).Str("error_body", string(errorBody)).Msg("Web search API error details")
		}

		return nil, fmt.Errorf("API returned status code %d", resp.StatusCode)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", requestURL).Msg("Failed to read web search API response")
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Log a sample of the response for debugging
	if len(body) > 500 {
		log.Debug().Ctx(ctx).Str("response_sample", string(body[:500])+"...").Msg("Received web search API response (sample)")
	} else {
		log.Debug().Ctx(ctx).Str("response", string(body)).Msg("Received web search API response")
	}

	// Parse the JSON response
	var searchResults GoogleSearchResponse
	if err := json.Unmarshal(body, &searchResults); err != nil {
		// Log the error and response for debugging
		log.Error().Ctx(ctx).Err(err).Str("response", string(body)).Msg("Failed to parse web search API response")

		// Try to create a minimal response with the raw data
		return &GoogleSearchResponse{
			SearchInformation: struct {
				SearchTime            float64 `json:"searchTime"`
				FormattedSearchTime   string  `json:"formattedSearchTime"`
				TotalResults          string  `json:"totalResults"`
				FormattedTotalResults string  `json:"formattedTotalResults"`
			}{
				TotalResults: "0",
			},
			Items: []GoogleSearchItem{},
		}, fmt.Errorf("failed to parse search results: %w", err)
	}

	// Check if we got any meaningful results
	if len(searchResults.Items) == 0 {
		log.Info().Ctx(ctx).Str("query", query).Msg("No results found for web search query")
	}

	return &searchResults, nil
}

// formatSearchResults formats the search results into a readable string
func formatSearchResults(data *GoogleSearchResponse, query string) string {
	var result strings.Builder

	result.WriteString(fmt.Sprintf("## Web Search Results for: %s\n\n", query))

	// Add search information
	if data.SearchInformation.TotalResults != "0" {
		result.WriteString(fmt.Sprintf("Found about %s results (%s seconds)\n\n",
			data.SearchInformation.FormattedTotalResults,
			data.SearchInformation.FormattedSearchTime))
	}

	// If no results were found
	if len(data.Items) == 0 {
		result.WriteString("No results found for this query. Try refining your search terms.\n")
		return result.String()
	}

	// Add search results
	for i, item := range data.Items {
		// Limit to 10 results
		if i >= 10 {
			break
		}

		// Clean up the snippet (remove HTML tags)
		snippet := strings.ReplaceAll(item.Snippet, "<b>", "**")
		snippet = strings.ReplaceAll(snippet, "</b>", "**")
		snippet = strings.ReplaceAll(snippet, "<br>", "\n")
		snippet = strings.ReplaceAll(snippet, "&nbsp;", " ")
		snippet = strings.ReplaceAll(snippet, "&quot;", "\"")
		snippet = strings.ReplaceAll(snippet, "&amp;", "&")
		snippet = strings.ReplaceAll(snippet, "&lt;", "<")
		snippet = strings.ReplaceAll(snippet, "&gt;", ">")

		result.WriteString(fmt.Sprintf("### [%s](%s)\n", item.Title, item.Link))
		result.WriteString(fmt.Sprintf("*%s*\n\n", item.DisplayLink))
		result.WriteString(fmt.Sprintf("%s\n\n", snippet))
	}

	result.WriteString("\n*Data provided by Google Custom Search*\n")

	return result.String()
}
