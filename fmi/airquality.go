package fmi

import (
	"context"
	"errors"
	"maps"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// AirQualityStation is the latest hourly air quality at one station. A nil field means the station doesn't
// measure that, or had no value for it in the last few hours. Concentrations are in µg/m³.
type AirQualityStation struct {
	Station     string
	DistanceKm  float64
	Time        time.Time
	Index       *float64 // FMI's air quality index, 1 (good) to 5 (very poor)
	PM25        *float64
	PM10        *float64
	NO2         *float64
	O3          *float64
	BlackCarbon *float64
	SO2         *float64
}

// AirQualityHour is one hour of an air quality forecast
type AirQualityHour struct {
	Time          time.Time
	Index         int     // 1 (good) to 5 (very poor)
	Pollutant     string  // the pollutant that sets the index, such as "NO2"
	Concentration float64 // that pollutant's concentration, µg/m³
}

// AirQualityForecast is an hourly forecast from one of FMI's models
type AirQualityForecast struct {
	Model string // "ENFUSER" in the Helsinki region, "SILAM" elsewhere
	Hours []AirQualityHour
}

var airQualityParams = []string{
	"aqindex_pt1h_avg", "pm25_pt1h_avg", "pm10_pt1h_avg", "no2_pt1h_avg", "o3_pt1h_avg", "qbcpm25_pt1h_avg",
	"so2_pt1h_avg",
}

// AirQualityNow returns the latest hourly air quality at the stations nearest a place, nearest first. It combines
// the municipalities' city stations with FMI's own background stations, which cover a few rural sites.
func AirQualityNow(ctx context.Context, place Place) ([]AirQualityStation, error) {
	var stations []stationValues
	var errs []error
	for _, producer := range []string{"airquality_urban", "airquality_fmi"} {
		rows, err := timeseries(ctx, url.Values{
			"producer":         {producer},
			"latlon":           {latLon(place.Lat, place.Lon)},
			"numberofstations": {strconv.Itoa(stationCandidates)},
			"starttime":        {"-3h"},
			"param":            {"stationname,distance,utctime," + strings.Join(airQualityParams, ",")},
		})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		stations = append(stations, latestPerStation(rows, airQualityParams)...)
	}
	if len(errs) == 2 {
		return nil, errors.Join(errs...)
	}

	sortByDistance(stations)
	if len(stations) > stationsNearby {
		stations = stations[:stationsNearby]
	}

	result := make([]AirQualityStation, 0, len(stations))
	for _, s := range stations {
		result = append(result, AirQualityStation{
			Station:     s.station,
			DistanceKm:  s.distance,
			Time:        s.time,
			Index:       s.value("aqindex_pt1h_avg"),
			PM25:        atLeastZero(s.value("pm25_pt1h_avg")),
			PM10:        atLeastZero(s.value("pm10_pt1h_avg")),
			NO2:         atLeastZero(s.value("no2_pt1h_avg")),
			O3:          atLeastZero(s.value("o3_pt1h_avg")),
			BlackCarbon: atLeastZero(s.value("qbcpm25_pt1h_avg")),
			SO2:         atLeastZero(s.value("so2_pt1h_avg")),
		})
	}
	return result, nil
}

// atLeastZero turns a concentration below zero into zero. Analysers measuring next to nothing report noise around
// zero, such as -0.1 µg/m³, and FMI passes it on as it is.
func atLeastZero(v *float64) *float64 {
	if v != nil && *v < 0 {
		zero := 0.0
		return &zero
	}
	return v
}

// pollutantLimits are the upper limits of the first four classes of FMI's air quality index, in µg/m³. A
// concentration at or above the last limit is class 5. The index is the worst pollutant's class.
// https://www.ilmatieteenlaitos.fi/ilmanlaatuindeksi
var pollutantLimits = []struct {
	name   string
	limits [4]float64
}{
	{"PM2.5", [4]float64{10, 25, 50, 75}},
	{"PM10", [4]float64{20, 50, 100, 200}},
	{"NO2", [4]float64{40, 70, 150, 200}},
	{"O3", [4]float64{60, 100, 140, 180}},
	{"SO2", [4]float64{20, 80, 250, 350}},
}

// SatisfactoryLimit is the concentration, in µg/m³, at which a pollutant moves an hour from good to satisfactory
func SatisfactoryLimit(pollutant string) (float64, bool) {
	for _, p := range pollutantLimits {
		if p.name == pollutant {
			return p.limits[0], true
		}
	}
	return 0, false
}

// pollutantClass is a concentration's class on FMI's index for one pollutant
func pollutantClass(limits [4]float64, concentration float64) int {
	for i, limit := range limits {
		if concentration < limit {
			return i + 1
		}
	}
	return 5
}

// airQualityIndex computes FMI's index from concentrations keyed by pollutant name, and names the pollutant
// that sets it. When pollutants tie, the one closest to its next class wins. ok is false when none of the
// indexed pollutants has a value.
func airQualityIndex(concentrations map[string]float64) (index int, pollutant string, ok bool) {
	bestShare := -1.0
	for _, p := range pollutantLimits {
		c, present := concentrations[p.name]
		if !present {
			continue
		}
		class := pollutantClass(p.limits, c)
		share := c / p.limits[min(class-1, 3)]
		if class > index || (class == index && share > bestShare) {
			index, pollutant, bestShare = class, p.name, share
		}
	}
	return index, pollutant, index > 0
}

// AirQualityForecastFor returns an hourly air quality forecast for a place. In the Helsinki region it uses
// ENFUSER, FMI's street-level model, which gives the index itself. Elsewhere it uses SILAM, which gives only
// concentrations, so the index is computed from FMI's limits.
func AirQualityForecastFor(ctx context.Context, place Place, hours int) (AirQualityForecast, error) {
	start := time.Now().UTC().Truncate(time.Hour)
	window := url.Values{
		"latlon":    {latLon(place.Lat, place.Lon)},
		"starttime": {isoTime(start)},
		"endtime":   {isoTime(start.Add(time.Duration(hours) * time.Hour))},
		"timestep":  {"60"},
	}

	enfuser := maps.Clone(window)
	enfuser.Set("producer", "enfuser_helsinki_metropolitan")
	enfuser.Set("param", "utctime,aqindex,pm25concentration,pm10concentration,no2concentration,o3concentration")
	rows, err := timeseries(ctx, enfuser)
	if err == nil && len(rows) > 0 {
		return AirQualityForecast{Model: "ENFUSER", Hours: forecastHours(rows, true)}, nil
	}
	var apiErr *APIError
	if err != nil && !errors.As(err, &apiErr) {
		// A network failure would fail SILAM too, so there's no point trying it
		return AirQualityForecast{}, err
	}
	// ENFUSER covers only the Helsinki region, and answers "No data available" for anywhere else
	log.Debug().Ctx(ctx).Str("place", place.Name).Msg("No ENFUSER forecast for place, using SILAM")

	silam := maps.Clone(window)
	silam.Set("producer", "silamaq_europe_surface")
	silam.Set("param", "utctime,pm25concentration,pm10concentration,no2concentration,o3concentration,so2concentration")
	rows, err = timeseries(ctx, silam)
	if err != nil {
		return AirQualityForecast{}, err
	}
	return AirQualityForecast{Model: "SILAM", Hours: forecastHours(rows, false)}, nil
}

// forecastHours turns forecast rows into hours, computing each hour's index unless the model gave one
func forecastHours(rows []row, modelIndex bool) []AirQualityHour {
	params := map[string]string{
		"PM2.5": "pm25concentration",
		"PM10":  "pm10concentration",
		"NO2":   "no2concentration",
		"O3":    "o3concentration",
		"SO2":   "so2concentration",
	}

	hours := make([]AirQualityHour, 0, len(rows))
	for _, r := range rows {
		at, ok := r.utcTime()
		if !ok {
			continue
		}
		concentrations := map[string]float64{}
		for name, param := range params {
			if c, present := r.number(param); present {
				concentrations[name] = c
			}
		}
		index, pollutant, ok := airQualityIndex(concentrations)
		if !ok {
			continue
		}
		if modelIndex {
			// The model's own index can be worse than the indexed pollutants show, because it covers more
			if given, present := r.number("aqindex"); present && int(given) > index {
				index = int(given)
			}
		}
		hours = append(hours, AirQualityHour{
			Time:          at,
			Index:         index,
			Pollutant:     pollutant,
			Concentration: concentrations[pollutant],
		})
	}
	return hours
}
