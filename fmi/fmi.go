// Package fmi reads the Finnish Meteorological Institute's open data: weather observations and forecasts,
// warnings, air quality, sea conditions and lightning.
//
// Most of it comes from FMI's timeseries API. It answers in JSON, turns place names into coordinates, finds
// the nearest stations itself and sorts them by distance. FMI's documented interface is the XML WFS service,
// but that needs a parser per response format and has awkward station lookups, so it's used only for the one
// thing the timeseries API lacks: the wave buoys. Warnings come from FMI's CAP feed.
//
// FMI's open data is licensed CC BY 4.0, so whatever presents it has to credit FMI.
package fmi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// FMI's endpoints. Variables rather than constants so tests can point them at a local server.
var (
	timeseriesURL = "https://opendata.fmi.fi/timeseries"
	wfsURL        = "https://opendata.fmi.fi/wfs"
	warningsURL   = "https://alerts.fmi.fi/cap/feed/atom_en-GB.xml"
)

// requestTimeout bounds a single request to FMI. One tool call can make several, inside a chat turn that has a
// deadline of its own.
const requestTimeout = 10 * time.Second

// maxResponseBytes caps how much of a response is read. The largest legitimate ones, the warnings feed and the
// wave buoys, are well under a megabyte.
const maxResponseBytes = 4 << 20

// maxErrorMessageLength caps how much of an error response is kept. FMI's explanations are a line long.
const maxErrorMessageLength = 300

var httpClient = &http.Client{
	Timeout:   requestTimeout,
	Transport: otelhttp.NewTransport(http.DefaultTransport),
}

// APIError is an error response from FMI, carrying FMI's own explanation, such as "Unknown location: Xyzzyville"
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("FMI answered %d: %s", e.StatusCode, e.Message)
}

// exceptionText finds the explanation in a WFS error response, which is XML rather than plain text
var exceptionText = regexp.MustCompile(`(?s)<ExceptionText>(.*?)</ExceptionText>`)

// errorMessage turns an error response body into a short, readable message
func errorMessage(body []byte) string {
	message := string(body)
	if match := exceptionText.FindStringSubmatch(message); match != nil {
		message = match[1]
	}
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > maxErrorMessageLength {
		cut := maxErrorMessageLength
		for cut > 0 && !utf8.RuneStart(message[cut]) {
			cut--
		}
		message = message[:cut] + "…"
	}
	return message
}

// get fetches a URL and returns the body of a successful response. An error response becomes an *APIError.
func get(ctx context.Context, endpoint string, params url.Values) ([]byte, error) {
	requestURL := endpoint
	if len(params) > 0 {
		requestURL += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", requestURL).Msg("Failed to create FMI request")
		return nil, fmt.Errorf("failed to create FMI request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", requestURL).Msg("Failed to fetch FMI data")
		return nil, fmt.Errorf("failed to fetch FMI data: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", requestURL).Msg("Failed to read FMI response")
		return nil, fmt.Errorf("failed to read FMI response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		message := errorMessage(body)
		log.Warn().Ctx(ctx).
			Int("status_code", resp.StatusCode).
			Str("url", requestURL).
			Str("response_message", message).
			Msg("FMI returned an error")
		return nil, &APIError{StatusCode: resp.StatusCode, Message: message}
	}

	return body, nil
}

// row is one line of a timeseries response, from parameter name to value. Numbers arrive as float64, and
// missing values as nil.
type row map[string]any

// timeseries queries FMI's timeseries API. Every query asks for full decimals, because by default the API
// rounds some values, temperatures and wave heights among them, to whole numbers.
func timeseries(ctx context.Context, params url.Values) ([]row, error) {
	params.Set("format", "json")
	params.Set("precision", "double")

	body, err := get(ctx, timeseriesURL, params)
	if err != nil {
		return nil, err
	}

	var rows []row
	if err := json.Unmarshal(body, &rows); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("producer", params.Get("producer")).Msg("Failed to parse FMI timeseries response")
		return nil, fmt.Errorf("failed to parse FMI response: %w", err)
	}
	return rows, nil
}

// number returns a numeric value, and false when it's missing
func (r row) number(key string) (float64, bool) {
	value, ok := r[key].(float64)
	return value, ok
}

// text returns a string value, or "" when it's missing
func (r row) text(key string) string {
	value, _ := r[key].(string)
	return value
}

// timeseriesTimeLayout is how the timeseries API writes times, such as "20261009T190000"
const timeseriesTimeLayout = "20060102T150405"

// utcTime returns the row's utctime value
func (r row) utcTime() (time.Time, bool) {
	t, err := time.Parse(timeseriesTimeLayout, r.text("utctime"))
	return t, err == nil
}

// latLon formats coordinates for the timeseries API's latlon parameter
func latLon(lat, lon float64) string {
	return strconv.FormatFloat(lat, 'f', 5, 64) + "," + strconv.FormatFloat(lon, 'f', 5, 64)
}

// isoTime formats a time for the APIs' starttime and endtime parameters
func isoTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// stationCandidates is how many of the nearest stations a query asks FMI for. FMI counts stations that no longer
// report, and then leaves them out of its answer, so asking for exactly as many as are wanted can return fewer,
// or none: the three air quality stations nearest Espoo's centre are all out of use. The nearest ones that
// reported are picked from the answer.
const stationCandidates = 10

// nearestReporting returns the rows of the nearest station that has a row reports accepts, from a response with
// rows for several stations, or nil when no station has one
func nearestReporting(rows []row, reports func(row) bool) []row {
	nearest, nearestDistance := "", math.Inf(1)
	for _, r := range rows {
		if distance, _ := r.number("distance"); distance < nearestDistance && reports(r) {
			nearest, nearestDistance = r.text("stationname"), distance
		}
	}
	if nearest == "" {
		return nil
	}

	var picked []row
	for _, r := range rows {
		if r.text("stationname") == nearest {
			picked = append(picked, r)
		}
	}
	return picked
}

// stationValues is the latest value of each parameter at one station
type stationValues struct {
	station  string
	fmisid   int
	distance float64 // km
	lat      float64
	lon      float64
	time     time.Time // of the newest row that had any of the parameters
	values   map[string]float64
}

// value returns a parameter's value, or nil when the station had none
func (s stationValues) value(key string) *float64 {
	if v, ok := s.values[key]; ok {
		return &v
	}
	return nil
}

// latestPerStation picks each station's newest value of every parameter, from a response with rows for several
// stations and times, and orders the stations by distance. Stations that had none of the parameters are left out.
//
// Stations don't report every parameter every time: precipitation, for example, comes once an hour while
// temperature comes every ten minutes. Taking each parameter's newest value separately keeps those.
func latestPerStation(rows []row, params []string) []stationValues {
	byStation := map[string]*stationValues{}
	var order []string
	for _, r := range rows {
		name := r.text("stationname")
		station, seen := byStation[name]
		if !seen {
			station = &stationValues{station: name, values: map[string]float64{}}
			station.distance, _ = r.number("distance")
			station.lat, _ = r.number("lat")
			station.lon, _ = r.number("lon")
			if fmisid, ok := r.number("fmisid"); ok {
				station.fmisid = int(fmisid)
			}
			byStation[name] = station
			order = append(order, name)
		}

		at, _ := r.utcTime()
		for _, param := range params {
			v, ok := r.number(param)
			if !ok {
				continue
			}
			// Rows arrive oldest first, so a later row's value replaces an earlier one
			station.values[param] = v
			if at.After(station.time) {
				station.time = at
			}
		}
	}

	stations := make([]stationValues, 0, len(order))
	for _, name := range order {
		if station := byStation[name]; len(station.values) > 0 {
			stations = append(stations, *station)
		}
	}
	sortByDistance(stations)
	return stations
}

// sortByDistance orders stations nearest first. FMI usually sorts them already, but not always.
func sortByDistance(stations []stationValues) {
	sort.SliceStable(stations, func(i, j int) bool { return stations[i].distance < stations[j].distance })
}
