package fmi

import (
	"context"
	"os"
	"testing"
	"time"
)

func waveBuoys(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("testdata/waves.xml")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return string(body)
}

func TestParseWaveBuoys(t *testing.T) {
	buoys, err := parseWaveBuoys([]byte(waveBuoys(t)), helsinki)
	if err != nil {
		t.Fatalf("parseWaveBuoys() error = %v", err)
	}
	if len(buoys) != 2 {
		t.Fatalf("got %d buoys, want 2", len(buoys))
	}

	coastal, offshore := buoys[0], buoys[1]
	if coastal.Station != "Coastal buoy" || offshore.Station != "Offshore buoy" {
		t.Fatalf("buoys not ordered by distance: %s, %s", coastal.Station, offshore.Station)
	}
	// The coastal buoy reports only water temperature; its wave heights are all NaN
	if coastal.WaveHeight != nil {
		t.Errorf("coastal wave height = %v, want nil", *coastal.WaveHeight)
	}
	assertFloat(t, "coastal water temperature", coastal.WaterTemperature, 12.8)
	assertFloat(t, "offshore wave height", offshore.WaveHeight, 0.8)
	assertFloat(t, "offshore wave period", offshore.WavePeriod, 6.1)
	if want := time.Date(2026, 10, 9, 19, 0, 0, 0, time.UTC); !offshore.Time.Equal(want) {
		t.Errorf("offshore time = %v, want %v", offshore.Time, want)
	}
	if coastal.DistanceKm < 4 || coastal.DistanceKm > 7 {
		t.Errorf("coastal distance = %.1f km, want about 5.5", coastal.DistanceKm)
	}
}

func TestSea(t *testing.T) {
	requests := fakeFMI(t, map[string]response{
		"opendata_mareograph": ok(`[{"stationname":"Helsinki Kaivopuisto","distance":2.1,"lat":60.1536293,"lon":24.9562206,"utctime":"20261009T190000","wlev_pt1s_instant":423.0,"tw":12.3}]`),
		"sealevel":            ok(`[{"utctime":"20261009T190000","sealevel":43.8},{"utctime":"20261009T200000","sealevel":40.8}]`),
		"wfs":                 ok(waveBuoys(t)),
		"wam":                 ok(`[{"utctime":"20261009T190000","sigwaveheight":0.9,"waveperiod":4.2,"wavedirection":232.0}]`),
	})

	report, err := Sea(context.Background(), helsinki, 48)
	if err != nil {
		t.Fatalf("Sea() error = %v", err)
	}
	if report.SeaLevel == nil || report.SeaLevel.Station != "Helsinki Kaivopuisto" {
		t.Fatalf("sea level = %+v", report.SeaLevel)
	}
	assertFloat(t, "sea level", report.SeaLevel.SeaLevel, 42.3) // millimetres become centimetres
	assertFloat(t, "water temperature", report.SeaLevel.WaterTemperature, 12.3)
	if len(report.SeaLevelForecast) != 2 || report.SeaLevelForecast[1].SeaLevel != 40.8 {
		t.Errorf("sea level forecast = %+v", report.SeaLevelForecast)
	}

	// The nearest buoy reports no waves, so the nearest one that does is used
	if report.Waves == nil || report.Waves.Station != "Offshore buoy" {
		t.Fatalf("waves = %+v", report.Waves)
	}
	if len(report.WaveForecast) != 1 {
		t.Fatalf("wave forecast = %+v", report.WaveForecast)
	}
	assertFloat(t, "forecast wave height", report.WaveForecast[0].Height, 0.9)

	// The forecasts are asked for at the station and the buoy, not at the place
	for _, r := range requests.list() {
		switch r.URL.Query().Get("producer") {
		case "sealevel":
			if got := r.URL.Query().Get("latlon"); got != "60.15363,24.95622" {
				t.Errorf("sea level forecast asked at %s, want the station", got)
			}
		case "wam":
			if got := r.URL.Query().Get("latlon"); got != "59.96000,25.23000" {
				t.Errorf("wave forecast asked at %s, want the buoy", got)
			}
		}
	}
}

func TestSeaKeepsWhatSucceeded(t *testing.T) {
	fakeFMI(t, map[string]response{
		"opendata_mareograph": ok(`[{"stationname":"Helsinki Kaivopuisto","distance":2.1,"lat":60.15,"lon":24.96,"utctime":"20261009T190000","wlev_pt1s_instant":423.0,"tw":12.3}]`),
		"sealevel":            ok(`[]`),
		"wfs":                 {status: 500, body: "<ExceptionReport><Exception><ExceptionText>Server error</ExceptionText></Exception></ExceptionReport>"},
	})

	report, err := Sea(context.Background(), helsinki, 48)
	if err != nil {
		t.Fatalf("Sea() error = %v, want the sea level without the waves", err)
	}
	if report.SeaLevel == nil || report.Waves != nil {
		t.Errorf("report = %+v", report)
	}
}
