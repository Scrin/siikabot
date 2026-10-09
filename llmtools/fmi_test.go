package llmtools

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/fmi"
)

func helsinkiLocation(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		t.Skipf("no time zone data: %v", err)
	}
	return loc
}

func ptr(v float64) *float64 {
	return &v
}

func assertContains(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
}

var testPlace = fmi.Place{Name: "Helsinki", Region: "Suomi", Lat: 60.17, Lon: 24.94}

func TestFMIToolSchemaListsEveryKind(t *testing.T) {
	var schema struct {
		Properties struct {
			Kind struct {
				Enum []string `json:"enum"`
			} `json:"kind"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(FMIToolDefinition.Function.Parameters, &schema); err != nil {
		t.Fatalf("parameters aren't valid JSON: %v", err)
	}
	if !slices.Equal(schema.Properties.Kind.Enum, fmiKinds) {
		t.Errorf("schema lists %v, the tool handles %v", schema.Properties.Kind.Enum, fmiKinds)
	}
	for _, kind := range fmiKinds {
		if !strings.Contains(FMIToolDefinition.Function.Description, kind+":") {
			t.Errorf("the description doesn't explain %s", kind)
		}
	}
}

func TestParseFMIRequest(t *testing.T) {
	loc := helsinkiLocation(t)
	now := time.Date(2026, 10, 9, 22, 30, 0, 0, loc)

	tests := []struct {
		name                 string
		kind, location, date string
		days                 int
		wantErr              string
		wantKind             string
		wantDays             int
		wantDate             string
	}{
		{name: "unknown kind", kind: "pollen", location: "Helsinki", wantErr: "unknown kind"},
		{name: "location missing", kind: "current", wantErr: "location is required"},
		{name: "warnings for all of Finland", kind: "warnings", wantKind: "warnings"},
		{name: "kind is case-insensitive", kind: " Current ", location: "Oulu", wantKind: "current"},
		{name: "forecast default", kind: "forecast", location: "Oulu", wantKind: "forecast", wantDays: 2},
		{name: "forecast clamped", kind: "forecast", location: "Oulu", days: 30, wantKind: "forecast", wantDays: 10},
		{name: "history defaults to yesterday", kind: "history", location: "Oulu", wantKind: "history", wantDays: 1, wantDate: "2026-10-08"},
		{name: "history of today", kind: "history", location: "Oulu", date: "2026-10-09", wantErr: "before today"},
		{name: "history clamped", kind: "history", location: "Oulu", date: "2026-09-01", days: 90, wantKind: "history", wantDays: 31, wantDate: "2026-09-01"},
		{name: "bad date", kind: "sun", location: "Oulu", date: "9.10.2026", wantErr: "YYYY-MM-DD"},
		{name: "sun defaults to today", kind: "sun", location: "Oulu", wantKind: "sun", wantDate: "2026-10-09"},
		{name: "lightning clamped", kind: "lightning", location: "Oulu", days: 14, wantKind: "lightning", wantDays: 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := parseFMIRequest(tt.kind, tt.location, tt.date, tt.days, now, loc)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %v, want one about %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFMIRequest() error = %v", err)
			}
			if request.kind != tt.wantKind || request.days != tt.wantDays {
				t.Errorf("request = %+v, want kind %s and %d days", request, tt.wantKind, tt.wantDays)
			}
			if got := request.date; tt.wantDate != "" && got.Format("2006-01-02") != tt.wantDate {
				t.Errorf("date = %s, want %s", got.Format("2006-01-02"), tt.wantDate)
			}
		})
	}
}

func TestCompass(t *testing.T) {
	for degrees, want := range map[float64]string{0: "N", 22: "N", 23: "NE", 90: "E", 181: "S", 266: "W", 337.4: "NW", 337.6: "N", 360: "N"} {
		if got := compass(degrees); got != want {
			t.Errorf("compass(%v) = %s, want %s", degrees, got, want)
		}
	}
}

func TestFormatFMICurrent(t *testing.T) {
	loc := helsinkiLocation(t)
	observations := []fmi.Observation{{
		Station: "Helsinki Kaisaniemi", DistanceKm: 0.8, Time: time.Date(2026, 10, 9, 19, 40, 0, 0, time.UTC),
		Temperature: ptr(9.9), WindSpeed: ptr(1.9), WindGust: ptr(2.9), WindDirection: ptr(265), Humidity: ptr(87),
		Precipitation: ptr(0), CloudCover: ptr(9), Visibility: ptr(50000), SnowDepth: ptr(0), Pressure: ptr(987.1),
	}}

	got := formatFMICurrent(testPlace, observations, loc)
	assertContains(t, got,
		"Current weather near Helsinki**",
		"Helsinki Kaisaniemi, 0.8 km away, at 22:40",
		"9.9 °C", "wind 1.9 m/s from W, gusts 2.9 m/s", "humidity 87 %", "sky obscured", "visibility 50 km",
		"pressure 987.1 hPa", "no snow")

	if got := formatFMICurrent(testPlace, nil, loc); !strings.Contains(got, "No weather station") {
		t.Errorf("empty observations = %q", got)
	}
}

func TestFormatFMIForecast(t *testing.T) {
	loc := helsinkiLocation(t)
	start := time.Date(2026, 10, 9, 20, 0, 0, 0, time.UTC) // 23:00 in Helsinki
	var hours []fmi.ForecastHour
	for i := 0; i < 48; i++ {
		hours = append(hours, fmi.ForecastHour{
			Time: start.Add(time.Duration(i) * time.Hour), Temperature: ptr(float64(5 + i%6)), Weather: "overcast",
			WindSpeed: ptr(3), WindDirection: ptr(180), WindGust: ptr(7), Precipitation: ptr(0.2),
			PrecipitationProbability: ptr(40), ThunderProbability: ptr(0),
		})
	}

	got := formatFMIForecast(testPlace, hours, nil, true, loc)
	assertContains(t, got, "Next 24 hours:", "- Fri 23:00: 5 °C, overcast, wind 3 m/s from S (gusts 7)", "Following days:",
		"Active warnings: none.")
	if lines := strings.Count(got, " °C, overcast,"); lines != 8 {
		t.Errorf("got %d detailed lines, want one every 3 hours for 24 hours", lines)
	}
	// The detailed hours end Sat 22:00, so the daily lines start on Sunday, not with Saturday's last hour
	if strings.Contains(got, "- Sat 10 Oct:") || !strings.Contains(got, "- Sun 11 Oct: 5 to 10 °C, overcast at midday") {
		t.Errorf("daily lines don't start at the first full day:\n%s", got)
	}

	warnings := []fmi.Warning{{Headline: "Yellow wind warning for land areas: Uusimaa", Description: "Gusts up to 20 m/s."}}
	assertContains(t, formatFMIForecast(testPlace, hours, warnings, true, loc),
		"Active warnings:\n- Yellow wind warning for land areas: Uusimaa. Gusts up to 20 m/s.")
	assertContains(t, formatFMIForecast(testPlace, hours, nil, false, loc), "Active warnings: couldn't be checked.")
}

func TestFormatFMIWarnings(t *testing.T) {
	loc := helsinkiLocation(t)
	if got := formatFMIWarnings("Helsinki", nil, loc); got != "No active FMI warning covers Helsinki." {
		t.Errorf("no warnings for a place = %q", got)
	}
	if got := formatFMIWarnings("", nil, loc); got != "FMI has no active warnings in Finland." {
		t.Errorf("no warnings at all = %q", got)
	}

	var many []fmi.Warning
	for i := 0; i < maxWarningLines+3; i++ {
		many = append(many, fmi.Warning{Headline: "Yellow wind warning for sea area"})
	}
	assertContains(t, formatFMIWarnings("", many, loc), "Active FMI warnings in Finland", "…and 3 more")
}

func TestFormatFMIAirQuality(t *testing.T) {
	loc := helsinkiLocation(t)
	stations := []fmi.AirQualityStation{{
		Station: "Helsinki Mannerheimintie", DistanceKm: 0.2, Time: time.Date(2026, 10, 9, 19, 0, 0, 0, time.UTC),
		Index: ptr(1), PM25: ptr(2.6), PM10: ptr(6.2), NO2: ptr(14.1), BlackCarbon: ptr(0.2),
	}}
	start := time.Date(2026, 10, 9, 20, 0, 0, 0, time.UTC)

	good := fmi.AirQualityForecast{Model: "ENFUSER", Hours: []fmi.AirQualityHour{
		{Time: start, Index: 1, Pollutant: "O3", Concentration: 30},
		{Time: start.Add(time.Hour), Index: 1, Pollutant: "O3", Concentration: 56},
		{Time: start.Add(2 * time.Hour), Index: 1, Pollutant: "NO2", Concentration: 20},
	}}
	got := formatFMIAirQuality(testPlace, stations, good, true, loc)
	assertContains(t, got,
		"Index: 1 good, 2 satisfactory",
		"Helsinki Mannerheimintie, 0.2 km away, at 22:00: index 1 (good), PM2.5 2.6, PM10 6.2, NO2 14.1, black carbon 0.2",
		"ENFUSER street-level model): good throughout. Closest to satisfactory: O3 56 µg/m³ at Sat 00:00 (satisfactory from 60).",
		"run by the municipalities")

	worse := fmi.AirQualityForecast{Model: "SILAM", Hours: []fmi.AirQualityHour{
		{Time: start, Index: 1, Pollutant: "NO2", Concentration: 30},
		{Time: start.Add(time.Hour), Index: 2, Pollutant: "NO2", Concentration: 45},
		{Time: start.Add(2 * time.Hour), Index: 2, Pollutant: "NO2", Concentration: 52},
		{Time: start.Add(3 * time.Hour), Index: 1, Pollutant: "NO2", Concentration: 30},
	}}
	assertContains(t, formatFMIAirQuality(testPlace, stations, worse, true, loc),
		"SILAM regional model; street-level traffic peaks can be higher): good except satisfactory Sat 00:00–02:00 (NO2 up to 52 µg/m³).")

	assertContains(t, formatFMIAirQuality(testPlace, nil, fmi.AirQualityForecast{}, false, loc),
		"No air quality station near Helsinki", "Forecast: not available.")
}

func TestFormatFMISun(t *testing.T) {
	loc := helsinkiLocation(t)
	date := time.Date(2026, 10, 12, 0, 0, 0, 0, loc)
	sun := fmi.SunTimes(2026, time.October, 12, 66.5, 25.7166691)
	got := formatFMISun(fmi.Place{Name: "Rovaniemi"}, date, sun, loc)
	assertContains(t, got, "Sun in Rovaniemi on Mon 12 Oct 2026", "Sunrise 08:04, sunset 18:01, day length 9 h 57 min.")

	assertContains(t, formatFMISun(fmi.Place{Name: "Utsjoki"}, date, fmi.SunDay{PolarNight: true}, loc), "polar night")
	assertContains(t, formatFMISun(fmi.Place{Name: "Utsjoki"}, date, fmi.SunDay{MidnightSun: true}, loc), "midnight sun")
}

func TestFormatFMISea(t *testing.T) {
	loc := helsinkiLocation(t)
	at := time.Date(2026, 10, 9, 19, 0, 0, 0, time.UTC)
	report := fmi.SeaReport{
		SeaLevel: &fmi.SeaLevelStation{Station: "Helsinki Kaivopuisto", DistanceKm: 2.1, Time: at, SeaLevel: ptr(42.3), WaterTemperature: ptr(12.3)},
		SeaLevelForecast: []fmi.SeaLevelPoint{
			{Time: at, SeaLevel: 43.8}, {Time: at.Add(time.Hour), SeaLevel: 48.2}, {Time: at.Add(2 * time.Hour), SeaLevel: -5},
		},
		Waves:        &fmi.WaveBuoy{Station: "Suomenlinna", DistanceKm: 5.5, Time: at, WaterTemperature: ptr(12.8)},
		WaveForecast: []fmi.WavePoint{{Time: at, Height: ptr(0.4)}, {Time: at.Add(time.Hour), Height: ptr(1.2)}},
	}

	got := formatFMISea(testPlace, report, loc)
	assertContains(t, got,
		"Helsinki Kaivopuisto, 2.1 km away, at Fri 22:00: sea level +42 cm compared with the theoretical mean sea level, water 12.3 °C",
		"highest +48 cm at Fri 23:00, lowest -5 cm at Sat 00:00",
		"Buoy Suomenlinna, 5.5 km away, at Fri 22:00: not reporting waves now, water 12.8 °C",
		"highest 1.2 m at Fri 23:00")

	if got := formatFMISea(testPlace, fmi.SeaReport{}, loc); !strings.Contains(got, "No sea station") {
		t.Errorf("empty report = %q", got)
	}
}

func TestFormatFMIHistory(t *testing.T) {
	loc := helsinkiLocation(t)
	history := fmi.History{
		Station: "Helsinki Kaisaniemi", DistanceKm: 0.8,
		Days: []fmi.DailyWeather{
			{Date: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), Mean: ptr(12.6), Min: ptr(9.4), Max: ptr(14.7), Precipitation: ptr(2.8), SnowDepth: ptr(0)},
			{Date: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), Mean: ptr(14.1), Min: ptr(12.5), Max: ptr(16.0), Precipitation: ptr(0), SnowDepth: ptr(0)},
		},
		Normals: map[time.Month]fmi.MonthlyNormal{time.October: {MeanTemperature: ptr(6.6), Precipitation: ptr(73.3)}},
	}
	first := time.Date(2026, 10, 5, 0, 0, 0, 0, loc)
	last := time.Date(2026, 10, 7, 0, 0, 0, 0, loc)

	got := formatFMIHistory(testPlace, history, first, last, loc)
	assertContains(t, got,
		"Daily weather at Helsinki Kaisaniemi, 0.8 km away from Helsinki**",
		"- Mon 5 Oct: mean 12.6 °C (9.4 to 14.7), precipitation 2.8 mm, no snow",
		"- Tue 6 Oct: mean 14.1 °C (12.5 to 16.0), no precipitation, no snow",
		"No statistics yet for the days after Tue 6 Oct.",
		"1991-2020 normal for October at this station: mean 6.6 °C, 73 mm of precipitation in the month")

	assertContains(t, formatFMIHistory(testPlace, fmi.History{}, first, last, loc), "no daily statistics")
}

func TestFormatFMILightning(t *testing.T) {
	loc := helsinkiLocation(t)
	if got := formatFMILightning(testPlace, nil, 1, loc); !strings.HasSuffix(got, "No strikes.") || !strings.Contains(got, "in the last 24 hours") {
		t.Errorf("no strikes = %q", got)
	}

	strikes := []fmi.Strike{
		{Time: time.Date(2026, 10, 9, 17, 51, 0, 0, time.UTC), DistanceKm: 22},
		{Time: time.Date(2026, 10, 9, 17, 40, 0, 0, time.UTC), DistanceKm: 8},
	}
	assertContains(t, formatFMILightning(testPlace, strikes, 3, loc),
		"in the last 3 days", "2 strikes. The latest at Fri 20:51, 22 km away. The nearest 8 km away, at Fri 20:40.")
}

func TestFormatFMIHeating(t *testing.T) {
	loc := helsinkiLocation(t)
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	report := fmi.HeatingReport{
		Station: "Helsinki Kaisaniemi", DistanceKm: 0.8,
		Months: []fmi.HeatingMonth{
			{Month: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), DegreeDays: 28, Days: 30},
			{Month: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), DegreeDays: 24.4, Days: 8},
		},
	}
	assertContains(t, formatFMIHeating(testPlace, report, now, loc),
		"Heating degree days (lämmitystarveluku) at Helsinki Kaisaniemi", "- Sep 2026: 28\n", "- Oct 2026: 24 so far (8 days)")
}
