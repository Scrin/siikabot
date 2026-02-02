package llmtools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

// TimezoneToolDefinition returns the tool definition for the timezone tool
var TimezoneToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "get_time",
		Description: "Get current time in a timezone or city, or convert a time between timezones. Supports IANA timezone names (e.g., Europe/Helsinki) or common city names (e.g., Tokyo, London).",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"timezone": {
					"type": "string",
					"description": "Target timezone as IANA name (e.g., Europe/Helsinki, America/New_York) or city name (e.g., Tokyo, London, New York)"
				},
				"convert_time": {
					"type": "string",
					"description": "Optional: a time to convert in format HH:MM or YYYY-MM-DD HH:MM (e.g., '15:30', '2024-01-15 09:00')"
				},
				"from_timezone": {
					"type": "string",
					"description": "Optional: source timezone when converting a time (defaults to UTC if not specified)"
				}
			},
			"required": ["timezone"]
		}`),
	},
	Handler:          handleTimezoneToolCall,
	ValidityDuration: 1 * time.Minute,
}

// cityToTimezone maps common city names to IANA timezone identifiers
var cityToTimezone = map[string]string{
	// Europe
	"helsinki":   "Europe/Helsinki",
	"stockholm":  "Europe/Stockholm",
	"oslo":       "Europe/Oslo",
	"copenhagen": "Europe/Copenhagen",
	"tallinn":    "Europe/Tallinn",
	"riga":       "Europe/Riga",
	"vilnius":    "Europe/Vilnius",
	"london":     "Europe/London",
	"dublin":     "Europe/Dublin",
	"paris":      "Europe/Paris",
	"berlin":     "Europe/Berlin",
	"amsterdam":  "Europe/Amsterdam",
	"brussels":   "Europe/Brussels",
	"vienna":     "Europe/Vienna",
	"zurich":     "Europe/Zurich",
	"rome":       "Europe/Rome",
	"madrid":     "Europe/Madrid",
	"lisbon":     "Europe/Lisbon",
	"athens":     "Europe/Athens",
	"warsaw":     "Europe/Warsaw",
	"prague":     "Europe/Prague",
	"budapest":   "Europe/Budapest",
	"moscow":     "Europe/Moscow",
	"istanbul":   "Europe/Istanbul",
	"kyiv":       "Europe/Kyiv",
	"kiev":       "Europe/Kyiv",

	// North America
	"new york":      "America/New_York",
	"nyc":           "America/New_York",
	"los angeles":   "America/Los_Angeles",
	"la":            "America/Los_Angeles",
	"chicago":       "America/Chicago",
	"houston":       "America/Chicago",
	"phoenix":       "America/Phoenix",
	"denver":        "America/Denver",
	"seattle":       "America/Los_Angeles",
	"san francisco": "America/Los_Angeles",
	"miami":         "America/New_York",
	"boston":        "America/New_York",
	"toronto":       "America/Toronto",
	"vancouver":     "America/Vancouver",
	"montreal":      "America/Montreal",
	"mexico city":   "America/Mexico_City",

	// South America
	"sao paulo":      "America/Sao_Paulo",
	"buenos aires":   "America/Argentina/Buenos_Aires",
	"rio de janeiro": "America/Sao_Paulo",
	"lima":           "America/Lima",
	"bogota":         "America/Bogota",
	"santiago":       "America/Santiago",

	// Asia
	"tokyo":        "Asia/Tokyo",
	"osaka":        "Asia/Tokyo",
	"seoul":        "Asia/Seoul",
	"beijing":      "Asia/Shanghai",
	"shanghai":     "Asia/Shanghai",
	"hong kong":    "Asia/Hong_Kong",
	"taipei":       "Asia/Taipei",
	"singapore":    "Asia/Singapore",
	"bangkok":      "Asia/Bangkok",
	"jakarta":      "Asia/Jakarta",
	"mumbai":       "Asia/Kolkata",
	"delhi":        "Asia/Kolkata",
	"bangalore":    "Asia/Kolkata",
	"kolkata":      "Asia/Kolkata",
	"dubai":        "Asia/Dubai",
	"abu dhabi":    "Asia/Dubai",
	"tel aviv":     "Asia/Jerusalem",
	"jerusalem":    "Asia/Jerusalem",
	"riyadh":       "Asia/Riyadh",
	"karachi":      "Asia/Karachi",
	"dhaka":        "Asia/Dhaka",
	"kuala lumpur": "Asia/Kuala_Lumpur",
	"manila":       "Asia/Manila",
	"hanoi":        "Asia/Ho_Chi_Minh",
	"ho chi minh":  "Asia/Ho_Chi_Minh",

	// Oceania
	"sydney":     "Australia/Sydney",
	"melbourne":  "Australia/Melbourne",
	"brisbane":   "Australia/Brisbane",
	"perth":      "Australia/Perth",
	"auckland":   "Pacific/Auckland",
	"wellington": "Pacific/Auckland",

	// Africa
	"cairo":        "Africa/Cairo",
	"johannesburg": "Africa/Johannesburg",
	"cape town":    "Africa/Johannesburg",
	"lagos":        "Africa/Lagos",
	"nairobi":      "Africa/Nairobi",
	"casablanca":   "Africa/Casablanca",

	// Common abbreviations
	"utc": "UTC",
	"gmt": "GMT",
}

// handleTimezoneToolCall handles timezone tool calls
func handleTimezoneToolCall(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Timezone     string `json:"timezone"`
		ConvertTime  string `json:"convert_time"`
		FromTimezone string `json:"from_timezone"`
	}

	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received timezone tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse timezone tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	if args.Timezone == "" {
		return "", fmt.Errorf("timezone is required")
	}

	// Resolve target timezone
	targetLoc, err := resolveTimezone(args.Timezone)
	if err != nil {
		log.Debug().Ctx(ctx).Err(err).Str("timezone", args.Timezone).Msg("Failed to resolve target timezone")
		return "", err
	}

	var sb strings.Builder

	if args.ConvertTime != "" {
		// Time conversion mode
		fromLoc := time.UTC
		if args.FromTimezone != "" {
			fromLoc, err = resolveTimezone(args.FromTimezone)
			if err != nil {
				log.Debug().Ctx(ctx).Err(err).Str("from_timezone", args.FromTimezone).Msg("Failed to resolve source timezone")
				return "", fmt.Errorf("invalid source timezone: %w", err)
			}
		}

		// Parse the time to convert
		parsedTime, err := parseTimeInput(args.ConvertTime, fromLoc)
		if err != nil {
			return "", fmt.Errorf("failed to parse time '%s': %w", args.ConvertTime, err)
		}

		// Convert to target timezone
		convertedTime := parsedTime.In(targetLoc)

		sb.WriteString("**Time Conversion**\n\n")
		sb.WriteString(fmt.Sprintf("From: %s (%s)\n", parsedTime.Format("Monday, 2 January 2006 15:04:05 MST"), fromLoc.String()))
		sb.WriteString(fmt.Sprintf("To: %s (%s)\n", convertedTime.Format("Monday, 2 January 2006 15:04:05 MST"), targetLoc.String()))

		// Show UTC offset difference
		_, fromOffset := parsedTime.Zone()
		_, toOffset := convertedTime.Zone()
		diffHours := float64(toOffset-fromOffset) / 3600
		if diffHours >= 0 {
			sb.WriteString(fmt.Sprintf("\nTime difference: +%.1f hours\n", diffHours))
		} else {
			sb.WriteString(fmt.Sprintf("\nTime difference: %.1f hours\n", diffHours))
		}
	} else {
		// Current time mode
		now := time.Now().In(targetLoc)
		zoneName, offset := now.Zone()
		offsetHours := float64(offset) / 3600

		sb.WriteString(fmt.Sprintf("**Current time in %s**\n\n", targetLoc.String()))
		sb.WriteString(fmt.Sprintf("%s\n", now.Format("Monday, 2 January 2006")))
		sb.WriteString(fmt.Sprintf("%s\n", now.Format("15:04:05")))
		sb.WriteString(fmt.Sprintf("\nTimezone: %s (UTC", zoneName))
		if offsetHours >= 0 {
			sb.WriteString(fmt.Sprintf("+%.0f)\n", offsetHours))
		} else {
			sb.WriteString(fmt.Sprintf("%.0f)\n", offsetHours))
		}

		// Also show UTC time for reference
		utcNow := time.Now().UTC()
		sb.WriteString(fmt.Sprintf("\nUTC: %s\n", utcNow.Format("15:04:05")))
	}

	result := sb.String()
	log.Debug().Ctx(ctx).Str("timezone", args.Timezone).Int("response_length", len(result)).Msg("Timezone lookup completed")

	return result, nil
}

// resolveTimezone attempts to resolve a timezone string to a *time.Location
func resolveTimezone(tz string) (*time.Location, error) {
	tzLower := strings.ToLower(strings.TrimSpace(tz))

	// First, check if it's a known city name
	if ianaName, ok := cityToTimezone[tzLower]; ok {
		loc, err := time.LoadLocation(ianaName)
		if err != nil {
			return nil, fmt.Errorf("failed to load timezone %s: %w", ianaName, err)
		}
		return loc, nil
	}

	// Try to load it directly as an IANA timezone
	loc, err := time.LoadLocation(tz)
	if err == nil {
		return loc, nil
	}

	// Provide a helpful error message with suggestions
	return nil, fmt.Errorf("unknown timezone '%s'. Use IANA format (e.g., Europe/Helsinki, America/New_York) or a city name (e.g., Tokyo, London, Helsinki)", tz)
}

// parseTimeInput parses a time string in various formats
func parseTimeInput(timeStr string, loc *time.Location) (time.Time, error) {
	timeStr = strings.TrimSpace(timeStr)

	// Try various formats
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"02.01.2006 15:04:05",
		"02.01.2006 15:04",
		"15:04:05",
		"15:04",
	}

	for _, format := range formats {
		if t, err := time.ParseInLocation(format, timeStr, loc); err == nil {
			// For time-only formats, use today's date
			if format == "15:04:05" || format == "15:04" {
				now := time.Now().In(loc)
				t = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc)
			}
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("could not parse time, try formats like '15:30' or '2024-01-15 09:00'")
}
