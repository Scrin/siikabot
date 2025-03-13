package llmtools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

// WeatherForecastToolDefinition returns the tool definition for the weather forecast tool
var WeatherForecastToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "get_weather_forecast",
		Description: "Get weather forecast for a location in Finland",
		Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"location": {
						"type": "string",
						"description": "The location in Finland to get weather forecast for (e.g., Helsinki, Tampere, Oulu)"
					},
					"days": {
						"type": "integer",
						"description": "Number of days to forecast (1-3). Defaults to 1 if not specified.",
						"minimum": 1,
						"maximum": 3
					}
				},
				"required": ["location"]
			}`),
	},
	Handler:          handleWeatherForecastToolCall,
	ValidityDuration: 15 * time.Minute,
}

// handleWeatherForecastToolCall handles weather forecast tool calls
func handleWeatherForecastToolCall(ctx context.Context, arguments string) (string, error) {
	// Parse the arguments
	var args struct {
		Location string `json:"location"`
		Days     int    `json:"days"`
	}

	// Log the raw arguments for debugging
	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received weather forecast tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	if args.Location == "" {
		return "", fmt.Errorf("location is required")
	}

	// Set default days if not specified
	if args.Days <= 0 {
		args.Days = 1
	} else if args.Days > 3 {
		args.Days = 3 // Cap at 3 days
	}

	// Sanitize and prepare the location
	location := strings.TrimSpace(args.Location)

	// Get forecast data from FMI
	forecastData, err := getForecastData(ctx, location, args.Days)
	if err != nil {
		return "", err
	}

	return formatForecastData(forecastData, location, args.Days), nil
}

// ForecastEntry represents a single forecast entry
type ForecastEntry struct {
	Time          time.Time
	Temperature   float64
	WindSpeed     float64
	Humidity      float64
	Pressure      float64
	Precipitation float64
	HasData       bool
}

// ForecastData holds the processed forecast information
type ForecastData struct {
	Forecasts []ForecastEntry
	Location  string
	StartTime time.Time
	EndTime   time.Time
}

// getForecastData fetches forecast data from the FMI API
func getForecastData(ctx context.Context, location string, days int) (*ForecastData, error) {
	// Construct the URL for the FMI API
	baseURL := "https://opendata.fmi.fi/wfs"

	// Create URL with query parameters
	params := url.Values{}
	params.Add("service", "WFS")
	params.Add("version", "2.0.0")
	params.Add("request", "getFeature")
	params.Add("storedquery_id", "ecmwf::forecast::surface::point::timevaluepair")
	params.Add("place", location)

	requestURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	log.Debug().Ctx(ctx).Str("url", requestURL).Str("location", location).Int("days", days).Msg("Fetching weather forecast data")

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", requestURL).Msg("Failed to create forecast API request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute the request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", requestURL).Msg("Failed to fetch forecast data")
		return nil, fmt.Errorf("failed to fetch forecast data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error().Ctx(ctx).Int("status_code", resp.StatusCode).Str("url", requestURL).Msg("Forecast API returned non-OK status")

		// Try to read the error response body for more details
		errorBody, _ := io.ReadAll(resp.Body)
		if len(errorBody) > 0 {
			log.Error().Ctx(ctx).Str("error_body", string(errorBody)).Msg("Forecast API error details")
		}

		return nil, fmt.Errorf("API returned status code %d", resp.StatusCode)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", requestURL).Msg("Failed to read forecast API response")
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Log a sample of the response for debugging
	if len(body) > 500 {
		log.Debug().Ctx(ctx).Str("response_sample", string(body[:500])+"...").Msg("Received forecast API response (sample)")
	} else {
		log.Debug().Ctx(ctx).Str("response", string(body)).Msg("Received forecast API response")
	}

	// Process the response to extract forecast data
	forecastData := &ForecastData{
		Location:  location,
		Forecasts: []ForecastEntry{},
	}

	// Extract temperature data
	tempData := extractForecastData(body, "temperature")

	// Extract wind speed data
	windData := extractForecastData(body, "windspeed")

	// Extract humidity data
	humidityData := extractForecastData(body, "humidity")

	// Extract pressure data
	pressureData := extractForecastData(body, "pressure")

	// Extract precipitation data
	precipData := extractForecastData(body, "precipitation1h")

	// Calculate the end time based on days parameter
	now := time.Now()
	endTime := now.Add(time.Duration(days) * 24 * time.Hour)

	// Create a map to store forecast entries by time
	forecastMap := make(map[string]ForecastEntry)

	// Process temperature data
	for timeStr, value := range tempData {
		t, err := time.Parse(time.RFC3339, timeStr)
		if err != nil {
			continue
		}

		// Skip entries before current time or after end time
		if t.Before(now) || t.After(endTime) {
			continue
		}

		entry := ForecastEntry{
			Time:        t,
			Temperature: value,
			HasData:     true,
		}

		forecastMap[timeStr] = entry
	}

	// Add wind speed data
	for timeStr, value := range windData {
		t, err := time.Parse(time.RFC3339, timeStr)
		if err != nil {
			continue
		}

		// Skip entries before current time or after end time
		if t.Before(now) || t.After(endTime) {
			continue
		}

		entry, exists := forecastMap[timeStr]
		if !exists {
			entry = ForecastEntry{
				Time:    t,
				HasData: true,
			}
		}

		entry.WindSpeed = value
		forecastMap[timeStr] = entry
	}

	// Add humidity data
	for timeStr, value := range humidityData {
		t, err := time.Parse(time.RFC3339, timeStr)
		if err != nil {
			continue
		}

		// Skip entries before current time or after end time
		if t.Before(now) || t.After(endTime) {
			continue
		}

		entry, exists := forecastMap[timeStr]
		if !exists {
			entry = ForecastEntry{
				Time:    t,
				HasData: true,
			}
		}

		entry.Humidity = value
		forecastMap[timeStr] = entry
	}

	// Add pressure data
	for timeStr, value := range pressureData {
		t, err := time.Parse(time.RFC3339, timeStr)
		if err != nil {
			continue
		}

		// Skip entries before current time or after end time
		if t.Before(now) || t.After(endTime) {
			continue
		}

		entry, exists := forecastMap[timeStr]
		if !exists {
			entry = ForecastEntry{
				Time:    t,
				HasData: true,
			}
		}

		entry.Pressure = value
		forecastMap[timeStr] = entry
	}

	// Add precipitation data
	for timeStr, value := range precipData {
		t, err := time.Parse(time.RFC3339, timeStr)
		if err != nil {
			continue
		}

		// Skip entries before current time or after end time
		if t.Before(now) || t.After(endTime) {
			continue
		}

		entry, exists := forecastMap[timeStr]
		if !exists {
			entry = ForecastEntry{
				Time:    t,
				HasData: true,
			}
		}

		entry.Precipitation = value
		forecastMap[timeStr] = entry
	}

	// Convert map to slice and sort by time
	for _, entry := range forecastMap {
		if entry.HasData {
			forecastData.Forecasts = append(forecastData.Forecasts, entry)
		}
	}

	// Sort forecasts by time
	sortForecastsByTime(forecastData.Forecasts)

	if len(forecastData.Forecasts) > 0 {
		forecastData.StartTime = forecastData.Forecasts[0].Time
		forecastData.EndTime = forecastData.Forecasts[len(forecastData.Forecasts)-1].Time
	}

	log.Debug().Ctx(ctx).
		Str("location", location).
		Int("forecast_count", len(forecastData.Forecasts)).
		Time("start_time", forecastData.StartTime).
		Time("end_time", forecastData.EndTime).
		Msg("Successfully fetched forecast data")

	return forecastData, nil
}

// extractForecastData extracts a specific type of forecast data from the XML response
func extractForecastData(xmlData []byte, dataType string) map[string]float64 {
	// Convert to string for easier processing
	xmlStr := string(xmlData)

	// Map to store time -> value pairs
	data := make(map[string]float64)

	// Different data types have different patterns in the XML
	var searchPattern string
	switch dataType {
	case "temperature":
		// Look for the temperature section (usually the second data block)
		searchPattern = "Temperature"
	case "windspeed":
		searchPattern = "WindSpeedMS"
	case "humidity":
		searchPattern = "Humidity"
	case "pressure":
		searchPattern = "Pressure"
	case "precipitation1h":
		searchPattern = "Precipitation1h"
	default:
		return data
	}

	// Find the section containing the data type
	sectionIndex := strings.Index(xmlStr, searchPattern)
	if sectionIndex == -1 {
		return data
	}

	// Get the section after the data type marker
	section := xmlStr[sectionIndex:]

	// Extract all time-value pairs
	timePattern := "<wml2:time>"
	valuePattern := "<wml2:value>"

	for {
		timeStartIndex := strings.Index(section, timePattern)
		if timeStartIndex == -1 {
			break
		}

		timeEndIndex := strings.Index(section[timeStartIndex:], "</wml2:time>")
		if timeEndIndex == -1 {
			break
		}

		timeStr := section[timeStartIndex+len(timePattern) : timeStartIndex+timeEndIndex]

		// Move to the value part
		valueSection := section[timeStartIndex+timeEndIndex:]

		valueStartIndex := strings.Index(valueSection, valuePattern)
		if valueStartIndex == -1 {
			break
		}

		valueEndIndex := strings.Index(valueSection[valueStartIndex:], "</wml2:value>")
		if valueEndIndex == -1 {
			break
		}

		valueStr := valueSection[valueStartIndex+len(valuePattern) : valueStartIndex+valueEndIndex]

		// Skip NaN values
		if valueStr != "NaN" {
			value, err := strconv.ParseFloat(valueStr, 64)
			if err == nil {
				data[timeStr] = value
			}
		}

		// Move to the next pair
		section = valueSection[valueStartIndex+valueEndIndex:]
	}

	return data
}

// sortForecastsByTime sorts forecast entries by time
func sortForecastsByTime(forecasts []ForecastEntry) {
	// Simple bubble sort for now
	for i := 0; i < len(forecasts); i++ {
		for j := i + 1; j < len(forecasts); j++ {
			if forecasts[i].Time.After(forecasts[j].Time) {
				forecasts[i], forecasts[j] = forecasts[j], forecasts[i]
			}
		}
	}
}

// formatForecastData formats the forecast data for display
func formatForecastData(data *ForecastData, location string, days int) string {
	if data == nil || len(data.Forecasts) == 0 {
		return fmt.Sprintf("No forecast data available for %s.", location)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Weather Forecast for %s**\n\n", data.Location))

	// Group forecasts by day
	forecastsByDay := make(map[string][]ForecastEntry)

	for _, forecast := range data.Forecasts {
		day := forecast.Time.Format("2006-01-02")
		forecastsByDay[day] = append(forecastsByDay[day], forecast)
	}

	// Sort days
	var days_sorted []string
	for day := range forecastsByDay {
		days_sorted = append(days_sorted, day)
	}

	// Simple bubble sort for days
	for i := 0; i < len(days_sorted); i++ {
		for j := i + 1; j < len(days_sorted); j++ {
			if days_sorted[i] > days_sorted[j] {
				days_sorted[i], days_sorted[j] = days_sorted[j], days_sorted[i]
			}
		}
	}

	// Display forecasts by day
	for _, day := range days_sorted {
		forecasts := forecastsByDay[day]

		// Parse the day for better formatting
		t, _ := time.Parse("2006-01-02", day)
		dayName := t.Format("Monday, January 2")

		sb.WriteString(fmt.Sprintf("**%s**\n\n", dayName))

		// Display forecasts for this day
		for _, forecast := range forecasts {
			// Only show every 3 hours to keep the output concise
			hour := forecast.Time.Hour()
			if hour%3 == 0 {
				localTime := forecast.Time.Local()
				timeStr := localTime.Format("15:04")

				sb.WriteString(fmt.Sprintf("**%s**:\n", timeStr))
				sb.WriteString(fmt.Sprintf("  Temperature: %.1f°C\n", forecast.Temperature))

				if forecast.WindSpeed > 0 {
					sb.WriteString(fmt.Sprintf("  Wind Speed: %.1f m/s\n", forecast.WindSpeed))
				}

				if forecast.Humidity > 0 {
					sb.WriteString(fmt.Sprintf("  Humidity: %.1f%%\n", forecast.Humidity))
				}

				if forecast.Precipitation > 0 {
					sb.WriteString(fmt.Sprintf("  Precipitation: %.1f mm\n", forecast.Precipitation))
				}

				if forecast.Pressure > 0 {
					sb.WriteString(fmt.Sprintf("  Pressure: %.1f hPa\n", forecast.Pressure))
				}

				sb.WriteString("\n")
			}
		}
	}

	return sb.String()
}
