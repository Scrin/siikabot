package llmtools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/fmi"
	"github.com/Scrin/siikabot/metrics"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// The kinds of data the FMI tool provides
const (
	fmiKindCurrent     = "current"
	fmiKindForecast    = "forecast"
	fmiKindWarnings    = "warnings"
	fmiKindAirQuality  = "air_quality"
	fmiKindSun         = "sun"
	fmiKindSea         = "sea"
	fmiKindHistory     = "history"
	fmiKindLightning   = "lightning"
	fmiKindHeatingDays = "heating_degree_days"
)

var fmiKinds = []string{
	fmiKindCurrent, fmiKindForecast, fmiKindWarnings, fmiKindAirQuality, fmiKindSun, fmiKindSea,
	fmiKindHistory, fmiKindLightning, fmiKindHeatingDays,
}

// FMIToolDefinition is the tool for the Finnish Meteorological Institute's open data
var FMIToolDefinition = aigateway.ToolDefinition{
	Type: "function",
	Function: aigateway.FunctionSchema{
		Name: "get_fmi_data",
		Description: "Weather and environment data for Finland from the Finnish Meteorological Institute (FMI). " +
			"Use it instead of fetching weather, warning, air quality or sea web pages. Kinds: " +
			"current: latest observations at the nearest weather stations. " +
			"forecast: FMI's official forecast, with any active warnings. " +
			"warnings: active weather warnings for a place, or all of Finland without a location. " +
			"air_quality: the nearest air quality stations and a forecast. " +
			"sun: sunrise, sunset and day length. " +
			"sea: sea level, water temperature and waves at the nearest coast. " +
			"history: past days' temperatures, precipitation and snow, compared with the 1991-2020 normal. " +
			"lightning: recent lightning strikes nearby. " +
			"heating_degree_days: monthly heating degree days for the past year.",
		Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"kind": {
						"type": "string",
						"enum": ["current", "forecast", "warnings", "air_quality", "sun", "sea", "history", "lightning", "heating_degree_days"],
						"description": "What data to get"
					},
					"location": {
						"type": "string",
						"description": "A place in Finland, such as Helsinki, or a district and city, such as Kallio,Helsinki. Required except for warnings across Finland."
					},
					"date": {
						"type": "string",
						"description": "YYYY-MM-DD. sun: the day (default today). history: the last day to include (default yesterday)."
					},
					"days": {
						"type": "integer",
						"description": "forecast: days ahead, 1-10 (default 2). history: how many days up to date, 1-31 (default 1). lightning: how many past days, 1-7 (default 1)."
					}
				},
				"required": ["kind"]
			}`),
	},
	Handler:          handleFMIToolCall,
	ValidityDuration: 15 * time.Minute,
}

func init() {
	for _, kind := range fmiKinds {
		metrics.InitializeFMIKind(kind)
	}
}

// fmiCredit credits FMI, which its CC BY 4.0 licence requires
const fmiCredit = "Source: Finnish Meteorological Institute open data (CC BY 4.0)."

// Limits on the time spans the tool accepts. A request outside them is clamped rather than refused.
const (
	fmiDefaultForecastDays   = 2
	fmiMaxForecastDays       = 10
	fmiMaxHistoryDays        = 31
	fmiMaxLightningDays      = 7
	fmiLightningRadiusKm     = 50
	fmiForecastHours         = 48 // for air quality and the sea
	fmiHeatingMonths         = 12
	fmiDetailedForecastHours = 24
	fmiDetailedForecastStep  = 3
)

// fmiRequest is a validated tool call, with defaults applied
type fmiRequest struct {
	kind     string
	location string
	date     time.Time // a calendar date at midnight in the bot's time zone; zero when the kind takes none
	days     int
}

func handleFMIToolCall(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Kind     string `json:"kind"`
		Location string `json:"location"`
		Date     string `json:"date"`
		Days     int    `json:"days"`
	}

	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received FMI data tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	loc := botLocation()
	request, err := parseFMIRequest(args.Kind, args.Location, args.Date, args.Days, time.Now(), loc)
	if err != nil {
		metrics.RecordFMIRequest("invalid", false, 0)
		return "", err
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("siikabot.fmi.kind", request.kind))

	start := time.Now()
	result, err := fmiData(ctx, request, loc)
	metrics.RecordFMIRequest(request.kind, err == nil, time.Since(start).Seconds())
	if err != nil {
		// Already logged where it happened, and the tool registry logs the failed call
		return "", err
	}
	return result + "\n\n" + fmiCredit, nil
}

// botLocation is the bot's time zone, in which the tool reads dates and writes times
func botLocation() *time.Location {
	loc, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// parseFMIRequest validates a tool call and applies the defaults
func parseFMIRequest(kind, location, date string, days int, now time.Time, loc *time.Location) (fmiRequest, error) {
	request := fmiRequest{
		kind:     strings.ToLower(strings.TrimSpace(kind)),
		location: strings.TrimSpace(location),
		days:     days,
	}
	if !slices.Contains(fmiKinds, request.kind) {
		return request, fmt.Errorf("unknown kind %q, use one of: %s", kind, strings.Join(fmiKinds, ", "))
	}
	if request.location == "" && request.kind != fmiKindWarnings {
		return request, fmt.Errorf("location is required for %s", request.kind)
	}

	local := now.In(loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	if date = strings.TrimSpace(date); date != "" {
		parsed, err := time.ParseInLocation("2006-01-02", date, loc)
		if err != nil {
			return request, fmt.Errorf("date must be in YYYY-MM-DD format, got %q", date)
		}
		request.date = parsed
	}

	switch request.kind {
	case fmiKindForecast:
		request.days = clamp(request.days, fmiDefaultForecastDays, fmiMaxForecastDays)
	case fmiKindHistory:
		request.days = clamp(request.days, 1, fmiMaxHistoryDays)
		if request.date.IsZero() {
			request.date = today.AddDate(0, 0, -1)
		}
		if !request.date.Before(today) {
			return request, fmt.Errorf("history covers days that have ended, so the date must be before today (%s)",
				today.Format("2006-01-02"))
		}
	case fmiKindLightning:
		request.days = clamp(request.days, 1, fmiMaxLightningDays)
	case fmiKindSun:
		if request.date.IsZero() {
			request.date = today
		}
	}
	return request, nil
}

// clamp returns value within 1..max, or the default when it wasn't given
func clamp(value, defaultValue, max int) int {
	switch {
	case value <= 0:
		return defaultValue
	case value > max:
		return max
	default:
		return value
	}
}

// fmiData gets and formats the data a request asks for
func fmiData(ctx context.Context, request fmiRequest, loc *time.Location) (string, error) {
	if request.kind == fmiKindWarnings && request.location == "" {
		warnings, err := fmi.ActiveWarnings(ctx)
		if err != nil {
			return "", err
		}
		return formatFMIWarnings("", warnings, loc), nil
	}

	place, err := fmi.ResolvePlace(ctx, request.location)
	if err != nil {
		return "", err
	}

	switch request.kind {
	case fmiKindCurrent:
		observations, err := fmi.CurrentWeather(ctx, place)
		if err != nil {
			return "", err
		}
		return formatFMICurrent(place, observations, loc), nil

	case fmiKindForecast:
		hours, err := fmi.Forecast(ctx, place, request.days*24)
		if err != nil {
			return "", err
		}
		// A forecast is still worth giving when the warnings can't be checked
		warnings, warningsErr := fmi.WarningsFor(ctx, place)
		return formatFMIForecast(place, hours, warnings, warningsErr == nil, loc), nil

	case fmiKindWarnings:
		warnings, err := fmi.WarningsFor(ctx, place)
		if err != nil {
			return "", err
		}
		return formatFMIWarnings(place.Label(), warnings, loc), nil

	case fmiKindAirQuality:
		stations, err := fmi.AirQualityNow(ctx, place)
		if err != nil {
			return "", err
		}
		forecast, forecastErr := fmi.AirQualityForecastFor(ctx, place, fmiForecastHours)
		return formatFMIAirQuality(place, stations, forecast, forecastErr == nil, loc), nil

	case fmiKindSun:
		sun := fmi.SunTimes(request.date.Year(), request.date.Month(), request.date.Day(), place.Lat, place.Lon)
		return formatFMISun(place, request.date, sun, loc), nil

	case fmiKindSea:
		report, err := fmi.Sea(ctx, place, fmiForecastHours)
		if err != nil {
			return "", err
		}
		return formatFMISea(place, report, loc), nil

	case fmiKindHistory:
		first := request.date.AddDate(0, 0, -(request.days - 1))
		history, err := fmi.WeatherHistory(ctx, place, first, request.date)
		if err != nil {
			return "", err
		}
		return formatFMIHistory(place, history, first, request.date, loc), nil

	case fmiKindLightning:
		since := time.Now().Add(-time.Duration(request.days) * 24 * time.Hour)
		strikes, err := fmi.Lightning(ctx, place, since, fmiLightningRadiusKm)
		if err != nil {
			return "", err
		}
		return formatFMILightning(place, strikes, request.days, loc), nil

	case fmiKindHeatingDays:
		report, err := fmi.HeatingDegreeDays(ctx, place, fmiHeatingMonths)
		if err != nil {
			return "", err
		}
		return formatFMIHeating(place, report, time.Now(), loc), nil
	}
	return "", fmt.Errorf("unknown kind %q", request.kind)
}

// Formatting helpers

// hourMinute writes a time as "Sat 14:05" in the bot's time zone
func hourMinute(t time.Time, loc *time.Location) string {
	return t.In(loc).Format("Mon 15:04")
}

// dayLabel writes a date as "Mon 5 Oct"
func dayLabel(t time.Time, loc *time.Location) string {
	return t.In(loc).Format("Mon 2 Jan")
}

// compass names the direction of a wind or waves, given in degrees, with one of eight compass points
func compass(degrees float64) string {
	points := []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}
	return points[int(math.Mod(degrees+22.5+360, 360)/45)%8]
}

// distanceNote says how far a station is from the place asked about, as "Helsinki Kaisaniemi, 0.8 km away"
func distanceNote(station string, km float64) string {
	return fmt.Sprintf("%s, %.1f km away", station, km)
}

func formatFMICurrent(place fmi.Place, observations []fmi.Observation, loc *time.Location) string {
	if len(observations) == 0 {
		return fmt.Sprintf("No weather station near %s reported in the last hour.", place.Label())
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "**Current weather near %s** (FMI observations)\n", place.Label())
	for _, o := range observations {
		var parts []string
		if o.Temperature != nil {
			parts = append(parts, fmt.Sprintf("%.1f °C", *o.Temperature))
		}
		if o.WindSpeed != nil {
			wind := fmt.Sprintf("wind %.1f m/s", *o.WindSpeed)
			if o.WindDirection != nil {
				wind += " from " + compass(*o.WindDirection)
			}
			if o.WindGust != nil {
				wind += fmt.Sprintf(", gusts %.1f m/s", *o.WindGust)
			}
			parts = append(parts, wind)
		}
		if o.Humidity != nil {
			parts = append(parts, fmt.Sprintf("humidity %.0f %%", *o.Humidity))
		}
		if o.Precipitation != nil {
			parts = append(parts, fmt.Sprintf("precipitation %.1f mm in the last hour", *o.Precipitation))
		}
		if o.CloudCover != nil {
			if *o.CloudCover > 8 {
				// FMI reports 9 when fog or snowfall hides the sky
				parts = append(parts, "sky obscured")
			} else {
				parts = append(parts, fmt.Sprintf("cloud cover %.0f/8", *o.CloudCover))
			}
		}
		if o.Visibility != nil {
			parts = append(parts, fmt.Sprintf("visibility %.0f km", *o.Visibility/1000))
		}
		if o.Pressure != nil {
			parts = append(parts, fmt.Sprintf("pressure %.1f hPa", *o.Pressure))
		}
		if o.SnowDepth != nil {
			if *o.SnowDepth > 0 {
				parts = append(parts, fmt.Sprintf("snow depth %.0f cm", *o.SnowDepth))
			} else {
				parts = append(parts, "no snow")
			}
		}
		fmt.Fprintf(&sb, "- %s, at %s: %s\n", distanceNote(o.Station, o.DistanceKm), o.Time.In(loc).Format("15:04"),
			strings.Join(parts, ", "))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatFMIForecast(place fmi.Place, hours []fmi.ForecastHour, warnings []fmi.Warning, warningsChecked bool,
	loc *time.Location) string {
	if len(hours) == 0 {
		return fmt.Sprintf("FMI has no forecast for %s.", place.Label())
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "**FMI forecast for %s**\n", place.Label())

	detailed := hours[:min(fmiDetailedForecastHours, len(hours))]
	fmt.Fprintf(&sb, "Next %d hours:\n", len(detailed))
	for i := 0; i < len(detailed); i += fmiDetailedForecastStep {
		h := detailed[i]
		var parts []string
		if h.Temperature != nil {
			parts = append(parts, fmt.Sprintf("%.0f °C", *h.Temperature))
		}
		if h.Weather != "" {
			parts = append(parts, h.Weather)
		}
		if h.WindSpeed != nil {
			wind := fmt.Sprintf("wind %.0f m/s", *h.WindSpeed)
			if h.WindDirection != nil {
				wind += " from " + compass(*h.WindDirection)
			}
			if h.WindGust != nil {
				wind += fmt.Sprintf(" (gusts %.0f)", *h.WindGust)
			}
			parts = append(parts, wind)
		}
		if h.Precipitation != nil {
			precipitation := fmt.Sprintf("precipitation %.1f mm/h", *h.Precipitation)
			if h.PrecipitationProbability != nil {
				precipitation += fmt.Sprintf(" (probability %.0f %%)", *h.PrecipitationProbability)
			}
			parts = append(parts, precipitation)
		}
		if h.ThunderProbability != nil && *h.ThunderProbability >= 5 {
			parts = append(parts, fmt.Sprintf("thunder probability %.0f %%", *h.ThunderProbability))
		}
		fmt.Fprintf(&sb, "- %s: %s\n", hourMinute(h.Time, loc), strings.Join(parts, ", "))
	}

	// The detailed hours already cover the rest of the day they end in
	rest := hours[len(detailed):]
	for len(rest) > 0 && rest[0].Time.In(loc).Hour() != 0 {
		rest = rest[1:]
	}
	if len(rest) > 0 {
		sb.WriteString("Following days:\n")
		for _, day := range groupByDay(rest, loc) {
			sb.WriteString("- " + summariseForecastDay(day, loc) + "\n")
		}
	}

	switch {
	case !warningsChecked:
		sb.WriteString("Active warnings: couldn't be checked.")
	case len(warnings) == 0:
		sb.WriteString("Active warnings: none.")
	default:
		sb.WriteString("Active warnings:\n")
		sb.WriteString(warningLines(warnings))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// groupByDay splits forecast hours into calendar days in the bot's time zone
func groupByDay(hours []fmi.ForecastHour, loc *time.Location) [][]fmi.ForecastHour {
	var days [][]fmi.ForecastHour
	for _, h := range hours {
		if n := len(days); n > 0 && sameDay(days[n-1][0].Time, h.Time, loc) {
			days[n-1] = append(days[n-1], h)
		} else {
			days = append(days, []fmi.ForecastHour{h})
		}
	}
	return days
}

func sameDay(a, b time.Time, loc *time.Location) bool {
	a, b = a.In(loc), b.In(loc)
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

// summariseForecastDay sums up a day of forecast hours on one line
func summariseForecastDay(hours []fmi.ForecastHour, loc *time.Location) string {
	minTemp, maxTemp := math.Inf(1), math.Inf(-1)
	var precipitation, maxProbability, maxWind, maxGust, maxThunder float64
	weather := ""
	for _, h := range hours {
		if h.Temperature != nil {
			minTemp, maxTemp = math.Min(minTemp, *h.Temperature), math.Max(maxTemp, *h.Temperature)
		}
		if h.Precipitation != nil {
			precipitation += *h.Precipitation
		}
		maxProbability = maxOf(maxProbability, h.PrecipitationProbability)
		maxWind = maxOf(maxWind, h.WindSpeed)
		maxGust = maxOf(maxGust, h.WindGust)
		maxThunder = maxOf(maxThunder, h.ThunderProbability)
		// The weather at midday describes the day best
		if local := h.Time.In(loc); h.Weather != "" && (weather == "" || local.Hour() <= 12) {
			weather = h.Weather
		}
	}

	parts := []string{}
	if !math.IsInf(minTemp, 0) {
		parts = append(parts, fmt.Sprintf("%.0f to %.0f °C", minTemp, maxTemp))
	}
	if weather != "" {
		parts = append(parts, weather+" at midday")
	}
	parts = append(parts, fmt.Sprintf("precipitation %.1f mm (probability up to %.0f %%)", precipitation, maxProbability))
	parts = append(parts, fmt.Sprintf("wind up to %.0f m/s, gusts up to %.0f m/s", maxWind, maxGust))
	if maxThunder >= 5 {
		parts = append(parts, fmt.Sprintf("thunder probability up to %.0f %%", maxThunder))
	}
	return dayLabel(hours[0].Time, loc) + ": " + strings.Join(parts, ", ")
}

// maxOf returns the larger of a running maximum and an optional value
func maxOf(current float64, value *float64) float64 {
	if value != nil && *value > current {
		return *value
	}
	return current
}

// maxWarningLines caps how many warnings are listed. In a storm FMI can have dozens in force across Finland.
const maxWarningLines = 20

// warningLines lists warnings, one per line
func warningLines(warnings []fmi.Warning) string {
	var sb strings.Builder
	for i, w := range warnings {
		if i == maxWarningLines {
			fmt.Fprintf(&sb, "- …and %d more\n", len(warnings)-maxWarningLines)
			break
		}
		line := w.Headline
		if w.Description != "" && !strings.Contains(w.Headline, w.Description) {
			line += ". " + w.Description
		}
		sb.WriteString("- " + line + "\n")
	}
	return sb.String()
}

func formatFMIWarnings(placeLabel string, warnings []fmi.Warning, loc *time.Location) string {
	if placeLabel == "" {
		if len(warnings) == 0 {
			return "FMI has no active warnings in Finland."
		}
		return strings.TrimRight("**Active FMI warnings in Finland**\n"+warningLines(warnings), "\n")
	}
	if len(warnings) == 0 {
		return fmt.Sprintf("No active FMI warning covers %s.", placeLabel)
	}
	return strings.TrimRight(fmt.Sprintf("**Active FMI warnings for %s**\n", placeLabel)+warningLines(warnings), "\n")
}

// airQualityClasses names the classes of FMI's air quality index, from 1
var airQualityClasses = []string{"", "good", "satisfactory", "fair", "poor", "very poor"}

func airQualityClass(index int) string {
	if index < 1 || index >= len(airQualityClasses) {
		return "unknown"
	}
	return airQualityClasses[index]
}

func formatFMIAirQuality(place fmi.Place, stations []fmi.AirQualityStation, forecast fmi.AirQualityForecast,
	forecastOK bool, loc *time.Location) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**Air quality near %s** (FMI)\n", place.Label())
	sb.WriteString("Index: 1 good, 2 satisfactory, 3 fair, 4 poor, 5 very poor. Concentrations in µg/m³, hourly averages.\n")

	if len(stations) == 0 {
		fmt.Fprintf(&sb, "No air quality station near %s reported in the last few hours.\n", place.Label())
	}
	for _, s := range stations {
		var parts []string
		if s.Index != nil {
			parts = append(parts, fmt.Sprintf("index %.0f (%s)", *s.Index, airQualityClass(int(*s.Index))))
		}
		for _, c := range []struct {
			name  string
			value *float64
		}{{"PM2.5", s.PM25}, {"PM10", s.PM10}, {"NO2", s.NO2}, {"O3", s.O3}, {"SO2", s.SO2}, {"black carbon", s.BlackCarbon}} {
			if c.value != nil {
				parts = append(parts, fmt.Sprintf("%s %.1f", c.name, *c.value))
			}
		}
		fmt.Fprintf(&sb, "- %s, at %s: %s\n", distanceNote(s.Station, s.DistanceKm), s.Time.In(loc).Format("15:04"),
			strings.Join(parts, ", "))
	}

	switch {
	case !forecastOK:
		sb.WriteString("Forecast: not available.\n")
	case len(forecast.Hours) > 0:
		sb.WriteString(summariseAirQualityForecast(forecast, loc) + "\n")
	}
	sb.WriteString("The city stations are run by the municipalities.")
	return sb.String()
}

// summariseAirQualityForecast sums up a forecast: either that it stays good, and what comes closest to
// satisfactory, or when it's worse and why
func summariseAirQualityForecast(forecast fmi.AirQualityForecast, loc *time.Location) string {
	model := "FMI's ENFUSER street-level model"
	if forecast.Model == "SILAM" {
		model = "FMI's SILAM regional model; street-level traffic peaks can be higher"
	}
	header := fmt.Sprintf("Forecast for the next %d hours (%s): ", len(forecast.Hours), model)

	type period struct {
		index      int
		start, end time.Time
		pollutant  string
		peak       float64
	}
	var periods []period
	var closest *fmi.AirQualityHour
	closestShare := 0.0
	for i, h := range forecast.Hours {
		if limit, ok := fmi.SatisfactoryLimit(h.Pollutant); ok && h.Concentration/limit > closestShare {
			closest, closestShare = &forecast.Hours[i], h.Concentration/limit
		}
		if h.Index < 2 {
			continue
		}
		if n := len(periods); n > 0 && periods[n-1].index == h.Index && periods[n-1].pollutant == h.Pollutant &&
			h.Time.Sub(periods[n-1].end) <= time.Hour {
			periods[n-1].end = h.Time
			periods[n-1].peak = math.Max(periods[n-1].peak, h.Concentration)
			continue
		}
		periods = append(periods, period{index: h.Index, start: h.Time, end: h.Time, pollutant: h.Pollutant, peak: h.Concentration})
	}

	if len(periods) == 0 {
		summary := header + "good throughout."
		if closest != nil {
			limit, _ := fmi.SatisfactoryLimit(closest.Pollutant)
			summary += fmt.Sprintf(" Closest to satisfactory: %s %.0f µg/m³ at %s (satisfactory from %.0f).",
				closest.Pollutant, closest.Concentration, hourMinute(closest.Time, loc), limit)
		}
		return summary
	}

	const maxPeriods = 8
	var parts []string
	for i, p := range periods {
		if i == maxPeriods {
			parts = append(parts, fmt.Sprintf("and %d more periods", len(periods)-maxPeriods))
			break
		}
		span := hourMinute(p.start, loc)
		if !p.end.Equal(p.start) {
			span += "–" + p.end.Add(time.Hour).In(loc).Format("15:04")
		}
		parts = append(parts, fmt.Sprintf("%s %s (%s up to %.0f µg/m³)", airQualityClass(p.index), span, p.pollutant, p.peak))
	}
	return header + "good except " + strings.Join(parts, "; ") + "."
}

func formatFMISun(place fmi.Place, date time.Time, sun fmi.SunDay, loc *time.Location) string {
	header := fmt.Sprintf("**Sun in %s on %s**\n", place.Label(), date.Format("Mon 2 Jan 2006"))
	switch {
	case sun.PolarNight:
		return header + "The sun doesn't rise: polar night."
	case sun.MidnightSun:
		return header + "The sun doesn't set: midnight sun."
	}
	length := sun.DayLength.Round(time.Minute)
	return header + fmt.Sprintf("Sunrise %s, sunset %s, day length %d h %d min.",
		sun.Sunrise.In(loc).Format("15:04"), sun.Sunset.In(loc).Format("15:04"),
		int(length.Hours()), int(length.Minutes())%60)
}

func formatFMISea(place fmi.Place, report fmi.SeaReport, loc *time.Location) string {
	if report.SeaLevel == nil && report.Waves == nil {
		return fmt.Sprintf("No sea station near %s reported in the last few hours.", place.Label())
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "**Sea near %s** (FMI)\n", place.Label())

	if s := report.SeaLevel; s != nil {
		var parts []string
		if s.SeaLevel != nil {
			parts = append(parts, fmt.Sprintf("sea level %+.0f cm compared with the theoretical mean sea level", *s.SeaLevel))
		}
		if s.WaterTemperature != nil {
			parts = append(parts, fmt.Sprintf("water %.1f °C", *s.WaterTemperature))
		}
		fmt.Fprintf(&sb, "- %s, at %s: %s\n", distanceNote(s.Station, s.DistanceKm), hourMinute(s.Time, loc), strings.Join(parts, ", "))
	}
	if len(report.SeaLevelForecast) > 0 {
		high, low := report.SeaLevelForecast[0], report.SeaLevelForecast[0]
		for _, p := range report.SeaLevelForecast {
			if p.SeaLevel > high.SeaLevel {
				high = p
			}
			if p.SeaLevel < low.SeaLevel {
				low = p
			}
		}
		fmt.Fprintf(&sb, "- Sea level forecast for the next %d hours: highest %+.0f cm at %s, lowest %+.0f cm at %s\n",
			len(report.SeaLevelForecast), high.SeaLevel, hourMinute(high.Time, loc), low.SeaLevel, hourMinute(low.Time, loc))
	}

	if b := report.Waves; b != nil {
		var parts []string
		if b.WaveHeight != nil {
			waves := fmt.Sprintf("waves %.1f m", *b.WaveHeight)
			if b.WavePeriod != nil {
				waves += fmt.Sprintf(", period %.1f s", *b.WavePeriod)
			}
			if b.WaveDirection != nil {
				waves += " from " + compass(*b.WaveDirection)
			}
			parts = append(parts, waves)
		} else {
			parts = append(parts, "not reporting waves now")
		}
		if b.WaterTemperature != nil {
			parts = append(parts, fmt.Sprintf("water %.1f °C", *b.WaterTemperature))
		}
		fmt.Fprintf(&sb, "- Buoy %s, at %s: %s\n", distanceNote(b.Station, b.DistanceKm), hourMinute(b.Time, loc), strings.Join(parts, ", "))
	}
	if len(report.WaveForecast) > 0 {
		var highest *fmi.WavePoint
		for i, p := range report.WaveForecast {
			if p.Height != nil && (highest == nil || *p.Height > *highest.Height) {
				highest = &report.WaveForecast[i]
			}
		}
		if highest != nil {
			fmt.Fprintf(&sb, "- Wave forecast at the buoy for the next %d hours: highest %.1f m at %s\n",
				len(report.WaveForecast), *highest.Height, hourMinute(highest.Time, loc))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatFMIHistory(place fmi.Place, history fmi.History, first, last time.Time, loc *time.Location) string {
	if len(history.Days) == 0 {
		return fmt.Sprintf("FMI has no daily statistics near %s for %s to %s yet.", place.Label(),
			first.Format("2006-01-02"), last.Format("2006-01-02"))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "**Daily weather at %s from %s** (FMI daily statistics)\n",
		distanceNote(history.Station, history.DistanceKm), place.Label())
	var months []time.Month
	for _, d := range history.Days {
		var parts []string
		if d.Mean != nil {
			mean := fmt.Sprintf("mean %.1f °C", *d.Mean)
			if d.Min != nil && d.Max != nil {
				mean += fmt.Sprintf(" (%.1f to %.1f)", *d.Min, *d.Max)
			}
			parts = append(parts, mean)
		} else if d.Min != nil && d.Max != nil {
			parts = append(parts, fmt.Sprintf("%.1f to %.1f °C", *d.Min, *d.Max))
		}
		if d.Precipitation != nil {
			if *d.Precipitation > 0 {
				parts = append(parts, fmt.Sprintf("precipitation %.1f mm", *d.Precipitation))
			} else {
				parts = append(parts, "no precipitation")
			}
		}
		if d.SnowDepth != nil {
			if *d.SnowDepth > 0 {
				parts = append(parts, fmt.Sprintf("snow depth %.0f cm", *d.SnowDepth))
			} else {
				parts = append(parts, "no snow")
			}
		}
		// The date is the day the statistics are for, stamped at midnight UTC, so it's read in UTC
		fmt.Fprintf(&sb, "- %s: %s\n", d.Date.UTC().Format("Mon 2 Jan"), strings.Join(parts, ", "))
		if month := d.Date.UTC().Month(); !slices.Contains(months, month) {
			months = append(months, month)
		}
	}

	if lastDay := history.Days[len(history.Days)-1].Date.UTC(); lastDay.Before(time.Date(last.Year(), last.Month(), last.Day(), 0, 0, 0, 0, time.UTC)) {
		fmt.Fprintf(&sb, "No statistics yet for the days after %s.\n", lastDay.Format("Mon 2 Jan"))
	}

	for _, month := range months {
		normal, ok := history.Normals[month]
		if !ok {
			continue
		}
		var parts []string
		if normal.MeanTemperature != nil {
			parts = append(parts, fmt.Sprintf("mean %.1f °C", *normal.MeanTemperature))
		}
		if normal.Precipitation != nil {
			parts = append(parts, fmt.Sprintf("%.0f mm of precipitation in the month", *normal.Precipitation))
		}
		fmt.Fprintf(&sb, "1991-2020 normal for %s at this station: %s\n", month, strings.Join(parts, ", "))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatFMILightning(place fmi.Place, strikes []fmi.Strike, days int, loc *time.Location) string {
	period := "the last 24 hours"
	if days > 1 {
		period = fmt.Sprintf("the last %d days", days)
	}
	header := fmt.Sprintf("**Lightning within %d km of %s in %s** (FMI)\n", fmiLightningRadiusKm, place.Label(), period)
	if len(strikes) == 0 {
		return header + "No strikes."
	}

	nearest := strikes[0]
	for _, s := range strikes {
		if s.DistanceKm < nearest.DistanceKm {
			nearest = s
		}
	}
	latest := strikes[0]
	return header + fmt.Sprintf("%d strikes. The latest at %s, %.0f km away. The nearest %.0f km away, at %s.",
		len(strikes), hourMinute(latest.Time, loc), latest.DistanceKm, nearest.DistanceKm, hourMinute(nearest.Time, loc))
}

func formatFMIHeating(place fmi.Place, report fmi.HeatingReport, now time.Time, loc *time.Location) string {
	if len(report.Months) == 0 {
		return fmt.Sprintf("FMI has no heating degree days near %s.", place.Label())
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "**Heating degree days (lämmitystarveluku) at %s from %s** (FMI)\n",
		distanceNote(report.Station, report.DistanceKm), place.Label())
	for _, m := range report.Months {
		line := fmt.Sprintf("- %s: %.0f", m.Month.Format("Jan 2006"), m.DegreeDays)
		daysInMonth := m.Month.AddDate(0, 1, -1).Day()
		if m.Days < daysInMonth {
			if m.Month.Year() == now.UTC().Year() && m.Month.Month() == now.UTC().Month() {
				line += fmt.Sprintf(" so far (%d days)", m.Days)
			} else {
				line += fmt.Sprintf(" (%d of %d days)", m.Days, daysInMonth)
			}
		}
		sb.WriteString(line + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}
