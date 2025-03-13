package chat

import (
	"context"
	"encoding/json"
	"encoding/xml"
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

// WeatherToolDefinition returns the tool definition for the weather tool
var WeatherToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "get_weather",
		Description: "Get current weather information for a location in Finland",
		Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"location": {
						"type": "string",
						"description": "The location in Finland to get weather information for (e.g., Helsinki, Tampere, Oulu)"
					}
				},
				"required": ["location"]
			}`),
	},
	Handler:          handleWeatherToolCall,
	ValidityDuration: 15 * time.Minute,
}

// handleWeatherToolCall handles weather tool calls
func handleWeatherToolCall(ctx context.Context, arguments string) (string, error) {
	// Parse the arguments
	var args struct {
		Location string `json:"location"`
	}

	// Log the raw arguments for debugging
	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received weather tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	if args.Location == "" {
		return "", fmt.Errorf("location is required")
	}

	// Sanitize and prepare the location
	location := strings.TrimSpace(args.Location)

	// Get weather data from FMI
	weatherData, err := getWeatherData(ctx, location)
	if err != nil {
		return "", err
	}

	return formatWeatherData(weatherData, location), nil
}

// FMIResponse represents the XML response from the FMI API
type FMIResponse struct {
	XMLName xml.Name `xml:"FeatureCollection"`
	Members []struct {
		Observation struct {
			Result struct {
				MeasurementTimeseries struct {
					ID     string `xml:"id,attr"`
					Points []struct {
						MeasurementTVP struct {
							Time  string `xml:"time"`
							Value string `xml:"value"`
						} `xml:"MeasurementTVP"`
					} `xml:"point"`
				} `xml:"MeasurementTimeseries"`
			} `xml:"result"`
		} `xml:"PointTimeSeriesObservation"`
	} `xml:"member"`
}

// WeatherData holds the processed weather information
type WeatherData struct {
	Temperature    *float64
	WindSpeed      *float64
	Humidity       *float64
	Precipitation  *float64
	Pressure       *float64
	Visibility     *float64
	TimeOfMeasure  time.Time
	Location       string
	ParameterCount int
}

// getWeatherData fetches weather data from the FMI API
func getWeatherData(ctx context.Context, location string) (*WeatherData, error) {
	// Construct the URL for the FMI API
	baseURL := "https://opendata.fmi.fi/wfs"

	// Create URL with query parameters
	params := url.Values{}
	params.Add("service", "WFS")
	params.Add("version", "2.0.0")
	params.Add("request", "getFeature")
	params.Add("storedquery_id", "fmi::observations::weather::timevaluepair")
	params.Add("place", location)

	requestURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	log.Debug().Ctx(ctx).Str("url", requestURL).Str("location", location).Msg("Fetching weather data")

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", requestURL).Msg("Failed to create weather API request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute the request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", requestURL).Msg("Failed to fetch weather data")
		return nil, fmt.Errorf("failed to fetch weather data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error().Ctx(ctx).Int("status_code", resp.StatusCode).Str("url", requestURL).Msg("Weather API returned non-OK status")

		// Try to read the error response body for more details
		errorBody, _ := io.ReadAll(resp.Body)
		if len(errorBody) > 0 {
			log.Error().Ctx(ctx).Str("error_body", string(errorBody)).Msg("Weather API error details")
		}

		return nil, fmt.Errorf("API returned status code %d", resp.StatusCode)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", requestURL).Msg("Failed to read weather API response")
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Log a sample of the response for debugging
	if len(body) > 500 {
		log.Debug().Ctx(ctx).Str("response_sample", string(body[:500])+"...").Msg("Received weather API response (sample)")
	} else {
		log.Debug().Ctx(ctx).Str("response", string(body)).Msg("Received weather API response")
	}

	// Process the XML directly without full parsing
	weatherData := &WeatherData{
		Location: location,
	}

	// Use a simpler approach to extract data from the XML
	// Look for measurement timeseries IDs and their values
	parameterMap := make(map[string]float64)
	latestTimeStr := ""

	// Extract temperature (t2m)
	t2mData := extractMeasurement(body, "obs-obs-1-1-t2m")
	if t2mData.Value != 0 {
		parameterMap["t2m"] = t2mData.Value
		if t2mData.Time > latestTimeStr {
			latestTimeStr = t2mData.Time
		}
	}

	// Extract wind speed (ws_10min)
	wsData := extractMeasurement(body, "obs-obs-1-1-ws_10min")
	if wsData.Value != 0 {
		parameterMap["ws_10min"] = wsData.Value
		if wsData.Time > latestTimeStr {
			latestTimeStr = wsData.Time
		}
	}

	// Extract humidity (rh)
	rhData := extractMeasurement(body, "obs-obs-1-1-rh")
	if rhData.Value != 0 {
		parameterMap["rh"] = rhData.Value
		if rhData.Time > latestTimeStr {
			latestTimeStr = rhData.Time
		}
	}

	// Extract precipitation (r_1h)
	r1hData := extractMeasurement(body, "obs-obs-1-1-r_1h")
	if r1hData.Value != 0 && !strings.Contains(r1hData.RawValue, "NaN") {
		parameterMap["r_1h"] = r1hData.Value
		if r1hData.Time > latestTimeStr {
			latestTimeStr = r1hData.Time
		}
	}

	// Extract pressure (p_sea)
	pSeaData := extractMeasurement(body, "obs-obs-1-1-p_sea")
	if pSeaData.Value != 0 {
		parameterMap["p_sea"] = pSeaData.Value
		if pSeaData.Time > latestTimeStr {
			latestTimeStr = pSeaData.Time
		}
	}

	// Extract visibility (vis)
	visData := extractMeasurement(body, "obs-obs-1-1-vis")
	if visData.Value != 0 {
		parameterMap["vis"] = visData.Value
		if visData.Time > latestTimeStr {
			latestTimeStr = visData.Time
		}
	}

	// Parse the measurement time
	if latestTimeStr != "" {
		timeOfMeasure, err := time.Parse(time.RFC3339, latestTimeStr)
		if err == nil {
			weatherData.TimeOfMeasure = timeOfMeasure
		}
	}

	// Assign values to the weather data struct
	if temp, ok := parameterMap["t2m"]; ok {
		weatherData.Temperature = &temp
		weatherData.ParameterCount++
	}

	if windSpeed, ok := parameterMap["ws_10min"]; ok {
		weatherData.WindSpeed = &windSpeed
		weatherData.ParameterCount++
	}

	if humidity, ok := parameterMap["rh"]; ok {
		weatherData.Humidity = &humidity
		weatherData.ParameterCount++
	}

	if precipitation, ok := parameterMap["r_1h"]; ok {
		weatherData.Precipitation = &precipitation
		weatherData.ParameterCount++
	}

	if pressure, ok := parameterMap["p_sea"]; ok {
		weatherData.Pressure = &pressure
		weatherData.ParameterCount++
	}

	if visibility, ok := parameterMap["vis"]; ok {
		// Convert from meters to kilometers for better readability
		visKm := visibility / 1000
		weatherData.Visibility = &visKm
		weatherData.ParameterCount++
	}

	if weatherData.ParameterCount == 0 {
		return nil, fmt.Errorf("no weather data found for location: %s", location)
	}

	log.Debug().Ctx(ctx).
		Str("location", location).
		Int("parameter_count", weatherData.ParameterCount).
		Msg("Successfully fetched weather data")

	return weatherData, nil
}

// MeasurementData holds extracted measurement data
type MeasurementData struct {
	Time     string
	Value    float64
	RawValue string
}

// extractMeasurement extracts a specific measurement from the XML response
func extractMeasurement(xmlData []byte, measurementID string) MeasurementData {
	// Convert to string for easier processing
	xmlStr := string(xmlData)

	// Find the measurement timeseries section
	idMarker := fmt.Sprintf("gml:id=\"%s\"", measurementID)
	idIndex := strings.Index(xmlStr, idMarker)
	if idIndex == -1 {
		return MeasurementData{}
	}

	// Find the first measurement point after the ID
	pointStartMarker := "<wml2:point>"
	pointStartIndex := strings.Index(xmlStr[idIndex:], pointStartMarker)
	if pointStartIndex == -1 {
		return MeasurementData{}
	}

	// Get the section containing the measurement
	measurementSection := xmlStr[idIndex+pointStartIndex:]

	// Extract time
	timeStartMarker := "<wml2:time>"
	timeEndMarker := "</wml2:time>"
	timeStartIndex := strings.Index(measurementSection, timeStartMarker)
	timeEndIndex := strings.Index(measurementSection, timeEndMarker)
	if timeStartIndex == -1 || timeEndIndex == -1 {
		return MeasurementData{}
	}

	timeStr := measurementSection[timeStartIndex+len(timeStartMarker) : timeEndIndex]

	// Extract value
	valueStartMarker := "<wml2:value>"
	valueEndMarker := "</wml2:value>"
	valueStartIndex := strings.Index(measurementSection, valueStartMarker)
	valueEndIndex := strings.Index(measurementSection, valueEndMarker)
	if valueStartIndex == -1 || valueEndIndex == -1 {
		return MeasurementData{}
	}

	valueStr := measurementSection[valueStartIndex+len(valueStartMarker) : valueEndIndex]

	// Convert value to float
	value, _ := strconv.ParseFloat(valueStr, 64)

	return MeasurementData{
		Time:     timeStr,
		Value:    value,
		RawValue: valueStr,
	}
}

// formatWeatherData formats the weather data for display
func formatWeatherData(data *WeatherData, location string) string {
	if data == nil || data.ParameterCount == 0 {
		return fmt.Sprintf("No weather data available for %s.", location)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Current Weather in %s**\n\n", data.Location))

	// Add measurement time if available
	if !data.TimeOfMeasure.IsZero() {
		// Convert to local time for display
		localTime := data.TimeOfMeasure.Local()
		sb.WriteString(fmt.Sprintf("Time of measurement: %s\n\n", localTime.Format("2006-01-02 15:04 MST")))
	}

	// Add temperature if available
	if data.Temperature != nil {
		sb.WriteString(fmt.Sprintf("Temperature: %.1f°C\n", *data.Temperature))
	}

	// Add wind speed if available
	if data.WindSpeed != nil {
		sb.WriteString(fmt.Sprintf("Wind Speed: %.1f m/s\n", *data.WindSpeed))
	}

	// Add humidity if available
	if data.Humidity != nil {
		sb.WriteString(fmt.Sprintf("Humidity: %.1f%%\n", *data.Humidity))
	}

	// Add precipitation if available
	if data.Precipitation != nil {
		sb.WriteString(fmt.Sprintf("Precipitation (1h): %.1f mm\n", *data.Precipitation))
	}

	// Add pressure if available
	if data.Pressure != nil {
		sb.WriteString(fmt.Sprintf("Pressure: %.1f hPa\n", *data.Pressure))
	}

	// Add visibility if available
	if data.Visibility != nil {
		sb.WriteString(fmt.Sprintf("Visibility: %.1f km\n", *data.Visibility))
	}

	return sb.String()
}
