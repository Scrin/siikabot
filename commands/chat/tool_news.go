package chat

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

// NewsToolDefinition returns the tool definition for the news headlines tool
var NewsToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "get_news_headlines",
		Description: "Get the latest news headlines from Yle (Finnish Broadcasting Company)",
		Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
	},
	Handler:          handleNewsToolCall,
	ValidityDuration: 15 * time.Minute,
}

// handleNewsToolCall handles news headlines tool calls
func handleNewsToolCall(ctx context.Context, arguments string) (string, error) {
	// Log the raw arguments for debugging
	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received news headlines tool call")

	// Get news headlines from Yle RSS feed
	headlines, err := getNewsHeadlines(ctx)
	if err != nil {
		return "", err
	}

	return formatNewsHeadlines(headlines), nil
}

// RSSFeed represents the RSS feed structure
type RSSFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Title       string    `xml:"title"`
		Description string    `xml:"description"`
		Link        string    `xml:"link"`
		Items       []RSSItem `xml:"item"`
	} `xml:"channel"`
}

// RSSItem represents a single item in the RSS feed
type RSSItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	Description string   `xml:"description"`
	PubDate     string   `xml:"pubDate"`
	Categories  []string `xml:"category"`
}

// NewsHeadline represents a processed news headline
type NewsHeadline struct {
	Title      string
	Link       string
	Categories []string
	PubDate    time.Time
}

// getNewsHeadlines fetches news headlines from the Yle RSS feed
func getNewsHeadlines(ctx context.Context) ([]NewsHeadline, error) {
	// Yle RSS feed URL
	feedURL := "https://feeds.yle.fi/uutiset/v1/majorHeadlines/YLE_UUTISET.rss"

	log.Debug().Ctx(ctx).Str("url", feedURL).Msg("Fetching news headlines")

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", feedURL).Msg("Failed to create news API request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute the request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", feedURL).Msg("Failed to fetch news headlines")
		return nil, fmt.Errorf("failed to fetch news headlines: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error().Ctx(ctx).Int("status_code", resp.StatusCode).Str("url", feedURL).Msg("News API returned non-OK status")
		return nil, fmt.Errorf("API returned status code %d", resp.StatusCode)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", feedURL).Msg("Failed to read news API response")
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse the XML
	var feed RSSFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to parse RSS feed")
		return nil, fmt.Errorf("failed to parse RSS feed: %w", err)
	}

	// Process all headlines
	headlines := make([]NewsHeadline, 0, len(feed.Channel.Items))
	for _, item := range feed.Channel.Items {
		// Parse the publication date
		pubDate, err := time.Parse(time.RFC1123, item.PubDate)
		if err != nil {
			// Try alternative format if standard RFC1123 fails
			pubDate, err = time.Parse(time.RFC1123Z, item.PubDate)
			if err != nil {
				// Use current time as fallback
				pubDate = time.Now()
			}
		}

		headlines = append(headlines, NewsHeadline{
			Title:      item.Title,
			Link:       item.Link,
			Categories: item.Categories,
			PubDate:    pubDate,
		})
	}

	return headlines, nil
}

// formatNewsHeadlines formats the news headlines into a readable string
func formatNewsHeadlines(headlines []NewsHeadline) string {
	if len(headlines) == 0 {
		return "No news headlines found."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Latest news headlines from Yle (%d):\n\n", len(headlines)))

	for i, headline := range headlines {
		// Format the publication date
		formattedDate := headline.PubDate.Format("2006-01-02 15:04")

		// Add category if available
		category := ""
		if len(headline.Categories) > 0 {
			category = fmt.Sprintf(" [%s]", strings.Join(headline.Categories, ", "))
		}

		sb.WriteString(fmt.Sprintf("%d. %s%s\n", i+1, headline.Title, category))
		sb.WriteString(fmt.Sprintf("   Published: %s\n", formattedDate))
		sb.WriteString(fmt.Sprintf("   Link: %s\n", headline.Link))

		if i < len(headlines)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
