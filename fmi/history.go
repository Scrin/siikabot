package fmi

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DailyWeather is one day's statistics at a station
type DailyWeather struct {
	Date          time.Time // midnight UTC of the day
	Mean          *float64  // °C
	Min           *float64  // °C
	Max           *float64  // °C
	Precipitation *float64  // mm, 0 on a dry day
	SnowDepth     *float64  // cm, 0 when there's no snow
}

// statisticsComplete is how long after a day starts, in UTC, FMI has all of its statistics. The mean temperature
// is over the day itself, and the lowest and highest over the 24 hours to 18 UTC, but the precipitation sum runs
// to 06 UTC the next day, and FMI publishes it a little after that.
const statisticsComplete = 32 * time.Hour

// Complete reports whether FMI has finished a day's statistics. Until then, it publishes them as they stand, so
// they can still change, and the precipitation is missing.
func (d DailyWeather) Complete(now time.Time) bool {
	return !now.Before(d.Date.Add(statisticsComplete))
}

// MonthlyNormal is a month's 1991–2020 average at a station
type MonthlyNormal struct {
	MeanTemperature *float64 // °C
	Precipitation   *float64 // mm for the whole month
}

// History is past daily weather at the climate station nearest a place
type History struct {
	Station    string
	DistanceKm float64
	Days       []DailyWeather
	Normals    map[time.Month]MonthlyNormal // empty when the station has no normals
}

var dailyParams = []string{"ta_p1d_avg", "ta_pt24h_min", "ta_pt24h_max", "pra_pt24h_sum", "snd_p1d_instant"}

// WeatherHistory returns daily statistics from first to last, inclusive, at the station nearest a place that has
// them, with the station's 1991–2020 monthly normals to compare them against. FMI publishes a day's statistics a
// while after it ends, so the latest days can be missing.
func WeatherHistory(ctx context.Context, place Place, first, last time.Time) (History, error) {
	firstDay := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, time.UTC)
	lastDay := time.Date(last.Year(), last.Month(), last.Day(), 0, 0, 0, 0, time.UTC)

	// Daily values are stamped at midnight UTC of their day, and timestep=data returns them as they are
	rows, err := timeseries(ctx, url.Values{
		"producer":         {"opendata_daily"},
		"latlon":           {latLon(place.Lat, place.Lon)},
		"numberofstations": {strconv.Itoa(stationCandidates)},
		"starttime":        {isoTime(firstDay)},
		"endtime":          {isoTime(lastDay)},
		"timestep":         {"data"},
		"param":            {"stationname,fmisid,distance,utctime," + strings.Join(dailyParams, ",")},
	})
	if err != nil {
		return History{}, err
	}

	history := History{Normals: map[time.Month]MonthlyNormal{}}
	fmisid := 0
	hasStatistics := func(r row) bool {
		_, ok := dailyWeather(r)
		return ok
	}
	for _, r := range nearestReporting(rows, hasStatistics) {
		day, ok := dailyWeather(r)
		if !ok {
			// No statistics for the day yet
			continue
		}
		if history.Station == "" {
			history.Station = r.text("stationname")
			history.DistanceKm, _ = r.number("distance")
			if id, ok := r.number("fmisid"); ok {
				fmisid = int(id)
			}
		}
		history.Days = append(history.Days, day)
	}

	if fmisid != 0 {
		normals, err := monthlyNormals(ctx, fmisid)
		if err == nil {
			history.Normals = normals
		}
		// Without normals the history is still worth returning; the error was logged where it happened
	}
	return history, nil
}

// dailyWeather reads a day's statistics from a row. ok is false when the row has none of them yet.
func dailyWeather(r row) (DailyWeather, bool) {
	at, ok := r.utcTime()
	if !ok {
		return DailyWeather{}, false
	}
	day := DailyWeather{
		Date:          at,
		Mean:          optional(r, "ta_p1d_avg"),
		Min:           optional(r, "ta_pt24h_min"),
		Max:           optional(r, "ta_pt24h_max"),
		Precipitation: noneAsZero(optional(r, "pra_pt24h_sum")),
		SnowDepth:     noneAsZero(optional(r, "snd_p1d_instant")),
	}
	if day.Mean == nil && day.Min == nil && day.Max == nil && day.Precipitation == nil {
		return DailyWeather{}, false
	}
	return day, true
}

// monthlyNormals returns a station's 1991–2020 monthly normals. FMI stores them as the twelve months of 1991.
func monthlyNormals(ctx context.Context, fmisid int) (map[time.Month]MonthlyNormal, error) {
	rows, err := timeseries(ctx, url.Values{
		"producer":  {"opendata_daily"},
		"fmisid":    {strconv.Itoa(fmisid)},
		"starttime": {"1991-01-01T00:00:00Z"},
		"endtime":   {"1991-12-31T00:00:00Z"},
		"timestep":  {"data"},
		"param":     {"utctime,tap1m,prap1m"},
	})
	if err != nil {
		return nil, err
	}

	normals := map[time.Month]MonthlyNormal{}
	for _, r := range rows {
		at, ok := r.utcTime()
		if !ok {
			continue
		}
		normal := MonthlyNormal{MeanTemperature: optional(r, "tap1m"), Precipitation: optional(r, "prap1m")}
		if normal.MeanTemperature != nil || normal.Precipitation != nil {
			normals[at.Month()] = normal
		}
	}
	return normals, nil
}
