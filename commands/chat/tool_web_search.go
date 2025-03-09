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

	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

// WebSearchToolDefinition returns the tool definition for the web search tool
var WebSearchToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "web_search",
		Description: "Search the web for information using DuckDuckGo. Use this tool to find up-to-date information about topics, definitions, or facts that you might not know about.",
		Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {
						"type": "string",
						"description": "The search query to look up on the web. Be specific and concise for better results. Use english for best results."
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

	// Get search results from DuckDuckGo
	searchResults, err := performDuckDuckGoSearch(ctx, query)
	if err != nil {
		return "", err
	}

	return formatSearchResults(searchResults, query), nil
}

// DDGResponse represents the JSON response from the DuckDuckGo API
type DDGResponse struct {
	Abstract       string `json:"Abstract"`
	AbstractText   string `json:"AbstractText"`
	AbstractSource string `json:"AbstractSource"`
	AbstractURL    string `json:"AbstractURL"`
	Image          string `json:"Image"`
	Heading        string `json:"Heading"`
	Answer         string `json:"Answer"`
	Redirect       string `json:"Redirect"`
	AnswerType     string `json:"AnswerType"`
	Definition     string `json:"Definition"`
	DefinitionURL  string `json:"DefinitionURL"`
	RelatedTopics  []struct {
		Result     string `json:"Result"`
		FirstURL   string `json:"FirstURL"`
		Icon       string `json:"Icon"`
		Text       string `json:"Text"`
		Topics     []any  `json:"Topics,omitempty"`
		Name       string `json:"Name,omitempty"`
		Repository string `json:"Repository,omitempty"`
	} `json:"RelatedTopics"`
	Results []struct {
		Result   string `json:"Result"`
		FirstURL string `json:"FirstURL"`
		Icon     string `json:"Icon"`
		Text     string `json:"Text"`
	} `json:"Results"`
	Type   string `json:"Type"`
	Entity string `json:"Entity"`
	// Use a custom type for Infobox to handle both string and object cases
	Infobox json.RawMessage `json:"Infobox"`
	// Add meta field to capture additional information
	Meta struct {
		Attribution  interface{} `json:"attribution"`
		Blockgroup   interface{} `json:"blockgroup"`
		CreatedDate  string      `json:"created_date"`
		Description  string      `json:"description"`
		Designer     interface{} `json:"designer"`
		DevDate      string      `json:"dev_date"`
		DevMilestone string      `json:"dev_milestone"`
		Developer    []struct {
			Name string `json:"name"`
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"developer"`
		ExampleQuery    string      `json:"example_query"`
		ID              string      `json:"id"`
		IsStackexchange int         `json:"is_stackexchange"`
		JsCallbackName  string      `json:"js_callback_name"`
		LiveDate        interface{} `json:"live_date"`
		Maintainer      struct {
			Github string `json:"github"`
		} `json:"maintainer"`
		Name            string      `json:"name"`
		PerlModule      string      `json:"perl_module"`
		Producer        interface{} `json:"producer"`
		ProductionState string      `json:"production_state"`
		Repo            string      `json:"repo"`
		SignalFrom      string      `json:"signal_from"`
		SrcDomain       string      `json:"src_domain"`
		SrcID           interface{} `json:"src_id"`
		SrcName         string      `json:"src_name"`
		SrcOptions      interface{} `json:"src_options"`
		SrcURL          string      `json:"src_url"`
		Status          interface{} `json:"status"`
		Tab             string      `json:"tab"`
		Topic           []string    `json:"topic"`
		Unsafe          interface{} `json:"unsafe"`
	} `json:"meta"`
}

// InfoboxContent represents the content of an infobox
type InfoboxContent struct {
	DataType  string `json:"data_type"`
	Label     string `json:"label"`
	Value     string `json:"value"`
	WikiOrder int    `json:"wiki_order"`
}

// InfoboxStruct represents the structure of an infobox
type InfoboxStruct struct {
	Content []InfoboxContent `json:"content"`
}

// performDuckDuckGoSearch fetches search results from the DuckDuckGo API
func performDuckDuckGoSearch(ctx context.Context, query string) (*DDGResponse, error) {
	// Construct the URL for the DuckDuckGo API
	baseURL := "https://api.duckduckgo.com/"

	// Create URL with query parameters
	params := url.Values{}
	params.Add("q", query)
	params.Add("format", "json")
	params.Add("no_html", "1")
	params.Add("skip_disambig", "1")

	requestURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	log.Debug().Ctx(ctx).Str("url", requestURL).Str("query", query).Msg("Fetching web search results")

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", requestURL).Msg("Failed to create web search API request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set a user agent to avoid being blocked
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
	var searchResults DDGResponse
	if err := json.Unmarshal(body, &searchResults); err != nil {
		// Log the error and response for debugging
		log.Error().Ctx(ctx).Err(err).Str("response", string(body)).Msg("Failed to parse web search API response")

		// Try to create a minimal response with the raw data
		// This allows us to return something even if the full parsing fails
		return &DDGResponse{
			AbstractText: fmt.Sprintf("Error parsing search results for query: %s", query),
			RelatedTopics: []struct {
				Result     string `json:"Result"`
				FirstURL   string `json:"FirstURL"`
				Icon       string `json:"Icon"`
				Text       string `json:"Text"`
				Topics     []any  `json:"Topics,omitempty"`
				Name       string `json:"Name,omitempty"`
				Repository string `json:"Repository,omitempty"`
			}{},
			Results: []struct {
				Result   string `json:"Result"`
				FirstURL string `json:"FirstURL"`
				Icon     string `json:"Icon"`
				Text     string `json:"Text"`
			}{},
		}, fmt.Errorf("failed to parse search results: %w", err)
	}

	// Check if we got any meaningful results
	if searchResults.AbstractText == "" &&
		searchResults.Answer == "" &&
		searchResults.Definition == "" &&
		len(searchResults.RelatedTopics) == 0 &&
		len(searchResults.Results) == 0 {
		log.Info().Ctx(ctx).Str("query", query).Msg("No meaningful results found for web search query")
	}

	return &searchResults, nil
}

// formatSearchResults formats the search results into a readable string
func formatSearchResults(data *DDGResponse, query string) string {
	var result strings.Builder

	result.WriteString(fmt.Sprintf("## Web Search Results for: %s\n\n", query))

	// Add the abstract if available
	if data.AbstractText != "" {
		result.WriteString(fmt.Sprintf("### %s\n", data.Heading))
		result.WriteString(data.AbstractText)
		if data.AbstractURL != "" {
			result.WriteString(fmt.Sprintf("\n\nSource: [%s](%s)\n\n", data.AbstractSource, data.AbstractURL))
		} else {
			result.WriteString("\n\n")
		}
	}

	// Add the answer if available
	if data.Answer != "" {
		result.WriteString(fmt.Sprintf("### Answer\n%s\n\n", data.Answer))
	}

	// Add the definition if available
	if data.Definition != "" {
		result.WriteString(fmt.Sprintf("### Definition\n%s\n", data.Definition))
		if data.DefinitionURL != "" {
			result.WriteString(fmt.Sprintf("Source: [%s](%s)\n\n", data.DefinitionURL, data.DefinitionURL))
		} else {
			result.WriteString("\n")
		}
	}

	// Add related topics if available
	if len(data.RelatedTopics) > 0 {
		result.WriteString("### Related Information\n")

		// Limit to 5 topics to avoid overwhelming responses
		topicLimit := 5
		if len(data.RelatedTopics) < topicLimit {
			topicLimit = len(data.RelatedTopics)
		}

		for i := 0; i < topicLimit; i++ {
			topic := data.RelatedTopics[i]
			if topic.Text != "" {
				if topic.FirstURL != "" {
					result.WriteString(fmt.Sprintf("- [%s](%s)\n", topic.Text, topic.FirstURL))
				} else {
					result.WriteString(fmt.Sprintf("- %s\n", topic.Text))
				}
			}
		}

		if len(data.RelatedTopics) > topicLimit {
			result.WriteString(fmt.Sprintf("\n*...and %d more related topics*\n", len(data.RelatedTopics)-topicLimit))
		}

		result.WriteString("\n")
	}

	// Add direct results if available
	if len(data.Results) > 0 {
		result.WriteString("### Direct Results\n")
		for _, res := range data.Results {
			if res.Text != "" && res.FirstURL != "" {
				result.WriteString(fmt.Sprintf("- [%s](%s)\n", res.Text, res.FirstURL))
			}
		}
		result.WriteString("\n")
	}

	// Add infobox content if available
	// Try to parse the infobox if it's not an empty string
	if len(data.Infobox) > 0 && string(data.Infobox) != "\"\"" {
		var infobox InfoboxStruct
		if err := json.Unmarshal(data.Infobox, &infobox); err == nil && len(infobox.Content) > 0 {
			result.WriteString("### Additional Information\n")
			for _, item := range infobox.Content {
				if item.Label != "" && item.Value != "" {
					// Clean up the value (remove HTML tags)
					value := strings.ReplaceAll(item.Value, "<", "&lt;")
					value = strings.ReplaceAll(value, ">", "&gt;")
					result.WriteString(fmt.Sprintf("- **%s**: %s\n", item.Label, value))
				}
			}
			result.WriteString("\n")
		}
	}

	// If no results were found
	if data.AbstractText == "" && data.Answer == "" && data.Definition == "" &&
		len(data.RelatedTopics) == 0 && len(data.Results) == 0 {
		result.WriteString("No specific results found for this query. Try refining your search terms.\n")
	}

	// Add source information if available
	if data.Meta.SrcName != "" {
		result.WriteString(fmt.Sprintf("\n*Data provided by %s*\n", data.Meta.SrcName))
	}

	return result.String()
}
