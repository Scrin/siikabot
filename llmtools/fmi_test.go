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

// forecastHours makes a forecast of count hours from start, with the temperature cycling from 5 to 10 °C, a
// shower of 1.2 mm at 02:00 in Helsinki every night, and the strongest wind of each day at 14:00
func forecastHours(t *testing.T, start time.Time, count int) []fmi.ForecastHour {
	t.Helper()
	loc := helsinkiLocation(t)
	var hours []fmi.ForecastHour
	for i := range count {
		at := start.Add(time.Duration(i) * time.Hour)
		h := fmi.ForecastHour{
			Time: at, Temperature: ptr(float64(5 + i%6)), Weather: "overcast", WindSpeed: ptr(3), WindDirection: ptr(180),
			WindGust: ptr(7), Precipitation: ptr(0), PrecipitationProbability: ptr(10), ThunderProbability: ptr(0),
		}
		switch at.In(loc).Hour() {
		case 2:
			h.Precipitation, h.PrecipitationProbability = ptr(1.2), ptr(70)
		case 14:
			h.WindSpeed, h.WindDirection, h.WindGust = ptr(6), ptr(270), ptr(11)
		}
		hours = append(hours, h)
	}
	return hours
}

func TestFormatFMIForecast(t *testing.T) {
	loc := helsinkiLocation(t)
	// Asked at 00:55 on Saturday: from 01:00 to the end of Monday
	hours := forecastHours(t, time.Date(2026, 10, 9, 22, 0, 0, 0, time.UTC), 71)

	got := formatFMIForecast(testPlace, hours, nil, true, loc)
	assertContains(t, got,
		"Next 23 hours, in 3-hour blocks:\n- Sat 01:00–04:00: 5 to 7 °C, overcast, wind up to 3 m/s from S (gusts 7), precipitation 1.2 mm (probability up to 70 %)\n",
		// The block with the day's strongest wind shows it, and the last block runs to midnight
		"- Sat 13:00–16:00: 5 to 7 °C, overcast, wind up to 6 m/s from W (gusts 11),",
		"- Sat 22:00–00:00:",
		"Following days:\n- Sun 11 Oct: 5 to 10 °C, overcast at midday, precipitation 1.2 mm (probability up to 70 %)",
		"- Mon 12 Oct:",
		"Active warnings: none.")
	if blocks := strings.Count(got, "–"); blocks != 8 {
		t.Errorf("got %d blocks, want 8 covering Saturday from 01:00:\n%s", blocks, got)
	}

	warnings := []fmi.Warning{{Headline: "Yellow wind warning for land areas: Uusimaa", Description: "Gusts up to 20 m/s."}}
	assertContains(t, formatFMIForecast(testPlace, hours, warnings, true, loc),
		"Active warnings:\n- Yellow wind warning for land areas: Uusimaa. Gusts up to 20 m/s.")
	assertContains(t, formatFMIForecast(testPlace, hours, nil, false, loc), "Active warnings: couldn't be checked.")
}

// Asked in the afternoon, the blocks run on to the midnight after tomorrow, so no hours fall between the blocks
// and the days
func TestFormatFMIForecastFromTheAfternoon(t *testing.T) {
	loc := helsinkiLocation(t)
	// From 15:00 on Saturday to the end of Monday
	hours := forecastHours(t, time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC), 57)

	got := formatFMIForecast(testPlace, hours, nil, true, loc)
	assertContains(t, got, "Next 33 hours, in 3-hour blocks:\n- Sat 15:00–18:00:", "- Sun 21:00–00:00:",
		"Following days:\n- Mon 12 Oct:")
	if strings.Contains(got, "- Sun 11 Oct:") {
		t.Errorf("Sunday is both in blocks and on a line of its own:\n%s", got)
	}
}

// A day the forecast ends partway through isn't summed up, and a range that rounds to one temperature is shown as
// that one
func TestFormatFMIForecastDays(t *testing.T) {
	loc := helsinkiLocation(t)
	hours := forecastHours(t, time.Date(2026, 10, 9, 22, 0, 0, 0, time.UTC), 48) // ends at 00:00 on Monday
	for i := range hours {
		if hours[i].Time.In(loc).Day() == 11 {
			hours[i].Temperature = ptr(6.6 + float64(i%2)*0.8)
		}
	}

	got := formatFMIForecast(testPlace, hours, nil, true, loc)
	assertContains(t, got, "- Sun 11 Oct: 7 °C, overcast at midday")
	if strings.Contains(got, "Mon 12 Oct") {
		t.Errorf("Monday is summed up from its first hour:\n%s", got)
	}
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

	// Sunset at 18:29:55 is shown as 18:30, and the day length agrees with the times shown
	espoo := fmi.SunDay{Sunrise: time.Date(2026, 10, 9, 4, 46, 4, 0, time.UTC), Sunset: time.Date(2026, 10, 9, 15, 29, 55, 0, time.UTC)}
	espoo.DayLength = espoo.Sunset.Sub(espoo.Sunrise)
	assertContains(t, formatFMISun(fmi.Place{Name: "Espoo"}, date, espoo, loc), "Sunrise 07:46, sunset 18:30, day length 10 h 44 min.")

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
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

	got := formatFMIHistory(testPlace, history, first, last, now, loc)
	assertContains(t, got,
		"Daily weather at Helsinki Kaisaniemi, 0.8 km away from Helsinki**",
		"- Mon 5 Oct: mean 12.6 °C (9.4 to 14.7), precipitation 2.8 mm, no snow",
		"- Tue 6 Oct: mean 14.1 °C (12.5 to 16.0), no precipitation, no snow",
		"No statistics yet for the days after Tue 6 Oct.",
		"1991-2020 normal for October at this station: mean 6.6 °C, 73 mm of precipitation in the month")
	if strings.Contains(got, "hasn't finished") {
		t.Errorf("finished days are said to be unfinished:\n%s", got)
	}

	assertContains(t, formatFMIHistory(testPlace, fmi.History{}, first, last, now, loc), "no daily statistics")
}

// Just after midnight, yesterday's statistics are still being added to, and its precipitation comes the next
// morning
func TestFormatFMIHistoryUnfinishedDay(t *testing.T) {
	loc := helsinkiLocation(t)
	history := fmi.History{
		Station: "Espoo Tapiola", DistanceKm: 8.1,
		Days: []fmi.DailyWeather{
			{Date: time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC), Mean: ptr(10.1), Min: ptr(8.0), Max: ptr(14.4), SnowDepth: ptr(0)},
			{Date: time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC), Mean: ptr(11.0), Min: ptr(8.7), Max: ptr(13.1), SnowDepth: ptr(0)},
		},
	}
	first := time.Date(2026, 10, 7, 0, 0, 0, 0, loc)
	last := time.Date(2026, 10, 9, 0, 0, 0, 0, loc)
	now := time.Date(2026, 10, 9, 21, 40, 0, 0, time.UTC) // 00:40 on Saturday in Helsinki

	assertContains(t, formatFMIHistory(testPlace, history, first, last, now, loc),
		"- Wed 7 Oct: mean 10.1 °C (8.0 to 14.4), precipitation not reported, no snow",
		"- Fri 9 Oct: mean 11.0 °C (8.7 to 13.1), precipitation not reported yet, no snow",
		"FMI hasn't finished the statistics for Fri 9 Oct yet: they can still change until the next morning")
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
