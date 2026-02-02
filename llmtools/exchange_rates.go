package llmtools

import (
	"context"
	"encoding/json"
	"encoding/xml"
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

// ExchangeRatesToolDefinition returns the tool definition for the exchange rates tool
var ExchangeRatesToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "get_exchange_rates",
		Description: "Get current EUR exchange rates from the European Central Bank. Returns how much of a target currency you get for 1 EUR. Rates are updated daily around 16:00 CET.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"currency": {
					"type": "string",
					"description": "The target currency code to get the exchange rate for (e.g., USD, GBP, SEK, JPY). If not specified, returns all available rates."
				}
			}
		}`),
	},
	Handler:          handleExchangeRatesToolCall,
	ValidityDuration: 1 * time.Hour,
}

// ECB XML response structures
type ecbEnvelope struct {
	XMLName xml.Name  `xml:"Envelope"`
	Cube    ecbCube   `xml:"Cube>Cube"`
}

type ecbCube struct {
	Time  string         `xml:"time,attr"`
	Rates []ecbRateEntry `xml:"Cube"`
}

type ecbRateEntry struct {
	Currency string  `xml:"currency,attr"`
	Rate     float64 `xml:"rate,attr"`
}

// Cache for exchange rates
var (
	exchangeRatesCache      *ecbCube
	exchangeRatesCacheMutex sync.RWMutex
	exchangeRatesCacheTime  time.Time
)

const ecbCacheDuration = 1 * time.Hour

// handleExchangeRatesToolCall handles exchange rates tool calls
func handleExchangeRatesToolCall(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Currency string `json:"currency"`
	}

	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received exchange rates tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse exchange rates tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	currency := strings.ToUpper(strings.TrimSpace(args.Currency))

	log.Debug().Ctx(ctx).Str("currency", currency).Msg("Fetching exchange rates")

	rates, err := getExchangeRates(ctx)
	if err != nil {
		return "", err
	}

	return formatExchangeRates(rates, currency), nil
}

// getExchangeRates fetches exchange rates from the ECB API
func getExchangeRates(ctx context.Context) (*ecbCube, error) {
	// Check cache first
	exchangeRatesCacheMutex.RLock()
	if exchangeRatesCache != nil && time.Since(exchangeRatesCacheTime) < ecbCacheDuration {
		cached := exchangeRatesCache
		exchangeRatesCacheMutex.RUnlock()
		log.Debug().Ctx(ctx).Str("cache_time", exchangeRatesCacheTime.Format(time.RFC3339)).Msg("Using cached exchange rates")
		return cached, nil
	}
	exchangeRatesCacheMutex.RUnlock()

	url := "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml"

	log.Debug().Ctx(ctx).Str("url", url).Msg("Fetching exchange rates from ECB")

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", url).Msg("Failed to create ECB API request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", url).Msg("Failed to fetch exchange rates")
		return nil, fmt.Errorf("failed to fetch exchange rates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error().Ctx(ctx).Int("status_code", resp.StatusCode).Str("url", url).Msg("ECB API returned non-OK status")
		return nil, fmt.Errorf("ECB API returned status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", url).Msg("Failed to read ECB API response")
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var envelope ecbEnvelope
	if err := xml.Unmarshal(body, &envelope); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to parse ECB XML response")
		return nil, fmt.Errorf("failed to parse XML response: %w", err)
	}

	log.Debug().Ctx(ctx).Str("date", envelope.Cube.Time).Int("rate_count", len(envelope.Cube.Rates)).Msg("Successfully fetched exchange rates")

	// Update cache
	exchangeRatesCacheMutex.Lock()
	exchangeRatesCache = &envelope.Cube
	exchangeRatesCacheTime = time.Now()
	exchangeRatesCacheMutex.Unlock()

	return &envelope.Cube, nil
}

// formatExchangeRates formats the exchange rates for display
func formatExchangeRates(rates *ecbCube, currency string) string {
	if rates == nil || len(rates.Rates) == 0 {
		return "No exchange rate data available."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**EUR Exchange Rates** (as of %s)\n\n", rates.Time))

	// If a specific currency is requested
	if currency != "" {
		for _, rate := range rates.Rates {
			if rate.Currency == currency {
				sb.WriteString(fmt.Sprintf("1 EUR = %.4f %s\n", rate.Rate, rate.Currency))
				sb.WriteString(fmt.Sprintf("1 %s = %.4f EUR\n", rate.Currency, 1/rate.Rate))
				return sb.String()
			}
		}
		// Currency not found - list available currencies
		sb.WriteString(fmt.Sprintf("Currency '%s' not found.\n\n", currency))
		sb.WriteString("Available currencies: ")
		var currencies []string
		for _, rate := range rates.Rates {
			currencies = append(currencies, rate.Currency)
		}
		sort.Strings(currencies)
		sb.WriteString(strings.Join(currencies, ", "))
		return sb.String()
	}

	// Sort currencies alphabetically
	sortedRates := make([]ecbRateEntry, len(rates.Rates))
	copy(sortedRates, rates.Rates)
	sort.Slice(sortedRates, func(i, j int) bool {
		return sortedRates[i].Currency < sortedRates[j].Currency
	})

	// Show all rates
	for _, rate := range sortedRates {
		sb.WriteString(fmt.Sprintf("1 EUR = %.4f %s\n", rate.Rate, rate.Currency))
	}

	return sb.String()
}
