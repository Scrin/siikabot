package chat

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

// FingridToolDefinition returns the tool definition for the Fingrid power production tool
var FingridToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "get_fingrid_power_stats",
		Description: "Get power production statistics from Fingrid for Finland's power system at a specific time",
		Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"datetime": {
						"type": "string",
						"description": "Datetime to fetch statistics for in YYYY-MM-DD HH:MM format."
					}
				},
				"required": ["datetime"]
			}`),
	},
	Handler: handleFingridToolCall,
}

// handleFingridToolCall handles Fingrid power production statistics tool calls
func handleFingridToolCall(ctx context.Context, arguments string) (string, error) {
	// Parse the arguments
	var args struct {
		Datetime string `json:"datetime"`
	}

	// Log the raw arguments for debugging
	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received Fingrid power stats tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Check if datetime is provided (should be required by schema, but double-check)
	if args.Datetime == "" {
		log.Error().Ctx(ctx).Msg("Datetime parameter is required but was not provided")
		return "", fmt.Errorf("datetime parameter is required")
	}

	// Parse the datetime
	var targetTime time.Time
	var err error

	// Try parsing with various formats
	formats := []string{
		"2006-01-02 15:04",
		"2006-01-02T15:04",
		"02.01.2006 15:04",
		"2.1.2006 15:04",
		"January 2, 2006 15:04",
		"Jan 2, 2006 15:04",
		// Also try date-only formats and use 00:00 as the time
		"2006-01-02",
		"02.01.2006",
		"2.1.2006",
		"January 2, 2006",
		"Jan 2, 2006",
	}

	parsed := false
	for _, format := range formats {
		if t, parseErr := time.Parse(format, args.Datetime); parseErr == nil {
			// Load the local timezone from config - we can assume it's always valid
			loc, _ := time.LoadLocation(config.Timezone)

			// Assume the input time is in the local timezone and convert to UTC
			targetTime = time.Date(
				t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0,
				loc, // Use the configured timezone
			).UTC() // Convert to UTC for API query

			parsed = true
			break
		}
	}

	if !parsed {
		log.Error().Ctx(ctx).Str("datetime", args.Datetime).Msg("Failed to parse datetime")
		return "", fmt.Errorf("failed to parse datetime: %s", args.Datetime)
	}

	log.Debug().Ctx(ctx).
		Time("local_datetime", targetTime.In(time.Local)).
		Time("utc_datetime", targetTime).
		Str("formatted_datetime", targetTime.Format("2006-01-02 15:04:05 MST")).
		Msg("Using datetime for Fingrid power stats (converted to UTC)")

	// Get the power production statistics
	stats, err := getFingridPowerStats(ctx, targetTime)
	if err != nil {
		return "", err
	}

	return formatFingridPowerStats(stats), nil
}

// FingridResponse represents the response from the Fingrid API
type FingridResponse struct {
	CogenerationDistrictHeating []FingridDataPoint `json:"CogenerationDistrictHeating"`
	CogenerationIndustry        []FingridDataPoint `json:"CogenerationIndustry"`
	Consumption                 []FingridDataPoint `json:"Consumption"`
	ConsumptionEmissionCo2      []FingridDataPoint `json:"ConsumptionEmissionCo2"`
	ConsumptionForecast         []FingridDataPoint `json:"ConsumptionForecast"`
	ElectricityPriceInFinland   []FingridDataPoint `json:"ElectricityPriceInFinland"`
	HydroPower                  []FingridDataPoint `json:"HydroPower"`
	NetImportExport             []FingridDataPoint `json:"NetImportExport"`
	NuclearPower                []FingridDataPoint `json:"NuclearPower"`
	OtherProduction             []FingridDataPoint `json:"OtherProduction"`
	PeakLoadPower               []FingridDataPoint `json:"PeakLoadPower"`
	Production                  []FingridDataPoint `json:"Production"`
	ProductionEmissionCo2       []FingridDataPoint `json:"ProductionEmissionCo2"`
	ProductionForecast          []FingridDataPoint `json:"ProductionForecast"`
	SolarPower                  []FingridDataPoint `json:"SolarPower"`
	WindPower                   []FingridDataPoint `json:"WindPower"`
}

// FingridDataPoint represents a single data point in the Fingrid API response
type FingridDataPoint struct {
	Value     *float64 `json:"value"`
	StartTime string   `json:"startTime"`
	EndTime   string   `json:"endTime"`
	DatasetID int      `json:"datasetId"`
}

// PowerStats represents the power production statistics for a specific time
type PowerStats struct {
	Timestamp                   time.Time
	CogenerationDistrictHeating float64
	CogenerationIndustry        float64
	Consumption                 float64
	HydroPower                  float64
	NetImportExport             float64
	NuclearPower                float64
	OtherProduction             float64
	Production                  float64
	SolarPower                  float64
	WindPower                   float64
}

// getFingridPowerStats fetches power production statistics from the Fingrid API
func getFingridPowerStats(ctx context.Context, targetTime time.Time) (*PowerStats, error) {
	// Ensure targetTime is in UTC
	targetTime = targetTime.UTC()

	// Format the dates for the API request
	// Use the date of the target time for the start date
	startDate := targetTime.Format("2006-01-02")
	// Use the next day for the end date to ensure we get data for the entire day
	endDate := targetTime.AddDate(0, 0, 1).Format("2006-01-02")

	// Construct the Fingrid API URL
	url := fmt.Sprintf("https://www.fingrid.fi/api/graph/power-system-production?start=%s&end=%s", startDate, endDate)

	log.Debug().Ctx(ctx).Str("url", url).Time("target_time_utc", targetTime).Msg("Fetching Fingrid power stats")

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", url).Msg("Failed to create Fingrid API request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // fingrid has a fishy cert
		},
	}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}

	log.Debug().Ctx(ctx).Msg("Using insecure HTTP client to ignore certificate errors")

	resp, err := client.Do(req)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", url).Msg("Failed to fetch Fingrid power stats")
		return nil, fmt.Errorf("failed to fetch power stats: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error().Ctx(ctx).Int("status_code", resp.StatusCode).Str("url", url).Msg("Fingrid API returned non-OK status")

		// Try to read the error response body for more details
		errorBody, _ := io.ReadAll(resp.Body)
		if len(errorBody) > 0 {
			log.Error().Ctx(ctx).Str("error_body", string(errorBody)).Msg("Fingrid API error details")
		}

		return nil, fmt.Errorf("API returned status code %d", resp.StatusCode)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", url).Msg("Failed to read Fingrid API response")
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse the response
	var fingridResponse FingridResponse
	if err := json.Unmarshal(body, &fingridResponse); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("response", string(body)).Msg("Failed to decode Fingrid response")
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Count valid (non-null) data points for each category
	countValidPoints := func(dataPoints []FingridDataPoint) int {
		valid := 0
		for _, point := range dataPoints {
			if point.Value != nil {
				valid++
			}
		}
		return valid
	}

	// Log the number of data points for each category
	log.Debug().Ctx(ctx).
		Int("cogeneration_district_heating_count", len(fingridResponse.CogenerationDistrictHeating)).
		Int("cogeneration_district_heating_valid", countValidPoints(fingridResponse.CogenerationDistrictHeating)).
		Int("cogeneration_industry_count", len(fingridResponse.CogenerationIndustry)).
		Int("cogeneration_industry_valid", countValidPoints(fingridResponse.CogenerationIndustry)).
		Int("consumption_count", len(fingridResponse.Consumption)).
		Int("consumption_valid", countValidPoints(fingridResponse.Consumption)).
		Int("hydro_power_count", len(fingridResponse.HydroPower)).
		Int("hydro_power_valid", countValidPoints(fingridResponse.HydroPower)).
		Int("nuclear_power_count", len(fingridResponse.NuclearPower)).
		Int("nuclear_power_valid", countValidPoints(fingridResponse.NuclearPower)).
		Int("production_count", len(fingridResponse.Production)).
		Int("production_valid", countValidPoints(fingridResponse.Production)).
		Int("solar_power_count", len(fingridResponse.SolarPower)).
		Int("solar_power_valid", countValidPoints(fingridResponse.SolarPower)).
		Int("wind_power_count", len(fingridResponse.WindPower)).
		Int("wind_power_valid", countValidPoints(fingridResponse.WindPower)).
		Msg("Fingrid API data point counts")

	// Create a new PowerStats object
	stats := &PowerStats{}

	// Helper function to find the nearest data point to the target time
	findNearestDataPoint := func(dataPoints []FingridDataPoint) (time.Time, float64, error) {
		if len(dataPoints) == 0 {
			return time.Time{}, 0, fmt.Errorf("no data points available")
		}

		var nearestPoint FingridDataPoint
		var nearestTime time.Time
		var minDiff time.Duration = time.Hour * 24 // Start with a large value
		const maxAllowedDiff = 10 * time.Minute    // Maximum allowed time difference (10 minutes)

		for _, point := range dataPoints {
			// Skip points with null values
			if point.Value == nil {
				continue
			}

			// Parse the timestamp (API returns timestamps in UTC)
			timestamp, err := time.Parse(time.RFC3339, point.StartTime)
			if err != nil {
				log.Error().Ctx(ctx).Err(err).Str("time_string", point.StartTime).Msg("Failed to parse timestamp")
				continue
			}

			// Calculate the absolute time difference
			diff := timestamp.Sub(targetTime)
			if diff < 0 {
				diff = -diff // Get absolute value
			}

			// Update if this point is closer to the target time
			if diff < minDiff {
				minDiff = diff
				nearestPoint = point
				nearestTime = timestamp
			}
		}

		// If we found a nearest point, check if it's within the allowed time difference
		if !nearestTime.IsZero() {
			diff := nearestTime.Sub(targetTime)
			if diff < 0 {
				diff = -diff // Get absolute value
			}

			if diff <= maxAllowedDiff {
				log.Debug().Ctx(ctx).
					Time("timestamp_utc", nearestTime).
					Float64("value", *nearestPoint.Value).
					Dur("diff", diff).
					Msg("Found nearest data point within allowed time difference")
				return nearestTime, *nearestPoint.Value, nil
			}

			// If the nearest point is too far away, return an error
			log.Debug().Ctx(ctx).
				Time("timestamp_utc", nearestTime).
				Float64("value", *nearestPoint.Value).
				Dur("diff", diff).
				Msg("Nearest data point is too far from target time")
			return time.Time{}, 0, fmt.Errorf("nearest data point is %s away from requested time (maximum allowed is 10 minutes)", diff.Round(time.Second))
		}

		return time.Time{}, 0, fmt.Errorf("no suitable data point found")
	}

	// Find the nearest data points for each category
	var dataTimestamp time.Time
	var dataError error

	// First check if we can get a valid timestamp from any category
	if len(fingridResponse.CogenerationDistrictHeating) > 0 {
		timestamp, value, err := findNearestDataPoint(fingridResponse.CogenerationDistrictHeating)
		if err == nil {
			stats.Timestamp = timestamp // Use this as the reference timestamp
			stats.CogenerationDistrictHeating = value
			dataTimestamp = timestamp
		} else {
			dataError = err
		}
	} else if len(fingridResponse.Production) > 0 {
		timestamp, value, err := findNearestDataPoint(fingridResponse.Production)
		if err == nil {
			stats.Timestamp = timestamp
			stats.Production = value
			dataTimestamp = timestamp
		} else if dataError == nil {
			dataError = err
		}
	} else if len(fingridResponse.Consumption) > 0 {
		timestamp, value, err := findNearestDataPoint(fingridResponse.Consumption)
		if err == nil {
			stats.Timestamp = timestamp
			stats.Consumption = value
			dataTimestamp = timestamp
		} else if dataError == nil {
			dataError = err
		}
	}

	// If we couldn't find a valid timestamp in any category, return the error
	if dataTimestamp.IsZero() {
		if dataError != nil {
			return nil, dataError
		}
		return nil, fmt.Errorf("no data available for the requested time")
	}

	// Now get the values for each category using the validated timestamp
	if len(fingridResponse.CogenerationIndustry) > 0 {
		_, value, err := findNearestDataPoint(fingridResponse.CogenerationIndustry)
		if err == nil {
			stats.CogenerationIndustry = value
		}
	}

	if len(fingridResponse.Consumption) > 0 && stats.Consumption == 0 {
		_, value, err := findNearestDataPoint(fingridResponse.Consumption)
		if err == nil {
			stats.Consumption = value
		}
	}

	if len(fingridResponse.HydroPower) > 0 {
		_, value, err := findNearestDataPoint(fingridResponse.HydroPower)
		if err == nil {
			stats.HydroPower = value
		}
	}

	if len(fingridResponse.NetImportExport) > 0 {
		_, value, err := findNearestDataPoint(fingridResponse.NetImportExport)
		if err == nil {
			stats.NetImportExport = value
		}
	}

	if len(fingridResponse.NuclearPower) > 0 {
		_, value, err := findNearestDataPoint(fingridResponse.NuclearPower)
		if err == nil {
			stats.NuclearPower = value
		}
	}

	if len(fingridResponse.OtherProduction) > 0 {
		_, value, err := findNearestDataPoint(fingridResponse.OtherProduction)
		if err == nil {
			stats.OtherProduction = value
		}
	}

	if len(fingridResponse.Production) > 0 && stats.Production == 0 {
		_, value, err := findNearestDataPoint(fingridResponse.Production)
		if err == nil {
			stats.Production = value
		}
	}

	if len(fingridResponse.SolarPower) > 0 {
		_, value, err := findNearestDataPoint(fingridResponse.SolarPower)
		if err == nil {
			stats.SolarPower = value
		}
	}

	if len(fingridResponse.WindPower) > 0 {
		_, value, err := findNearestDataPoint(fingridResponse.WindPower)
		if err == nil {
			stats.WindPower = value
		}
	}

	return stats, nil
}

// formatFingridPowerStats formats the power statistics into a readable string
func formatFingridPowerStats(stats *PowerStats) string {
	if stats == nil {
		return "No power production statistics available."
	}

	// Load the local timezone from config - we can assume it's always valid
	loc, _ := time.LoadLocation(config.Timezone)

	// Format the timestamp in the local timezone
	localTime := stats.Timestamp.In(loc)
	timestamp := localTime.Format("2006-01-02 15:04:05 MST")

	// Create a formatted response
	response := fmt.Sprintf("# Finland Power Production Statistics\n\n**Time**: %s\n\n", timestamp)

	response += "## Production\n\n"
	response += fmt.Sprintf("- **Total Production**: %.1f MW\n", stats.Production)
	response += fmt.Sprintf("- **Nuclear Power**: %.1f MW\n", stats.NuclearPower)
	response += fmt.Sprintf("- **Hydro Power**: %.1f MW\n", stats.HydroPower)
	response += fmt.Sprintf("- **Wind Power**: %.1f MW\n", stats.WindPower)
	response += fmt.Sprintf("- **Solar Power**: %.1f MW\n", stats.SolarPower)
	response += fmt.Sprintf("- **Cogeneration (District Heating)**: %.1f MW\n", stats.CogenerationDistrictHeating)
	response += fmt.Sprintf("- **Cogeneration (Industry)**: %.1f MW\n", stats.CogenerationIndustry)
	response += fmt.Sprintf("- **Other Production**: %.1f MW\n", stats.OtherProduction)

	response += "\n## Consumption and Balance\n\n"
	response += fmt.Sprintf("- **Total Consumption**: %.1f MW\n", stats.Consumption)

	// Net import/export (positive = import, negative = export)
	importExportLabel := "Import"
	importExportValue := stats.NetImportExport
	if importExportValue < 0 {
		importExportLabel = "Export"
		importExportValue = -importExportValue
	}
	response += fmt.Sprintf("- **Net %s**: %.1f MW\n", importExportLabel, importExportValue)

	// Calculate the percentage of each production type
	if stats.Production > 0 {
		response += "\n## Production Mix\n\n"
		response += fmt.Sprintf("- **Nuclear**: %.1f%%\n", (stats.NuclearPower/stats.Production)*100)
		response += fmt.Sprintf("- **Hydro**: %.1f%%\n", (stats.HydroPower/stats.Production)*100)
		response += fmt.Sprintf("- **Wind**: %.1f%%\n", (stats.WindPower/stats.Production)*100)
		response += fmt.Sprintf("- **Solar**: %.1f%%\n", (stats.SolarPower/stats.Production)*100)
		response += fmt.Sprintf("- **Cogeneration (DH)**: %.1f%%\n", (stats.CogenerationDistrictHeating/stats.Production)*100)
		response += fmt.Sprintf("- **Cogeneration (Industry)**: %.1f%%\n", (stats.CogenerationIndustry/stats.Production)*100)
		response += fmt.Sprintf("- **Other**: %.1f%%\n", (stats.OtherProduction/stats.Production)*100)
	}

	return response
}
