package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

// ElectricityPricesToolDefinition returns the tool definition for the electricity prices tool
var ElectricityPricesToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "get_electricity_prices",
		Description: "Get detailed electricity prices in Finland for a specific date at 1 hour resolution",
		Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"date": {
						"type": "string",
						"description": "Date to fetch prices for in YYYY-MM-DD format. If not provided, defaults to today."
					}
				},
				"required": []
			}`),
	},
	Handler: handleElectricityPricesToolCall,
}

// handleElectricityPricesToolCall handles electricity prices tool calls
func handleElectricityPricesToolCall(ctx context.Context, arguments string) (string, error) {
	// Parse the arguments
	var args struct {
		Date string `json:"date"`
	}

	// Log the raw arguments for debugging
	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received electricity prices tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Parse the date or use today's date if not provided
	var date time.Time
	var err error
	if args.Date == "" {
		// Use current date
		date = time.Now()
		log.Debug().Ctx(ctx).Time("date", date).Msg("Using current date for electricity prices")
	} else {
		// Try parsing with standard format
		date, err = time.Parse("2006-01-02", args.Date)
		if err != nil {
			// Try alternative formats
			formats := []string{
				"2006-01-02",
				"02.01.2006",
				"2.1.2006",
				"January 2, 2006",
				"Jan 2, 2006",
			}

			parsed := false
			for _, format := range formats {
				if d, parseErr := time.Parse(format, args.Date); parseErr == nil {
					date = d
					parsed = true
					break
				}
			}

			if !parsed {
				log.Error().Ctx(ctx).Err(err).Str("date", args.Date).Msg("Failed to parse date, using current date")
				date = time.Now() // Fallback to today's date
			}
		}
	}

	log.Debug().Ctx(ctx).Time("date", date).Str("formatted_date", date.Format("2006-01-02")).Msg("Using date for electricity prices")

	prices, err := getElectricityPrices(ctx, date)
	if err != nil {
		return "", err
	}

	return formatElectricityPrices(prices), nil
}

// ElectricityPriceEntry represents a single electricity price entry from the API
type ElectricityPriceEntry struct {
	TimestampFinnish string `json:"aikaleima_suomi"`
	TimestampUTC     string `json:"aikaleima_utc"`
	Price            string `json:"hinta"`
}

// getElectricityPrices fetches electricity prices from the API
func getElectricityPrices(ctx context.Context, date time.Time) ([]ElectricityPriceEntry, error) {
	// Format the date as YYYY-MM-DD
	dateStr := date.Format("2006-01-02")

	// The API requires tunnit=24 and accepts a date parameter
	url := fmt.Sprintf("https://www.sahkohinta-api.fi/api/v1/halpa?tunnit=24&tulos=sarja&aikaraja=%s", dateStr)

	log.Debug().Ctx(ctx).Str("url", url).Str("date", dateStr).Msg("Fetching electricity prices")

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", url).Msg("Failed to create electricity price API request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", url).Msg("Failed to fetch electricity prices")
		return nil, fmt.Errorf("failed to fetch electricity prices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error().Ctx(ctx).Int("status_code", resp.StatusCode).Str("url", url).Msg("Electricity price API returned non-OK status")

		// Try to read the error response body for more details
		errorBody, _ := io.ReadAll(resp.Body)
		if len(errorBody) > 0 {
			log.Error().Ctx(ctx).Str("error_body", string(errorBody)).Msg("Electricity price API error details")
		}

		return nil, fmt.Errorf("API returned status code %d", resp.StatusCode)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", url).Msg("Failed to read electricity price API response")
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Log the response for debugging
	log.Debug().Ctx(ctx).Str("response", string(body)).Msg("Received electricity price API response")

	// Parse the response
	var prices []ElectricityPriceEntry
	if err := json.Unmarshal(body, &prices); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("response", string(body)).Msg("Failed to decode electricity price response")
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	log.Debug().Ctx(ctx).Int("price_count", len(prices)).Msg("Successfully fetched electricity prices")

	return prices, nil
}

// formatElectricityPrices formats the electricity prices for display
func formatElectricityPrices(prices []ElectricityPriceEntry) string {
	if len(prices) == 0 {
		return "No electricity price data available for the requested date."
	}

	// Convert prices to float for calculations
	var priceValues []float64
	for _, entry := range prices {
		price, err := strconv.ParseFloat(entry.Price, 64)
		if err == nil {
			priceValues = append(priceValues, price)
		}
	}

	if len(priceValues) == 0 {
		return "Electricity price data was received but contained no valid price values."
	}

	// Calculate statistics
	var sum float64
	min := priceValues[0]
	max := priceValues[0]
	for _, price := range priceValues {
		sum += price
		if price < min {
			min = price
		}
		if price > max {
			max = price
		}
	}
	avg := sum / float64(len(priceValues))

	// Format the output
	var sb strings.Builder
	sb.WriteString("**Electricity Prices (c/kWh)**\n\n")

	// Add statistics in a clear format for easy extraction
	sb.WriteString(fmt.Sprintf("Average: %.2f\n", avg))
	sb.WriteString(fmt.Sprintf("Min: %.2f\n", min))
	sb.WriteString(fmt.Sprintf("Max: %.2f\n\n", max))

	// Add hourly prices
	sb.WriteString("Hourly prices:\n")

	validEntries := 0

	// Sort entries by time
	var timeEntries []struct {
		Time  time.Time
		Price float64
	}

	for _, entry := range prices {
		// Parse the Finnish timestamp
		t, err := time.Parse("2006-01-02T15:04", entry.TimestampFinnish)
		if err != nil {
			continue
		}

		price, err := strconv.ParseFloat(entry.Price, 64)
		if err != nil {
			continue
		}

		timeEntries = append(timeEntries, struct {
			Time  time.Time
			Price float64
		}{t, price})
		validEntries++
	}

	// Sort by time
	sort.Slice(timeEntries, func(i, j int) bool {
		return timeEntries[i].Time.Before(timeEntries[j].Time)
	})

	// Output sorted entries
	for _, entry := range timeEntries {
		timeStr := entry.Time.Format("15:04")
		sb.WriteString(fmt.Sprintf("%s: %.2f\n", timeStr, entry.Price))
	}

	if validEntries == 0 {
		sb.WriteString("\nNo valid hourly price entries were found in the data.\n")
	}

	return sb.String()
}
