package fmi

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestWeatherHistory(t *testing.T) {
	// The daily statistics and the normals come from the same producer, so they're told apart by what they ask
	fakeHistory(t, `[
		{"stationname":"Helsinki Kaisaniemi","fmisid":100971,"distance":0.8,"utctime":"20261005T000000","ta_p1d_avg":12.6,"ta_pt24h_min":9.4,"ta_pt24h_max":14.7,"pra_pt24h_sum":2.8,"snd_p1d_instant":-1.0},
		{"stationname":"Helsinki Kaisaniemi","fmisid":100971,"distance":0.8,"utctime":"20261006T000000","ta_p1d_avg":14.1,"ta_pt24h_min":12.5,"ta_pt24h_max":16.0,"pra_pt24h_sum":-1.0,"snd_p1d_instant":-1.0},
		{"stationname":"Helsinki Kaisaniemi","fmisid":100971,"distance":0.8,"utctime":"20261007T000000","ta_p1d_avg":null,"ta_pt24h_min":null,"ta_pt24h_max":null,"pra_pt24h_sum":null,"snd_p1d_instant":null}
	]`, `[
		{"utctime":"19910901T000000","tap1m":12.3,"prap1m":55.7},
		{"utctime":"19911001T000000","tap1m":6.6,"prap1m":73.3}
	]`)

	history, err := WeatherHistory(context.Background(), helsinki,
		time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("WeatherHistory() error = %v", err)
	}
	if history.Station != "Helsinki Kaisaniemi" || history.DistanceKm != 0.8 {
		t.Errorf("station = %s at %v km", history.Station, history.DistanceKm)
	}
	// The day without any statistics yet is left out
	if len(history.Days) != 2 {
		t.Fatalf("got %d days, want 2", len(history.Days))
	}
	assertFloat(t, "dry day's precipitation", history.Days[1].Precipitation, 0)
	assertFloat(t, "snow depth", history.Days[0].SnowDepth, 0)
	assertFloat(t, "first day's maximum", history.Days[0].Max, 14.7)

	october, found := history.Normals[time.October]
	if !found {
		t.Fatalf("no normal for October: %+v", history.Normals)
	}
	assertFloat(t, "October mean", october.MeanTemperature, 6.6)
	assertFloat(t, "October precipitation", october.Precipitation, 73.3)
}

// fakeHistory answers the daily statistics query with one response and the normals query, which asks for 1991,
// with another
func fakeHistory(t *testing.T, daily, normals string) {
	t.Helper()
	fakeFMIFunc(t, func(producer, query string) response {
		if producer != "opendata_daily" {
			t.Errorf("unexpected producer %q", producer)
			return response{status: 500}
		}
		if strings.Contains(query, "1991") {
			if !strings.Contains(query, "fmisid=100971") {
				t.Errorf("normals asked without the station: %s", query)
			}
			return ok(normals)
		}
		return ok(daily)
	})
}

func TestLightning(t *testing.T) {
	requests := fakeFMI(t, map[string]response{
		"flash": ok(`[
			{"utctime":"20261009T174843","latitude":60.30,"longitude":24.90,"peak_current":-24},
			{"utctime":"20261009T175127","latitude":60.20,"longitude":25.00,"peak_current":-51},
			{"utctime":"20261009T180000","latitude":61.20,"longitude":24.90,"peak_current":-8}
		]`),
	})

	since := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	strikes, err := Lightning(context.Background(), helsinki, since, 50)
	if err != nil {
		t.Fatalf("Lightning() error = %v", err)
	}
	// The third strike is over 100 km away
	if len(strikes) != 2 {
		t.Fatalf("got %d strikes, want 2", len(strikes))
	}
	if !strikes[0].Time.After(strikes[1].Time) || strikes[0].PeakCurrent != -51 {
		t.Errorf("strikes not latest first: %+v", strikes)
	}
	if strikes[0].DistanceKm > 5 {
		t.Errorf("distance = %.1f km, want about 4", strikes[0].DistanceKm)
	}

	query := requests.list()[0].URL.Query()
	if bbox := strings.Split(query.Get("bbox"), ","); len(bbox) != 4 {
		t.Errorf("bbox = %q", query.Get("bbox"))
	}
	if query.Get("starttime") != "2026-10-09T00:00:00Z" {
		t.Errorf("starttime = %q", query.Get("starttime"))
	}
}

func TestHeatingDegreeDays(t *testing.T) {
	now := time.Now().UTC()
	thisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	lastMonth := thisMonth.AddDate(0, -1, 0)
	day := func(t time.Time) string { return t.Format(timeseriesTimeLayout) }
	fakeFMI(t, map[string]response{
		"lammitystarve": ok(fmt.Sprintf(`[
			{"stationname":"Helsinki Kaisaniemi","distance":0.8,"utctime":"%s","astepaivaluku":10.5},
			{"stationname":"Helsinki Kaisaniemi","distance":0.8,"utctime":"%s","astepaivaluku":12.0},
			{"stationname":"Helsinki Kaisaniemi","distance":0.8,"utctime":"%s","astepaivaluku":null},
			{"stationname":"Helsinki Kaisaniemi","distance":0.8,"utctime":"%s","astepaivaluku":15.0}
		]`, day(lastMonth), day(lastMonth.AddDate(0, 0, 1)), day(lastMonth.AddDate(0, 0, 2)), day(thisMonth))),
	})

	report, err := HeatingDegreeDays(context.Background(), helsinki, 12)
	if err != nil {
		t.Fatalf("HeatingDegreeDays() error = %v", err)
	}
	if report.Station != "Helsinki Kaisaniemi" || len(report.Months) != 2 {
		t.Fatalf("report = %+v", report)
	}
	if got := report.Months[0]; !got.Month.Equal(lastMonth) || got.DegreeDays != 22.5 || got.Days != 2 {
		t.Errorf("last month = %+v", got)
	}
	if got := report.Months[1]; !got.Month.Equal(thisMonth) || got.DegreeDays != 15 || got.Days != 1 {
		t.Errorf("this month = %+v", got)
	}
}
