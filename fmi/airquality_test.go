package fmi

import (
	"context"
	"net/http"
	"testing"
)

func TestAirQualityIndex(t *testing.T) {
	tests := []struct {
		name           string
		concentrations map[string]float64
		wantIndex      int
		wantPollutant  string
		wantOK         bool
	}{
		{"all good", map[string]float64{"PM2.5": 2.2, "NO2": 10.9, "O3": 38.8}, 1, "O3", true},
		{"limit is the next class", map[string]float64{"PM2.5": 10}, 2, "PM2.5", true},
		{"worst pollutant decides", map[string]float64{"PM2.5": 30, "NO2": 75, "O3": 20}, 3, "PM2.5", true},
		{"tie goes to the one closer to its next class", map[string]float64{"NO2": 41, "O3": 99}, 2, "O3", true},
		{"very poor", map[string]float64{"SO2": 400, "PM10": 30}, 5, "SO2", true},
		{"nothing indexed", map[string]float64{}, 0, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index, pollutant, ok := airQualityIndex(tt.concentrations)
			if index != tt.wantIndex || pollutant != tt.wantPollutant || ok != tt.wantOK {
				t.Errorf("airQualityIndex() = %d, %q, %v; want %d, %q, %v",
					index, pollutant, ok, tt.wantIndex, tt.wantPollutant, tt.wantOK)
			}
		})
	}
}

func TestAirQualityNowCombinesNetworks(t *testing.T) {
	fakeFMI(t, map[string]response{
		"airquality_urban": ok(`[
			{"stationname":"Tampere Kaleva","distance":0.8,"utctime":"20261009T180000","aqindex_pt1h_avg":1.0,"pm25_pt1h_avg":0.3,"o3_pt1h_avg":46.0},
			{"stationname":"Tampere Kaleva","distance":0.8,"utctime":"20261009T190000","aqindex_pt1h_avg":2.0,"pm25_pt1h_avg":null,"o3_pt1h_avg":47.0},
			{"stationname":"Tampere Pirkankatu","distance":2.7,"utctime":"20261009T190000","aqindex_pt1h_avg":1.0,"no2_pt1h_avg":2.7},
			{"stationname":"Nokia","distance":50.1,"utctime":"20261009T190000","aqindex_pt1h_avg":1.0}
		]`),
		"airquality_fmi": ok(`[
			{"stationname":"Juupajoki Hyytiälä","distance":46.7,"utctime":"20261009T190000","aqindex_pt1h_avg":1.0,"o3_pt1h_avg":45.2}
		]`),
	})

	stations, err := AirQualityNow(context.Background(), Place{Name: "Tampere", Lat: 61.5, Lon: 23.79})
	if err != nil {
		t.Fatalf("AirQualityNow() error = %v", err)
	}
	if len(stations) != 3 {
		t.Fatalf("got %d stations, want the nearest 3", len(stations))
	}
	if stations[2].Station != "Juupajoki Hyytiälä" {
		t.Errorf("third station = %s, want the background station, which is nearer than Nokia", stations[2].Station)
	}
	kaleva := stations[0]
	assertFloat(t, "index", kaleva.Index, 2)
	assertFloat(t, "PM2.5", kaleva.PM25, 0.3) // the newest hour had none, so the hour before counts
	if kaleva.BlackCarbon != nil {
		t.Errorf("black carbon = %v, want nil", *kaleva.BlackCarbon)
	}
}

func TestAirQualityNowSurvivesOneNetworkFailing(t *testing.T) {
	fakeFMI(t, map[string]response{
		"airquality_urban": {status: http.StatusInternalServerError, body: "Internal error"},
		"airquality_fmi":   ok(`[{"stationname":"Juupajoki Hyytiälä","distance":0.5,"utctime":"20261009T190000","aqindex_pt1h_avg":1.0}]`),
	})

	stations, err := AirQualityNow(context.Background(), Place{Name: "Hyytiälä", Lat: 61.85, Lon: 24.29})
	if err != nil {
		t.Fatalf("AirQualityNow() error = %v", err)
	}
	if len(stations) != 1 {
		t.Errorf("got %d stations, want 1", len(stations))
	}
}

func TestAirQualityForecastUsesENFUSER(t *testing.T) {
	fakeFMI(t, map[string]response{
		"enfuser_helsinki_metropolitan": ok(`[
			{"utctime":"20261009T200000","aqindex":1.0,"pm25concentration":2.0,"pm10concentration":5.0,"no2concentration":7.0,"o3concentration":30.0},
			{"utctime":"20261009T210000","aqindex":2.0,"pm25concentration":5.0,"pm10concentration":6.0,"no2concentration":12.0,"o3concentration":40.0}
		]`),
	})

	forecast, err := AirQualityForecastFor(context.Background(), helsinki, 48)
	if err != nil {
		t.Fatalf("AirQualityForecastFor() error = %v", err)
	}
	if forecast.Model != "ENFUSER" || len(forecast.Hours) != 2 {
		t.Fatalf("forecast = %+v", forecast)
	}
	// The model's index is worse than its indexed pollutants show, and the model's index wins
	if forecast.Hours[1].Index != 2 {
		t.Errorf("second hour index = %d, want the model's 2", forecast.Hours[1].Index)
	}
}

func TestAirQualityForecastFallsBackToSILAM(t *testing.T) {
	requests := fakeFMI(t, map[string]response{
		"enfuser_helsinki_metropolitan": {status: http.StatusBadRequest, body: "No data available for Tampere"},
		"silamaq_europe_surface": ok(`[
			{"utctime":"20261009T200000","pm25concentration":0.5,"pm10concentration":0.6,"no2concentration":1.3,"o3concentration":61.0,"so2concentration":0.1}
		]`),
	})

	forecast, err := AirQualityForecastFor(context.Background(), Place{Name: "Tampere", Lat: 61.5, Lon: 23.79}, 48)
	if err != nil {
		t.Fatalf("AirQualityForecastFor() error = %v", err)
	}
	if forecast.Model != "SILAM" || len(forecast.Hours) != 1 {
		t.Fatalf("forecast = %+v", forecast)
	}
	hour := forecast.Hours[0]
	if hour.Index != 2 || hour.Pollutant != "O3" || hour.Concentration != 61 {
		t.Errorf("hour = %+v, want satisfactory because of O3", hour)
	}
	if len(requests.list()) != 2 {
		t.Errorf("made %d requests, want ENFUSER then SILAM", len(requests.list()))
	}
}
