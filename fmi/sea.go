package fmi

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// SeaLevelStation is the latest reading at the sea level station nearest a place
type SeaLevelStation struct {
	Station          string
	DistanceKm       float64
	Time             time.Time
	SeaLevel         *float64 // cm above the theoretical mean sea level
	WaterTemperature *float64 // °C
	lat, lon         float64
}

// SeaLevelPoint is one hour of the sea level forecast
type SeaLevelPoint struct {
	Time     time.Time
	SeaLevel float64 // cm above the theoretical mean sea level
}

// WaveBuoy is the latest reading at the wave buoy nearest a place. Some buoys report only the water temperature
// outside the open water season.
type WaveBuoy struct {
	Station          string
	DistanceKm       float64
	Time             time.Time
	WaveHeight       *float64 // m, significant wave height
	WavePeriod       *float64 // s
	WaveDirection    *float64 // degrees, where the waves come from
	WaterTemperature *float64 // °C
	lat, lon         float64
}

// WavePoint is one hour of the wave forecast
type WavePoint struct {
	Time      time.Time
	Height    *float64 // m, significant wave height
	Period    *float64 // s
	Direction *float64 // degrees, where the waves come from
}

// SeaReport is what FMI knows about the sea near a place. Parts FMI couldn't provide are nil or empty.
type SeaReport struct {
	SeaLevel         *SeaLevelStation
	SeaLevelForecast []SeaLevelPoint
	Waves            *WaveBuoy
	WaveForecast     []WavePoint // at the buoy, because a place's own coordinates are usually on land
}

// Sea returns sea level, water temperature and waves near a place, with forecasts for the coming hours. Each
// part is fetched separately, and the report has whatever succeeded. It's an error only when nothing did.
func Sea(ctx context.Context, place Place, hours int) (SeaReport, error) {
	var report SeaReport
	var errs []error

	if station, err := nearestSeaLevelStation(ctx, place); err != nil {
		errs = append(errs, err)
	} else if station != nil {
		report.SeaLevel = station
		if forecast, err := seaLevelForecast(ctx, station.lat, station.lon, hours); err != nil {
			errs = append(errs, err)
		} else {
			report.SeaLevelForecast = forecast
		}
	}

	if buoy, err := nearestWaveBuoy(ctx, place); err != nil {
		errs = append(errs, err)
	} else if buoy != nil {
		report.Waves = buoy
		if forecast, err := waveForecast(ctx, buoy.lat, buoy.lon, hours); err != nil {
			errs = append(errs, err)
		} else {
			report.WaveForecast = forecast
		}
	}

	if report.SeaLevel == nil && report.Waves == nil && len(errs) > 0 {
		return report, errors.Join(errs...)
	}
	for _, err := range errs {
		log.Warn().Ctx(ctx).Err(err).Str("place", place.Name).Msg("Part of the FMI sea report is missing")
	}
	return report, nil
}

// nearestSeaLevelStation returns the latest reading at the nearest sea level station, or nil if there's none
func nearestSeaLevelStation(ctx context.Context, place Place) (*SeaLevelStation, error) {
	params := []string{"wlev_pt1s_instant", "tw"}
	rows, err := timeseries(ctx, url.Values{
		"producer":         {"opendata_mareograph"},
		"latlon":           {latLon(place.Lat, place.Lon)},
		"numberofstations": {strconv.Itoa(stationCandidates)},
		"starttime":        {"-3h"},
		"param":            {"stationname,distance,lat,lon,utctime," + strings.Join(params, ",")},
	})
	if err != nil {
		return nil, err
	}
	stations := latestPerStation(rows, params)
	if len(stations) == 0 {
		return nil, nil
	}

	s := stations[0]
	station := &SeaLevelStation{
		Station:          s.station,
		DistanceKm:       s.distance,
		Time:             s.time,
		WaterTemperature: s.value("tw"),
		lat:              s.lat,
		lon:              s.lon,
	}
	if level := s.value("wlev_pt1s_instant"); level != nil {
		// Reported in millimetres, while the forecast uses centimetres
		cm := *level / 10
		station.SeaLevel = &cm
	}
	return station, nil
}

// seaLevelForecast returns the sea level forecast at a point on the coast
func seaLevelForecast(ctx context.Context, lat, lon float64, hours int) ([]SeaLevelPoint, error) {
	start := time.Now().UTC().Truncate(time.Hour)
	rows, err := timeseries(ctx, url.Values{
		"producer":  {"sealevel"},
		"latlon":    {latLon(lat, lon)},
		"starttime": {isoTime(start)},
		"endtime":   {isoTime(start.Add(time.Duration(hours) * time.Hour))},
		"timestep":  {"60"},
		"param":     {"utctime,sealevel"},
	})
	if err != nil {
		return nil, err
	}

	points := make([]SeaLevelPoint, 0, len(rows))
	for _, r := range rows {
		at, timeOK := r.utcTime()
		level, levelOK := r.number("sealevel")
		if timeOK && levelOK {
			points = append(points, SeaLevelPoint{Time: at, SeaLevel: level})
		}
	}
	return points, nil
}

// waveForecast returns the wave forecast at a point at sea
func waveForecast(ctx context.Context, lat, lon float64, hours int) ([]WavePoint, error) {
	start := time.Now().UTC().Truncate(time.Hour)
	rows, err := timeseries(ctx, url.Values{
		"producer":  {"wam"},
		"latlon":    {latLon(lat, lon)},
		"starttime": {isoTime(start)},
		"endtime":   {isoTime(start.Add(time.Duration(hours) * time.Hour))},
		"timestep":  {"60"},
		"param":     {"utctime,sigwaveheight,waveperiod,wavedirection"},
	})
	if err != nil {
		return nil, err
	}

	points := make([]WavePoint, 0, len(rows))
	for _, r := range rows {
		at, ok := r.utcTime()
		if !ok {
			continue
		}
		points = append(points, WavePoint{
			Time:      at,
			Height:    optional(r, "sigwaveheight"),
			Period:    optional(r, "waveperiod"),
			Direction: optional(r, "wavedirection"),
		})
	}
	return points, nil
}

// The wave buoys come from WFS, the one thing here the timeseries API doesn't provide. A timevaluepair
// response has one member per buoy and parameter, each with the buoy's name, position and a series of values.
type wfsTimeValuePairs struct {
	Members []struct {
		Observation struct {
			Names []struct {
				CodeSpace string `xml:"codeSpace,attr"`
				Value     string `xml:",chardata"`
			} `xml:"featureOfInterest>SF_SpatialSamplingFeature>sampledFeature>LocationCollection>member>Location>name"`
			Position string `xml:"featureOfInterest>SF_SpatialSamplingFeature>shape>Point>pos"`
			Series   struct {
				ID     string `xml:"id,attr"`
				Points []struct {
					Time  string `xml:"MeasurementTVP>time"`
					Value string `xml:"MeasurementTVP>value"`
				} `xml:"point"`
			} `xml:"result>MeasurementTimeseries"`
		} `xml:"PointTimeSeriesObservation"`
	} `xml:"member"`
}

// nearestWaveBuoy returns the latest reading at the wave buoy nearest a place that reports waves, or failing
// that the nearest one that reports anything. It's nil when no buoy reports at all.
func nearestWaveBuoy(ctx context.Context, place Place) (*WaveBuoy, error) {
	body, err := get(ctx, wfsURL, url.Values{
		"service":        {"WFS"},
		"version":        {"2.0.0"},
		"request":        {"getFeature"},
		"storedquery_id": {"fmi::observations::wave::timevaluepair"},
		"starttime":      {isoTime(time.Now().Add(-3 * time.Hour).Truncate(time.Hour))},
	})
	if err != nil {
		return nil, err
	}
	buoys, err := parseWaveBuoys(body, place)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to parse FMI wave buoy response")
		return nil, err
	}

	var nearestAny *WaveBuoy
	for i := range buoys {
		if buoys[i].WaveHeight != nil {
			return &buoys[i], nil
		}
		if nearestAny == nil {
			nearestAny = &buoys[i]
		}
	}
	return nearestAny, nil
}

// parseWaveBuoys reads a wave buoy response into buoys with their latest values, nearest the place first.
// Buoys without any values are left out.
func parseWaveBuoys(body []byte, place Place) ([]WaveBuoy, error) {
	var response wfsTimeValuePairs
	if err := xml.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse wave buoys: %w", err)
	}

	byName := map[string]*WaveBuoy{}
	var order []string
	for _, member := range response.Members {
		observation := member.Observation
		var name string
		for _, n := range observation.Names {
			if strings.HasSuffix(n.CodeSpace, "/name") {
				name = strings.TrimSpace(n.Value)
			}
		}
		lat, lon, ok := parsePosition(observation.Position)
		if name == "" || !ok {
			continue
		}

		buoy, seen := byName[name]
		if !seen {
			buoy = &WaveBuoy{
				Station:    name,
				DistanceKm: distanceKm(point{lat: place.Lat, lon: place.Lon}, point{lat: lat, lon: lon}),
				lat:        lat,
				lon:        lon,
			}
			byName[name] = buoy
			order = append(order, name)
		}

		// The series id ends in the parameter's name, such as "obs-obs-1-1-WaveHs"
		parameter := observation.Series.ID[strings.LastIndex(observation.Series.ID, "-")+1:]
		var latest *float64
		var latestTime time.Time
		for _, p := range observation.Series.Points {
			value, err := strconv.ParseFloat(strings.TrimSpace(p.Value), 64)
			at, timeErr := time.Parse(time.RFC3339, strings.TrimSpace(p.Time))
			if err != nil || timeErr != nil || math.IsNaN(value) {
				continue
			}
			latest, latestTime = &value, at
		}
		if latest == nil {
			continue
		}
		if latestTime.After(buoy.Time) {
			buoy.Time = latestTime
		}
		switch parameter {
		case "WaveHs":
			buoy.WaveHeight = latest
		case "WTP":
			buoy.WavePeriod = latest
		case "ModalWDi":
			buoy.WaveDirection = latest
		case "TWATER":
			buoy.WaterTemperature = latest
		}
	}

	buoys := make([]WaveBuoy, 0, len(order))
	for _, name := range order {
		if buoy := byName[name]; !buoy.Time.IsZero() {
			buoys = append(buoys, *buoy)
		}
	}
	sort.SliceStable(buoys, func(i, j int) bool { return buoys[i].DistanceKm < buoys[j].DistanceKm })
	return buoys, nil
}

// parsePosition reads a GML position: latitude and longitude separated by whitespace
func parsePosition(text string) (lat, lon float64, ok bool) {
	fields := strings.Fields(text)
	if len(fields) != 2 {
		return 0, 0, false
	}
	lat, latErr := strconv.ParseFloat(fields[0], 64)
	lon, lonErr := strconv.ParseFloat(fields[1], 64)
	return lat, lon, latErr == nil && lonErr == nil
}
