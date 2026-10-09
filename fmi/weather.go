package fmi

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Observation is the latest weather at one station. A nil field means the station doesn't measure that, or had
// no value for it in the last hour.
type Observation struct {
	Station       string
	DistanceKm    float64
	Time          time.Time
	Temperature   *float64 // °C
	WindSpeed     *float64 // m/s, 10-minute average
	WindGust      *float64 // m/s
	WindDirection *float64 // degrees, where the wind blows from
	Humidity      *float64 // %
	Precipitation *float64 // mm in the last hour
	CloudCover    *float64 // eighths of the sky, 0-8
	Visibility    *float64 // m
	SnowDepth     *float64 // cm, 0 when there's no snow
	Pressure      *float64 // hPa, at sea level
}

var observationParams = []string{
	"temperature", "windspeedms", "windgust", "winddirection", "humidity", "precipitation1h",
	"totalcloudcover", "visibility", "snowdepth", "pressure",
}

// stationsNearby is how many stations observations come from. Not every station measures everything, so the
// nearest few together give a fuller picture than the nearest one alone.
const stationsNearby = 3

// CurrentWeather returns the latest observations at the stations nearest a place, nearest first
func CurrentWeather(ctx context.Context, place Place) ([]Observation, error) {
	rows, err := timeseries(ctx, url.Values{
		"producer":         {"opendata"},
		"latlon":           {latLon(place.Lat, place.Lon)},
		"numberofstations": {strconv.Itoa(stationCandidates)},
		"starttime":        {"-1h"},
		"param":            {"stationname,distance,utctime," + strings.Join(observationParams, ",")},
	})
	if err != nil {
		return nil, err
	}

	stations := latestPerStation(rows, observationParams)
	if len(stations) > stationsNearby {
		stations = stations[:stationsNearby]
	}
	observations := make([]Observation, 0, len(stations))
	for _, s := range stations {
		observations = append(observations, Observation{
			Station:       s.station,
			DistanceKm:    s.distance,
			Time:          s.time,
			Temperature:   s.value("temperature"),
			WindSpeed:     s.value("windspeedms"),
			WindGust:      s.value("windgust"),
			WindDirection: s.value("winddirection"),
			Humidity:      s.value("humidity"),
			Precipitation: s.value("precipitation1h"),
			CloudCover:    s.value("totalcloudcover"),
			Visibility:    s.value("visibility"),
			SnowDepth:     noneAsZero(s.value("snowdepth")),
			Pressure:      s.value("pressure"),
		})
	}
	return observations, nil
}

// noneAsZero turns FMI's -1, which stands for none (no snow, a dry day), into 0
func noneAsZero(v *float64) *float64 {
	if v != nil && *v < 0 {
		zero := 0.0
		return &zero
	}
	return v
}

// ForecastHour is one hour of FMI's official forecast
type ForecastHour struct {
	Time                     time.Time
	Temperature              *float64 // °C
	WindSpeed                *float64 // m/s
	WindDirection            *float64 // degrees, where the wind blows from
	WindGust                 *float64 // m/s, the hour's strongest
	Precipitation            *float64 // mm in the hour
	PrecipitationProbability *float64 // %
	ThunderProbability       *float64 // %
	CloudCover               *float64 // %
	Weather                  string   // FMI's description, such as "mostly cloudy"
}

// Forecast returns FMI's official forecast for a place, hour by hour from the next whole hour up to, but not
// including, until. It's the forecast FMI's meteorologists edit and FMI's own website and app show, and it
// reaches about ten days ahead. The current hour is left out: part of it has gone by, and the observations tell
// how it is now.
func Forecast(ctx context.Context, place Place, until time.Time) ([]ForecastHour, error) {
	start := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
	rows, err := timeseries(ctx, url.Values{
		"producer":  {"pal_skandinavia"},
		"latlon":    {latLon(place.Lat, place.Lon)},
		"starttime": {isoTime(start)},
		// FMI includes the end time in its answer
		"endtime":  {isoTime(until.Add(-time.Hour))},
		"timestep": {"60"},
		"lang":     {"en"},
		"param": {"utctime,temperature,windspeedms,winddirection,hourlymaximumgust,precipitation1h,pop," +
			"probabilitythunderstorm,totalcloudcover,smartsymboltext"},
	})
	if err != nil {
		return nil, err
	}

	forecast := make([]ForecastHour, 0, len(rows))
	for _, r := range rows {
		at, ok := r.utcTime()
		if !ok {
			continue
		}
		forecast = append(forecast, ForecastHour{
			Time:                     at,
			Temperature:              optional(r, "temperature"),
			WindSpeed:                optional(r, "windspeedms"),
			WindDirection:            optional(r, "winddirection"),
			WindGust:                 optional(r, "hourlymaximumgust"),
			Precipitation:            optional(r, "precipitation1h"),
			PrecipitationProbability: optional(r, "pop"),
			ThunderProbability:       optional(r, "probabilitythunderstorm"),
			CloudCover:               optional(r, "totalcloudcover"),
			Weather:                  r.text("smartsymboltext"),
		})
	}
	return forecast, nil
}

// optional returns a row's numeric value, or nil when it's missing
func optional(r row, key string) *float64 {
	if v, ok := r.number(key); ok {
		return &v
	}
	return nil
}
