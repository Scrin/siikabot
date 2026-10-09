package fmi

import (
	"context"
	"net/url"
	"sort"
	"strconv"
	"time"
)

// Strike is one lightning strike
type Strike struct {
	Time        time.Time
	DistanceKm  float64
	PeakCurrent float64 // kA
}

// Lightning returns the lightning strikes within radiusKm of a place since a given time, latest first
func Lightning(ctx context.Context, place Place, since time.Time, radiusKm float64) ([]Strike, error) {
	centre := point{lat: place.Lat, lon: place.Lon}
	minLat, minLon, maxLat, maxLon := boundingBox(centre, radiusKm)
	format := func(f float64) string { return strconv.FormatFloat(f, 'f', 4, 64) }

	// The strike data ignores the place and distance parameters, so the query narrows it down with a bounding
	// box, and the exact distance is checked here. Without an end time, FMI answers with only the 24 hours after
	// the start.
	rows, err := timeseries(ctx, url.Values{
		"producer":  {"flash"},
		"bbox":      {format(minLon) + "," + format(minLat) + "," + format(maxLon) + "," + format(maxLat)},
		"starttime": {isoTime(since)},
		"endtime":   {isoTime(time.Now())},
		"param":     {"utctime,latitude,longitude,peak_current"},
	})
	if err != nil {
		return nil, err
	}

	strikes := []Strike{}
	for _, r := range rows {
		at, timeOK := r.utcTime()
		lat, latOK := r.number("latitude")
		lon, lonOK := r.number("longitude")
		if !timeOK || !latOK || !lonOK {
			continue
		}
		distance := distanceKm(centre, point{lat: lat, lon: lon})
		if distance > radiusKm {
			continue
		}
		current, _ := r.number("peak_current")
		strikes = append(strikes, Strike{Time: at, DistanceKm: distance, PeakCurrent: current})
	}

	sort.SliceStable(strikes, func(i, j int) bool { return strikes[i].Time.After(strikes[j].Time) })
	return strikes, nil
}
