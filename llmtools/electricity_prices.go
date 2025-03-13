package llmtools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
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
						"description": "Date to fetch prices for in YYYY-MM-DD format."
					}
				},
				"required": ["date"]
			}`),
	},
	Handler: handleElectricityPricesToolCall,
}

// Cache for electricity prices
var (
	electricityPriceCache      = make(map[string][]PriceEntry)
	electricityPriceCacheMutex sync.RWMutex
)

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

// NordPoolResponse represents the response from the Nord Pool API
type NordPoolResponse struct {
	DeliveryDateCET      string                `json:"deliveryDateCET"`
	Version              int                   `json:"version"`
	UpdatedAt            string                `json:"updatedAt"`
	DeliveryAreas        []string              `json:"deliveryAreas"`
	Market               string                `json:"market"`
	MultiAreaEntries     []MultiAreaEntry      `json:"multiAreaEntries"`
	BlockPriceAggregates []BlockPriceAggregate `json:"blockPriceAggregates"`
	Currency             string                `json:"currency"`
	ExchangeRate         float64               `json:"exchangeRate"`
	AreaStates           []AreaState           `json:"areaStates"`
	AreaAverages         []AreaAverage         `json:"areaAverages"`
}

// MultiAreaEntry represents a single price entry for multiple areas
type MultiAreaEntry struct {
	DeliveryStart string             `json:"deliveryStart"`
	DeliveryEnd   string             `json:"deliveryEnd"`
	EntryPerArea  map[string]float64 `json:"entryPerArea"`
}

// BlockPriceAggregate represents aggregated prices for a specific time block
type BlockPriceAggregate struct {
	BlockName           string                `json:"blockName"`
	DeliveryStart       string                `json:"deliveryStart"`
	DeliveryEnd         string                `json:"deliveryEnd"`
	AveragePricePerArea map[string]BlockStats `json:"averagePricePerArea"`
}

// BlockStats represents statistics for a price block
type BlockStats struct {
	Average float64 `json:"average"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
}

// AreaState represents the state of prices for an area
type AreaState struct {
	State string   `json:"state"`
	Areas []string `json:"areas"`
}

// AreaAverage represents the average price for an area
type AreaAverage struct {
	AreaCode string  `json:"areaCode"`
	Price    float64 `json:"price"`
}

// PriceEntry represents a processed price entry for internal use
type PriceEntry struct {
	Time  time.Time
	Price float64
}

// getElectricityPrices fetches electricity prices from the Nord Pool API
func getElectricityPrices(ctx context.Context, date time.Time) ([]PriceEntry, error) {
	// Format the date as YYYY-MM-DD
	dateStr := date.Format("2006-01-02")

	// Check cache first
	electricityPriceCacheMutex.RLock()
	cachedPrices, found := electricityPriceCache[dateStr]
	electricityPriceCacheMutex.RUnlock()

	if found {
		log.Debug().Ctx(ctx).Str("date", dateStr).Int("price_count", len(cachedPrices)).Msg("Using cached electricity prices")
		return cachedPrices, nil
	}

	// Construct the Nord Pool API URL
	url := fmt.Sprintf("https://dataportal-api.nordpoolgroup.com/api/DayAheadPrices?currency=EUR&market=DayAhead&deliveryArea=FI&date=%s", dateStr)

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
	var nordPoolResponse NordPoolResponse
	if err := json.Unmarshal(body, &nordPoolResponse); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("response", string(body)).Msg("Failed to decode electricity price response")
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Process the response into our internal format
	var prices []PriceEntry
	for _, entry := range nordPoolResponse.MultiAreaEntries {
		// Parse the delivery start time
		startTime, err := time.Parse(time.RFC3339, entry.DeliveryStart)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Str("time_string", entry.DeliveryStart).Msg("Failed to parse delivery start time")
			continue
		}

		// Get the price for Finland (FI)
		price, ok := entry.EntryPerArea["FI"]
		if !ok {
			log.Error().Ctx(ctx).Str("delivery_start", entry.DeliveryStart).Msg("No price for Finland in entry")
			continue
		}

		// Convert price from EUR/MWh to cents/kWh
		priceInCentsPerKWh := price / 10

		prices = append(prices, PriceEntry{
			Time:  startTime,
			Price: priceInCentsPerKWh,
		})
	}

	// Sort prices by time
	sort.Slice(prices, func(i, j int) bool {
		return prices[i].Time.Before(prices[j].Time)
	})

	log.Debug().Ctx(ctx).Int("price_count", len(prices)).Msg("Successfully fetched electricity prices")

	// Store in cache
	if len(prices) > 0 {
		electricityPriceCacheMutex.Lock()
		electricityPriceCache[dateStr] = prices
		electricityPriceCacheMutex.Unlock()
		log.Debug().Ctx(ctx).Str("date", dateStr).Int("price_count", len(prices)).Msg("Cached electricity prices")
	}

	return prices, nil
}

// formatElectricityPrices formats the electricity prices for display
func formatElectricityPrices(prices []PriceEntry) string {
	if len(prices) == 0 {
		return "No electricity price data available for the requested date."
	}

	// Calculate statistics
	var sum float64
	min := prices[0].Price
	max := prices[0].Price
	for _, entry := range prices {
		sum += entry.Price
		if entry.Price < min {
			min = entry.Price
		}
		if entry.Price > max {
			max = entry.Price
		}
	}
	avg := sum / float64(len(prices))

	// Format the output
	var sb strings.Builder
	sb.WriteString("**Electricity Prices (c/kWh)**\n\n")

	// Add statistics in a clear format for easy extraction
	sb.WriteString(fmt.Sprintf("Average: %.2f\n", avg))
	sb.WriteString(fmt.Sprintf("Min: %.2f\n", min))
	sb.WriteString(fmt.Sprintf("Max: %.2f\n\n", max))

	// Add hourly prices
	sb.WriteString("Hourly prices:\n")

	// Convert to Finnish time zone for display
	finlandLocation, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		// Fallback to UTC if timezone loading fails
		finlandLocation = time.UTC
	}

	// Output sorted entries
	for _, entry := range prices {
		// Convert to Finnish time
		localTime := entry.Time.In(finlandLocation)
		timeStr := localTime.Format("15:04")
		sb.WriteString(fmt.Sprintf("%s: %.2f\n", timeStr, entry.Price))
	}

	return sb.String()
}
