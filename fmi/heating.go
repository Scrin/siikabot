package fmi

import (
	"context"
	"net/url"
	"time"
)

// HeatingMonth is one month's heating degree days at a station
type HeatingMonth struct {
	Month      time.Time // the first day of the month, UTC
	DegreeDays float64
	Days       int // how many days the sum covers, fewer than the month's for the current month
}

// HeatingReport is monthly heating degree days at the station nearest a place
type HeatingReport struct {
	Station    string
	DistanceKm float64
	Months     []HeatingMonth // oldest first
}

// HeatingDegreeDays returns the monthly heating degree days (lämmitystarveluku) at the station nearest a place,
// for the given number of months up to and including the current one. FMI publishes them per day, and they're
// summed here by month.
func HeatingDegreeDays(ctx context.Context, place Place, months int) (HeatingReport, error) {
	now := time.Now().UTC()
	firstMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -(months - 1), 0)

	rows, err := timeseries(ctx, url.Values{
		"producer":         {"lammitystarve"},
		"latlon":           {latLon(place.Lat, place.Lon)},
		"numberofstations": {"1"},
		"starttime":        {isoTime(firstMonth)},
		"endtime":          {isoTime(now)},
		"timestep":         {"data"},
		"param":            {"stationname,distance,utctime,astepaivaluku"},
	})
	if err != nil {
		return HeatingReport{}, err
	}

	report := HeatingReport{}
	byMonth := map[time.Time]*HeatingMonth{}
	for _, r := range rows {
		at, timeOK := r.utcTime()
		value, valueOK := r.number("astepaivaluku")
		if !timeOK || !valueOK {
			continue
		}
		if report.Station == "" {
			report.Station = r.text("stationname")
			report.DistanceKm, _ = r.number("distance")
		}
		month := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, time.UTC)
		entry, seen := byMonth[month]
		if !seen {
			entry = &HeatingMonth{Month: month}
			byMonth[month] = entry
		}
		entry.DegreeDays += value
		entry.Days++
	}

	for month := firstMonth; !month.After(now); month = month.AddDate(0, 1, 0) {
		if entry, ok := byMonth[month]; ok {
			report.Months = append(report.Months, *entry)
		}
	}
	return report, nil
}
