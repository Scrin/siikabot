package fmi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// response is a canned answer from the fake FMI
type response struct {
	status int
	body   string
}

// ok is a successful response with a body
func ok(body string) response {
	return response{status: http.StatusOK, body: body}
}

// recorder keeps the requests the fake FMI received
type recorder struct {
	mu       sync.Mutex
	requests []*http.Request
}

func (r *recorder) list() []*http.Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*http.Request(nil), r.requests...)
}

// fakeFMIFunc serves answers from a function in place of FMI's endpoints for the rest of a test, and empties the
// caches before and after. The function gets the producer of a timeseries request, or the path of a WFS or
// warnings feed request, and the raw query.
func fakeFMIFunc(t *testing.T, answer func(key, query string) response) *recorder {
	t.Helper()
	recorded := &recorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded.mu.Lock()
		recorded.requests = append(recorded.requests, r)
		recorded.mu.Unlock()

		key := strings.TrimPrefix(r.URL.Path, "/")
		if key == "timeseries" {
			key = r.URL.Query().Get("producer")
		}
		resp := answer(key, r.URL.RawQuery)
		w.WriteHeader(resp.status)
		_, _ = w.Write([]byte(resp.body))
	}))
	t.Cleanup(server.Close)

	originalTimeseries, originalWFS, originalWarnings := timeseriesURL, wfsURL, warningsURL
	timeseriesURL, wfsURL, warningsURL = server.URL+"/timeseries", server.URL+"/wfs", server.URL+"/warnings"
	t.Cleanup(func() { timeseriesURL, wfsURL, warningsURL = originalTimeseries, originalWFS, originalWarnings })

	resetCaches()
	t.Cleanup(resetCaches)
	return recorded
}

// fakeFMI serves canned responses, looked up by producer or path
func fakeFMI(t *testing.T, responses map[string]response) *recorder {
	t.Helper()
	return fakeFMIFunc(t, func(key, _ string) response {
		resp, found := responses[key]
		if !found {
			t.Errorf("unexpected request for %q", key)
			return response{status: http.StatusInternalServerError, body: "unexpected request"}
		}
		return resp
	})
}

func resetCaches() {
	placeCacheMutex.Lock()
	placeCache = map[string]Place{}
	placeCacheMutex.Unlock()

	warningsMutex.Lock()
	warningsCache = nil
	warningsFetched = time.Time{}
	warningsMutex.Unlock()
}

var helsinki = Place{Name: "Helsinki", Region: "Suomi", Lat: 60.1695213, Lon: 24.9354496}

func assertFloat(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Errorf("%s = nil, want %v", name, want)
		return
	}
	if diff := *got - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("%s = %v, want %v", name, *got, want)
	}
}

func TestErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"plain text", "Unknown location: Xyzzyville\n", "Unknown location: Xyzzyville"},
		{
			"WFS exception",
			`<ExceptionReport><Exception><ExceptionText>Unrecognized parameter 'PM25_PT1M_1'</ExceptionText></Exception></ExceptionReport>`,
			"Unrecognized parameter 'PM25_PT1M_1'",
		},
		{"whitespace collapsed", "  No data\n\tavailable  ", "No data available"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errorMessage([]byte(tt.body)); got != tt.want {
				t.Errorf("errorMessage() = %q, want %q", got, tt.want)
			}
		})
	}

	// A long message is cut, and never inside a character
	long := errorMessage([]byte(strings.Repeat("ä", 400)))
	if !strings.HasSuffix(long, "…") || len(long) > maxErrorMessageLength+len("…") {
		t.Errorf("long message not cut: %d bytes", len(long))
	}
	if !strings.HasPrefix(long, "ää") || strings.ContainsRune(long, '�') {
		t.Errorf("long message cut inside a character: %q", long[:10])
	}
}

func TestLatestPerStation(t *testing.T) {
	rows := []row{
		{"stationname": "Far", "distance": 7.5, "utctime": "20261009T190000", "temperature": 11.0},
		{"stationname": "Near", "distance": 0.8, "fmisid": 100971.0, "utctime": "20261009T185000", "temperature": 10.3, "precipitation1h": nil},
		{"stationname": "Near", "distance": 0.8, "fmisid": 100971.0, "utctime": "20261009T190000", "temperature": 10.1, "precipitation1h": 0.2},
		{"stationname": "Near", "distance": 0.8, "fmisid": 100971.0, "utctime": "20261009T191000", "temperature": 9.8, "precipitation1h": nil},
		{"stationname": "Empty", "distance": 3.0, "utctime": "20261009T191000", "temperature": nil},
	}

	stations := latestPerStation(rows, []string{"temperature", "precipitation1h"})
	if len(stations) != 2 {
		t.Fatalf("got %d stations, want 2 (the empty one left out)", len(stations))
	}
	near := stations[0]
	if near.station != "Near" || stations[1].station != "Far" {
		t.Errorf("stations not ordered by distance: %s, %s", near.station, stations[1].station)
	}
	assertFloat(t, "temperature", near.value("temperature"), 9.8)
	// The newest row had no precipitation, so the one before it counts
	assertFloat(t, "precipitation", near.value("precipitation1h"), 0.2)
	if want := time.Date(2026, 10, 9, 19, 10, 0, 0, time.UTC); !near.time.Equal(want) {
		t.Errorf("time = %v, want %v", near.time, want)
	}
	if near.fmisid != 100971 {
		t.Errorf("fmisid = %d, want 100971", near.fmisid)
	}
}

func TestResolvePlace(t *testing.T) {
	requests := fakeFMI(t, map[string]response{
		"pal_skandinavia": ok(`[{"name":"Kallio","region":"Helsinki","lat":60.184269,"lon":24.9503403}]`),
	})
	ctx := context.Background()

	place, err := ResolvePlace(ctx, " Kallio,Helsinki ")
	if err != nil {
		t.Fatalf("ResolvePlace() error = %v", err)
	}
	want := Place{Name: "Kallio", Region: "Helsinki", Lat: 60.184269, Lon: 24.9503403}
	if place != want {
		t.Errorf("ResolvePlace() = %+v, want %+v", place, want)
	}
	if label := place.Label(); label != "Kallio, Helsinki" {
		t.Errorf("Label() = %q", label)
	}
	if got := requests.list()[0].URL.Query().Get("place"); got != "Kallio,Helsinki" {
		t.Errorf("place sent as %q", got)
	}

	// The same name again, in different case, comes from the cache
	if _, err := ResolvePlace(ctx, "kallio,helsinki"); err != nil {
		t.Fatalf("cached ResolvePlace() error = %v", err)
	}
	if len(requests.list()) != 1 {
		t.Errorf("made %d requests, want 1", len(requests.list()))
	}
}

func TestResolvePlaceUnknown(t *testing.T) {
	fakeFMI(t, map[string]response{
		"pal_skandinavia": {status: http.StatusBadRequest, body: "Unknown location: Xyzzyville"},
	})

	_, err := ResolvePlace(context.Background(), "Xyzzyville")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want an *APIError", err)
	}
	if apiErr.Message != "Unknown location: Xyzzyville" {
		t.Errorf("message = %q", apiErr.Message)
	}
	if !strings.Contains(err.Error(), "Unknown location: Xyzzyville") {
		t.Errorf("Error() = %q, should carry FMI's explanation", err.Error())
	}
}

func TestPlaceLabel(t *testing.T) {
	if got := (Place{Name: "Espoo", Region: "Suomi"}).Label(); got != "Espoo" {
		t.Errorf("Label() = %q, the country adds nothing", got)
	}
	if got := (Place{Name: "Riika", Region: "Latvia"}).Label(); got != "Riika, Latvia" {
		t.Errorf("Label() = %q", got)
	}
	if got := (Place{Name: "Helsinki", Region: "helsinki"}).Label(); got != "Helsinki" {
		t.Errorf("Label() = %q, the region repeats the name", got)
	}
	if got := (Place{Name: "Oulu"}).Label(); got != "Oulu" {
		t.Errorf("Label() = %q", got)
	}
}

func TestCurrentWeather(t *testing.T) {
	requests := fakeFMI(t, map[string]response{
		"opendata": ok(`[
			{"stationname":"Helsinki Kaisaniemi","distance":0.8,"utctime":"20261009T194000","temperature":9.9,"windspeedms":1.9,"windgust":2.9,"winddirection":265.0,"humidity":87.0,"precipitation1h":0.0,"totalcloudcover":6.0,"visibility":50000.0,"snowdepth":-1.0,"pressure":987.1},
			{"stationname":"Helsinki Harmaja","distance":7.5,"utctime":"20261009T194900","temperature":11.0,"windspeedms":4.6,"windgust":null,"winddirection":266.0,"humidity":null,"precipitation1h":null,"totalcloudcover":null,"visibility":null,"snowdepth":null,"pressure":null}
		]`),
	})

	observations, err := CurrentWeather(context.Background(), helsinki)
	if err != nil {
		t.Fatalf("CurrentWeather() error = %v", err)
	}
	if len(observations) != 2 {
		t.Fatalf("got %d observations, want 2", len(observations))
	}
	first := observations[0]
	if first.Station != "Helsinki Kaisaniemi" || first.DistanceKm != 0.8 {
		t.Errorf("first station = %s at %v km", first.Station, first.DistanceKm)
	}
	assertFloat(t, "temperature", first.Temperature, 9.9)
	assertFloat(t, "snow depth", first.SnowDepth, 0) // FMI's -1 means no snow
	if observations[1].Humidity != nil {
		t.Errorf("humidity = %v, want nil for a station that doesn't measure it", *observations[1].Humidity)
	}

	query := requests.list()[0].URL.Query()
	if query.Get("latlon") != "60.16952,24.93545" || query.Get("numberofstations") != "3" || query.Get("precision") != "double" {
		t.Errorf("unexpected query: %s", query.Encode())
	}
}

func TestForecast(t *testing.T) {
	requests := fakeFMI(t, map[string]response{
		"pal_skandinavia": ok(`[
			{"utctime":"20261009T190000","temperature":10.4,"windspeedms":1.5,"winddirection":240.0,"hourlymaximumgust":3.3,"precipitation1h":0.0,"pop":63.8,"probabilitythunderstorm":0.0,"totalcloudcover":91.0,"smartsymboltext":"mostly cloudy"},
			{"utctime":"20261009T200000","temperature":10.1,"windspeedms":2.6,"winddirection":250.0,"hourlymaximumgust":4.7,"precipitation1h":0.4,"pop":57.2,"probabilitythunderstorm":2.0,"totalcloudcover":95.2,"smartsymboltext":"overcast"}
		]`),
	})

	forecast, err := Forecast(context.Background(), helsinki, 24)
	if err != nil {
		t.Fatalf("Forecast() error = %v", err)
	}
	if len(forecast) != 2 {
		t.Fatalf("got %d hours, want 2", len(forecast))
	}
	second := forecast[1]
	if second.Weather != "overcast" || !second.Time.Equal(time.Date(2026, 10, 9, 20, 0, 0, 0, time.UTC)) {
		t.Errorf("second hour = %+v", second)
	}
	assertFloat(t, "precipitation", second.Precipitation, 0.4)
	assertFloat(t, "gust", second.WindGust, 4.7)
	assertFloat(t, "precipitation probability", second.PrecipitationProbability, 57.2)

	query := requests.list()[0].URL.Query()
	if query.Get("lang") != "en" || !strings.Contains(query.Get("param"), "smartsymboltext") {
		t.Errorf("unexpected query: %s", query.Encode())
	}
	start, _ := time.Parse(time.RFC3339, query.Get("starttime"))
	end, _ := time.Parse(time.RFC3339, query.Get("endtime"))
	if end.Sub(start) != 24*time.Hour {
		t.Errorf("asked for %v of forecast, want 24h", end.Sub(start))
	}
}
